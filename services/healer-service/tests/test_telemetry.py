import asyncio
import importlib
import json
from types import SimpleNamespace
from unittest.mock import AsyncMock, Mock
import pytest
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter
from healer import telemetry, engine
from healer.models import HealResult
from test_publisher import Pool, Connection
m=importlib.import_module('healer.app')


def tracer(monkeypatch):
    provider=TracerProvider();exporter=InMemorySpanExporter();provider.add_span_processor(SimpleSpanProcessor(exporter))
    monkeypatch.setattr(telemetry,'tracer',provider.get_tracer('healer-test'))
    return exporter


@pytest.mark.parametrize('explicit',[False,True])
def test_sdk_configuration_http_exporter_and_flush(monkeypatch,explicit):
    fake=SimpleNamespace(add_span_processor=Mock(),force_flush=Mock())
    monkeypatch.setattr(telemetry,'_provider',None);monkeypatch.setattr(telemetry,'TracerProvider',Mock(return_value=fake))
    exporter=Mock();monkeypatch.setattr(telemetry,'OTLPSpanExporter',exporter);monkeypatch.setattr(telemetry,'BatchSpanProcessor',lambda item:item)
    monkeypatch.setattr(telemetry.trace,'set_tracer_provider',Mock());monkeypatch.setenv('OTEL_EXPORTER_OTLP_ENDPOINT','http://collector:4318')
    monkeypatch.delenv('OTEL_EXPORTER_OTLP_TRACES_ENDPOINT',raising=False)
    if explicit:monkeypatch.setenv('OTEL_EXPORTER_OTLP_TRACES_ENDPOINT','http://collector:4318/custom')
    assert telemetry.configure() is fake and telemetry.configure() is fake
    exporter.assert_called_once_with(endpoint='http://collector:4318/custom' if explicit else 'http://collector:4318/v1/traces',timeout=5)
    telemetry.flush();fake.force_flush.assert_called_once_with(timeout_millis=5000)


def test_w3c_parent_and_broker_publication_preserve_trace(monkeypatch):
    exporter=tracer(monkeypatch)
    trace_id='1234567890abcdef1234567890abcdef';parent_id='1234567890abcdef'
    headers={'traceparent':f'00-{trace_id}-{parent_id}-01','tracestate':'sentinel=value','Authorization':'do-not-propagate','baggage':'secret=private'}
    result=HealResult(verification_status='REJECTED',root_cause_analysis='unsupported')
    monkeypatch.setattr(m,'heal',AsyncMock(return_value=result))
    message=SimpleNamespace(body=b'{"logs":"failure","files":{"app.py":"x=1"},"test_command":"false"}',headers=headers,routing_key='healer.trigger',ack=AsyncMock(),nack=AsyncMock())
    exchange=SimpleNamespace(publish=AsyncMock())
    asyncio.run(m.process_message(SimpleNamespace(state=SimpleNamespace(pool=None)),message,exchange))
    span=exporter.get_finished_spans()[0]
    assert format(span.context.trace_id,'032x')==trace_id and format(span.parent.span_id,'016x')==parent_id
    outgoing=exchange.publish.await_args.args[0].headers
    assert outgoing['traceparent'].split('-')[1]==trace_id and outgoing['tracestate']=='sentinel=value'
    assert set(outgoing)=={'traceparent','tracestate'}


def test_http_middleware_propagates_only_trace_context(monkeypatch):
    exporter=tracer(monkeypatch)
    request=SimpleNamespace(method='GET',url=SimpleNamespace(path='/api/v1/health'),headers={'traceparent':'00-1234567890abcdef1234567890abcdef-1234567890abcdef-01'})
    response=SimpleNamespace(status_code=503,headers={})
    result=asyncio.run(telemetry.middleware(request,AsyncMock(return_value=response)))
    assert result is response and 'traceparent' in response.headers
    span=exporter.get_finished_spans()[0]
    assert span.attributes['http.response.status_code']==503 and span.status.is_ok is False


def test_candidate_persistence_and_outbox_w3c_are_atomic_and_idempotent(monkeypatch):
    exporter=tracer(monkeypatch)
    async def run(files,command,timeout):return (1,'IndexError') if 'len(items)]' in files['app.py'] else (0,'tests pass')
    monkeypatch.setattr(engine,'run_bundle',run)
    from healer.models import HealRequest
    request=HealRequest(pipeline_id='c9d081a2-7b1e-4156-8eae-065f0137f4d0',step_id='ccf5c280-b3e5-4ea5-a529-407bca558453',logs='IndexError',files={'app.py':'def last(items):\n return items[len(items)]\n'},test_command='python3 -m unittest -v')
    con=Connection();pool=Pool(con)
    async def perform():
        with telemetry.tracer.start_as_current_span('root'):
            result=await engine.heal(request)
            await m.save_result(pool,result)
            identities=[item.id for item in result.candidates]
            selected=result.selected_candidate_id
            await m.save_result(pool,result)
            assert identities==[item.id for item in result.candidates] and selected==result.selected_candidate_id
            return result
    result=asyncio.run(perform())
    candidate_sql=[call for call in con.calls if 'INSERT INTO patch_candidates' in call[0]]
    assert len(candidate_sql)==6 and all('ON CONFLICT DO NOTHING' in call[0] for call in candidate_sql)
    selections=[call for call in con.calls if 'UPDATE healing_patches SET selected_candidate_id' in call[0]]
    assert all('COALESCE' in call[0] for call in selections)
    outbox=[call for call in con.calls if 'INSERT INTO outbox_events' in call[0]]
    assert 'traceparent' in json.loads(outbox[0][-1])
    assert json.loads(outbox[0][3])['selected_candidate_id']==result.selected_candidate_id
    spans=exporter.get_finished_spans()
    assert len([span for span in spans if span.name=='healer.candidate.verify'])==3
