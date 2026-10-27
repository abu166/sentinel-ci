import asyncio
from contextlib import asynccontextmanager
import json
import logging
import hmac
import os
from uuid import NAMESPACE_URL, uuid5
import aio_pika
import asyncpg
from fastapi import FastAPI, HTTPException, Header, Depends
from pydantic import ValidationError, BaseModel
from typing import Literal
from uuid import UUID
from .engine import heal
from .models import HealRequest, HealResult
from .publisher import publish_patch, PublishError
from . import telemetry

logger = logging.getLogger("sentinel.healer")


async def save_result(pool: asyncpg.Pool, result: HealResult, payload: dict | None = None) -> None:
    if not result.pipeline_id or not result.step_id:
        return
    result.patch_id = str(uuid5(NAMESPACE_URL, f"{result.pipeline_id}:{result.step_id}:{result.unified_diff}:{result.verification_status}"))
    async with pool.acquire() as connection:
        async with connection.transaction():
            await connection.execute("""INSERT INTO healing_patches
                (id,pipeline_id,step_id,file_path,bug_category,root_cause_analysis,ast_node_summary,original_code,patched_code,unified_diff,verification_status,confidence_score,test_output,diagnostics)
                VALUES ($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::jsonb) ON CONFLICT (id) DO NOTHING""",
                result.patch_id, result.pipeline_id, result.step_id, result.file_path, result.bug_category, result.root_cause_analysis, result.ast_node_summary, result.original_code, result.patched_code, result.unified_diff, result.verification_status, result.confidence_score, result.test_output, json.dumps(result.diagnostics))
            await connection.execute("UPDATE pipelines SET status=$2, finished_at=NOW() WHERE id=$1::uuid", result.pipeline_id, "HEALED" if result.verification_status == "VERIFIED" else "FAILED")
            await connection.execute("UPDATE pipeline_steps SET status=$2 WHERE id=$1::uuid", result.step_id, "HEALED" if result.verification_status == "VERIFIED" else "FAILED")
            for candidate in result.candidates:
                candidate.diagnostics.setdefault("proposal_id", candidate.id)
                candidate.id = str(uuid5(NAMESPACE_URL, f"{result.patch_id}:{candidate.file_path}:{candidate.patched_code}"))
                if candidate.selected:
                    result.selected_candidate_id = candidate.id
                await connection.execute("""INSERT INTO patch_candidates(id,patch_id,rank,category,variant,file_path,original_code,patched_code,unified_diff,ast_cost,test_success,score,test_output,status,selected,diagnostics)
                    VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16::jsonb) ON CONFLICT DO NOTHING""",
                    candidate.id, result.patch_id, candidate.rank, candidate.category, candidate.variant, candidate.file_path, candidate.original_code, candidate.patched_code, candidate.unified_diff, candidate.ast_cost,
                    candidate.test_success, candidate.score, candidate.test_output, candidate.status, candidate.selected, json.dumps(candidate.diagnostics))
            if result.selected_candidate_id:
                await connection.execute("UPDATE healing_patches SET selected_candidate_id=COALESCE(selected_candidate_id,$2::uuid) WHERE id=$1::uuid", result.patch_id, result.selected_candidate_id)
            event = payload if payload is not None else result.model_dump()
            event["candidates"] = [candidate.model_dump() for candidate in result.candidates]
            event["selected_candidate_id"] = result.selected_candidate_id
            event["patch_id"] = result.patch_id
            await connection.execute("""INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload,idempotency_key,status,attempts,next_retry_at,headers)
                VALUES($1::uuid,'healing_patch',$2::uuid,'healer.result',$3::jsonb,$4,'PENDING',0,NOW(),$5::jsonb) ON CONFLICT(idempotency_key) DO NOTHING""",
                str(uuid5(NAMESPACE_URL, "healer.result:" + result.patch_id)), result.patch_id, json.dumps(event), "healer.result:" + result.patch_id, json.dumps(telemetry.headers()))


async def auto_publish(pool, result: HealResult) -> None:
    if result.verification_status != "VERIFIED" or not result.patch_id:
        return
    async with pool.acquire() as connection:
        policy = await connection.fetchrow("SELECT r.auto_commit,r.open_pr,r.manual_approval FROM repositories r JOIN pipelines p ON p.repo_id=r.id WHERE p.id=$1::uuid", result.pipeline_id)
    if policy and not policy["manual_approval"] and (policy["auto_commit"] or policy["open_pr"]):
        await publish_patch(pool, result.patch_id, True, policy["open_pr"])



async def consume(app: FastAPI) -> None:
    connection = await aio_pika.connect_robust(os.environ["RABBITMQ_URL"])
    app.state.rabbit = connection
    async with connection:
        channel = await connection.channel(publisher_confirms=True)
        await channel.set_qos(prefetch_count=int(os.getenv("HEALER_CONCURRENCY", "2")))
        exchange = await channel.declare_exchange("sentinel", aio_pika.ExchangeType.TOPIC, durable=True)
        queue = await channel.declare_queue("healer.trigger", durable=True)
        await queue.bind(exchange, routing_key="healer.trigger")
        await queue.bind(exchange, routing_key="git.publish")
        dead = await channel.declare_queue("healer.dead", durable=True)
        await dead.bind(exchange, routing_key="healer.dead")
        async with queue.iterator() as iterator:
            async for message in iterator:
                await process_message(app, message, exchange)


async def process_message(app: FastAPI, message, exchange) -> None:
    route = getattr(message, "routing_key", "healer.trigger")
    with telemetry.tracer.start_as_current_span("healer.consume", context=telemetry.parent(getattr(message, "headers", {})),
                                               kind=telemetry.SpanKind.CONSUMER, attributes={"messaging.system": "rabbitmq", "messaging.destination.name": route}):
        await _process_message(app, message, exchange)


