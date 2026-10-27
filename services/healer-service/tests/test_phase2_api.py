import asyncio,importlib,json
from types import SimpleNamespace
from unittest.mock import AsyncMock
import pytest
from fastapi import HTTPException
from fastapi.testclient import TestClient
from healer.models import HealResult
from test_publisher import Pool,Connection
m=importlib.import_module('healer.app')
def test_internal_auth(monkeypatch):
    monkeypatch.setenv('INTERNAL_TOKEN','secret')
    with pytest.raises(HTTPException) as e:asyncio.run(m.internal_auth(None))
    assert e.value.status_code==401
    asyncio.run(m.internal_auth('secret'));monkeypatch.delenv('INTERNAL_TOKEN');monkeypatch.setenv('DATABASE_URL','configured')
    with pytest.raises(HTTPException) as e:asyncio.run(m.internal_auth(None))
    assert e.value.status_code==503
def test_publish_endpoint(monkeypatch):
    monkeypatch.delenv('DATABASE_URL',raising=False);monkeypatch.delenv('RABBITMQ_URL',raising=False);monkeypatch.setenv('INTERNAL_TOKEN','secret')
    with TestClient(m.app) as client:
        body={'patch_id':'c9d081a2-7b1e-4156-8eae-065f0137f4d0','approved':True};headers={'X-Sentinel-Internal-Token':'secret'}
        assert client.post('/api/v1/publish',json=body).status_code==401
        assert client.post('/api/v1/publish',json=body,headers=headers).status_code==503
        m.app.state.pool=object();monkeypatch.setattr(m,'publish_patch',AsyncMock(return_value={'id':'pull1'}))
        assert client.post('/api/v1/publish',json=body,headers=headers).json()['id']=='pull1'
        monkeypatch.setattr(m,'publish_patch',AsyncMock(side_effect=m.PublishError('not verified')))
        assert client.post('/api/v1/publish',json=body,headers=headers).status_code==409
        m.app.state.pool=None
def test_auto_policy(monkeypatch):
    publish=AsyncMock();monkeypatch.setattr(m,'publish_patch',publish)
    r=HealResult(verification_status='VERIFIED',root_cause_analysis='fix',patch_id='p',pipeline_id='pipeline')
    con=Connection();con.fetchrow=AsyncMock(return_value={'auto_commit':True,'open_pr':True,'manual_approval':True});pool=Pool(con)
    asyncio.run(m.auto_publish(pool,r));publish.assert_not_awaited();con.fetchrow.return_value['manual_approval']=False
    asyncio.run(m.auto_publish(pool,r));publish.assert_awaited_once_with(pool,'p',True,True)
    r.verification_status='REJECTED';asyncio.run(m.auto_publish(pool,r));assert publish.await_count==1
def test_delivery_transactional_outbox(monkeypatch):
    result=HealResult(verification_status='VERIFIED',root_cause_analysis='fix',pipeline_id='c9d081a2-7b1e-4156-8eae-065f0137f4d0',step_id='ccf5c280-b3e5-4ea5-a529-407bca558453')
    monkeypatch.setattr(m,'heal',AsyncMock(return_value=result));monkeypatch.setattr(m,'auto_publish',AsyncMock())
    con=Connection();body={'logs':'failure','files':{'app.py':'x=1'},'test_command':'false'}
    message=SimpleNamespace(body=json.dumps(body).encode(),headers={},ack=AsyncMock(),nack=AsyncMock());exchange=SimpleNamespace(publish=AsyncMock())
    asyncio.run(m.process_message(SimpleNamespace(state=SimpleNamespace(pool=Pool(con))),message,exchange))
    exchange.publish.assert_not_awaited();message.ack.assert_awaited_once()
    outbox=[c for c in con.calls if 'INSERT INTO outbox_events' in c[0]];assert len(outbox)==1 and json.loads(outbox[0][3])['patch_id']==result.patch_id
@pytest.mark.parametrize('dependency',['DATABASE_URL','RABBITMQ_URL'])
def test_health_failures(monkeypatch,dependency):
    monkeypatch.delenv('DATABASE_URL',raising=False);monkeypatch.delenv('RABBITMQ_URL',raising=False)
    with TestClient(m.app) as client:
        monkeypatch.setenv(dependency,'configured');assert client.get('/api/v1/health').status_code==503
        monkeypatch.delenv(dependency)
def test_configured_lifespan_and_analysis(monkeypatch):
    con=Connection();con.fetchval=AsyncMock(return_value=1);pool=Pool(con);pool.acquire=lambda **kw:con;pool.close=AsyncMock()
    monkeypatch.setenv('DATABASE_URL','configured');monkeypatch.setenv('INTERNAL_TOKEN','secret');monkeypatch.delenv('RABBITMQ_URL',raising=False)
    monkeypatch.setattr(m.asyncpg,'create_pool',AsyncMock(return_value=pool));monkeypatch.setattr(m,'heal',AsyncMock(return_value=HealResult(verification_status='REJECTED',root_cause_analysis='unsupported')))
    monkeypatch.setattr(m,'save_result',AsyncMock());monkeypatch.setattr(m,'auto_publish',AsyncMock())
    with TestClient(m.app) as client:
        assert client.get('/api/v1/health').json()['checks']['postgres']=='ok'
        result=client.post('/api/v1/analyze-and-heal',headers={'X-Sentinel-Internal-Token':'secret'},json={'logs':'x','files':{'a.py':'x=1'},'test_command':'false'})
        assert result.status_code==200;m.save_result.assert_awaited_once();m.auto_publish.assert_awaited_once()
    pool.close.assert_awaited_once()
