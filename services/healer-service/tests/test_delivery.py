"""Broker failure handling: no input is acknowledged before a durable publication."""
import asyncio
import importlib
from types import SimpleNamespace
from unittest.mock import AsyncMock
import pytest
from healer.models import HealRequest, HealResult

module = importlib.import_module("healer.app")


def delivery(body=None, attempts=0):
    if body is None:
        body = HealRequest(logs="IndexError", files={"app.py": "x = 1"}, test_command="python3 app.py").model_dump_json().encode()
    return SimpleNamespace(body=body, headers={"x-retry-count": attempts}, ack=AsyncMock(), nack=AsyncMock())


def test_success_confirms_result_before_ack(monkeypatch):
    result = HealResult(verification_status="REJECTED", root_cause_analysis="unsupported")
    monkeypatch.setattr(module, "heal", AsyncMock(return_value=result))
    message = delivery()
    async def publish(*args, **kwargs):
        message.ack.assert_not_awaited()
        assert kwargs["routing_key"] == "healer.result"
    exchange = SimpleNamespace(publish=AsyncMock(side_effect=publish))
    asyncio.run(module.process_message(SimpleNamespace(state=SimpleNamespace(pool=None)), message, exchange))
    message.ack.assert_awaited_once()
    message.nack.assert_not_awaited()


@pytest.mark.parametrize("body,attempts,route", [(None, 0, "healer.trigger"), (None, 3, "healer.dead"), (b"{}", 0, "healer.dead")])
def test_transient_retry_and_poison_quarantine(monkeypatch, body, attempts, route):
    monkeypatch.setattr(module, "heal", AsyncMock(side_effect=RuntimeError("temporary database failure")))
    message = delivery(body, attempts)
    exchange = SimpleNamespace(publish=AsyncMock())
    asyncio.run(module.process_message(SimpleNamespace(state=SimpleNamespace(pool=None)), message, exchange))
    assert exchange.publish.await_args.kwargs["routing_key"] == route
    assert exchange.publish.await_args.args[0].headers["x-retry-count"] == attempts + 1
    message.ack.assert_awaited_once()
    message.nack.assert_not_awaited()


def test_republish_failure_requeues_original(monkeypatch):
    monkeypatch.setattr(module, "heal", AsyncMock(side_effect=RuntimeError("temporary failure")))
    message = delivery()
    exchange = SimpleNamespace(publish=AsyncMock(side_effect=ConnectionError("broker disconnected")))
    with pytest.raises(ConnectionError):
        asyncio.run(module.process_message(SimpleNamespace(state=SimpleNamespace(pool=None)), message, exchange))
    message.nack.assert_awaited_once_with(requeue=True)
    message.ack.assert_not_awaited()


@pytest.mark.parametrize("failing", [False, True])
def test_git_publication_delivery_retains_route_and_ack(monkeypatch, failing):
    message=delivery(b'{"patch_id":"c9d081a2-7b1e-4156-8eae-065f0137f4d0","approved":true,"open_pr":false}')
    message.routing_key="git.publish"
    publisher=AsyncMock(side_effect=RuntimeError("temporary Git failure") if failing else None)
    monkeypatch.setattr(module,"publish_patch",publisher)
    pool=object();exchange=SimpleNamespace(publish=AsyncMock())
    asyncio.run(module.process_message(SimpleNamespace(state=SimpleNamespace(pool=pool)),message,exchange))
    publisher.assert_awaited_once_with(pool,"c9d081a2-7b1e-4156-8eae-065f0137f4d0",True,False,None)
    message.ack.assert_awaited_once()
    if failing:assert exchange.publish.await_args.kwargs["routing_key"]=="git.publish"
    else:exchange.publish.assert_not_awaited()