async def _process_message(app: FastAPI, message, exchange) -> None:
    """Confirm a result/retry publication before acknowledging its input delivery."""
    try:
        if getattr(message, "routing_key", "healer.trigger") == "git.publish":
            request = PublishRequest.model_validate_json(message.body)
            if app.state.pool is None:
                raise RuntimeError("publication database unavailable")
            await publish_patch(app.state.pool, str(request.patch_id), request.approved, request.open_pr, request.provider)
            await message.ack()
            return
        request = HealRequest.model_validate_json(message.body)
        async with semaphore:
            result = await heal(request)
        payload = result.model_dump()
        payload["files"] = {**request.files, result.file_path: result.patched_code} if result.verification_status == "VERIFIED" else request.files
        payload["test_command"] = request.test_command
        payload["regression_command"] = request.regression_command
        payload["language"] = request.language
        if app.state.pool is not None:
            await save_result(app.state.pool, result, payload)
            await auto_publish(app.state.pool, result)
        else:
            await exchange.publish(aio_pika.Message(body=json.dumps(payload).encode(), content_type="application/json", delivery_mode=aio_pika.DeliveryMode.PERSISTENT, headers=telemetry.headers()), routing_key="healer.result")
    except asyncio.CancelledError:
        await message.nack(requeue=True)
        raise
    except Exception as error:
        try:
            attempts = max(0, int((message.headers or {}).get("x-retry-count", 0)))
        except (ValueError, TypeError):
            attempts = 0
        original_route = "git.publish" if getattr(message, "routing_key", "healer.trigger") == "git.publish" else "healer.trigger"
        route = "healer.dead" if isinstance(error, ValidationError) or attempts >= 3 else original_route
        logger.exception("Healing delivery failed", extra={"retry_count": attempts, "route": route})
        retry = aio_pika.Message(body=message.body, content_type="application/json", delivery_mode=aio_pika.DeliveryMode.PERSISTENT,
                                 headers={**telemetry.headers(), "x-retry-count": attempts + 1, "x-error-type": type(error).__name__})
        try:
            await exchange.publish(retry, routing_key=route)
        except Exception:
            await message.nack(requeue=True)
            raise
    await message.ack()


async def supervise(app: FastAPI) -> None:
    while True:
        try:
            await consume(app)
        except asyncio.CancelledError:
            raise
        except Exception:
            logger.exception("Healer consumer failed; reconnecting")
            await asyncio.sleep(3)


@asynccontextmanager
async def lifespan(app: FastAPI):
    telemetry.configure()
    app.state.pool = await asyncpg.create_pool(os.environ["DATABASE_URL"], min_size=1, max_size=5) if os.getenv("DATABASE_URL") else None
    app.state.rabbit = None
    task = asyncio.create_task(supervise(app)) if os.getenv("RABBITMQ_URL") else None
    try:
        yield
    finally:
        if task:
            task.cancel()
            await asyncio.gather(task, return_exceptions=True)
        if app.state.rabbit:
            await app.state.rabbit.close()
        if app.state.pool:
            await app.state.pool.close()
        await asyncio.to_thread(telemetry.flush)


app = FastAPI(title="SentinelCI Healer", version="1.0.0", lifespan=lifespan)
app.middleware("http")(telemetry.middleware)
semaphore = asyncio.Semaphore(int(os.getenv("HEALER_CONCURRENCY", "2")))


@app.get("/api/v1/health")
async def health():
    checks = {"engine": "ok"}
    pool = getattr(app.state, "pool", None)
    rabbit = getattr(app.state, "rabbit", None)
    if os.getenv("DATABASE_URL"):
        try:
            if pool is None:
                raise RuntimeError("database pool unavailable")
            async with pool.acquire(timeout=3) as connection:
                await connection.fetchval("SELECT 1")
            checks["postgres"] = "ok"
        except Exception as error:
            raise HTTPException(503, "PostgreSQL unavailable") from error
    if os.getenv("RABBITMQ_URL"):
        if rabbit is None or rabbit.is_closed:
            raise HTTPException(503, "RabbitMQ unavailable")
        checks["rabbitmq"] = "ok"
    return {"status": "ok", "checks": checks}


async def internal_auth(x_sentinel_internal_token: str | None = Header(default=None)) -> None:
    expected = os.getenv("INTERNAL_TOKEN", "")
    if expected and not hmac.compare_digest(x_sentinel_internal_token or "", expected):
        raise HTTPException(401, "valid internal service token required")
    if not expected and os.getenv("DATABASE_URL"):
        raise HTTPException(503, "internal service authentication is not configured")


class PublishRequest(BaseModel):
    patch_id: UUID
    approved: bool = False
    open_pr: bool = True
    provider: Literal["local", "github", "gitlab"] | None = None


@app.post("/api/v1/publish", dependencies=[Depends(internal_auth)])
async def publish(request: PublishRequest):
    pool = getattr(app.state, "pool", None)
    if pool is None:
        raise HTTPException(503, "publication database unavailable")
    try:
        return await publish_patch(pool, str(request.patch_id), request.approved, request.open_pr, request.provider)
    except PublishError as error:
        raise HTTPException(409, str(error)) from error


@app.post("/api/v1/analyze-and-heal", response_model=HealResult, dependencies=[Depends(internal_auth)])
async def analyze_and_heal(request: HealRequest):
    async with semaphore:
        result = await heal(request)
        pool = getattr(app.state, "pool", None)
        if pool is not None:
            await save_result(pool, result)
            await auto_publish(pool, result)
        return result
