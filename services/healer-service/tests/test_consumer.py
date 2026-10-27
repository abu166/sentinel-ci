import asyncio,importlib
from types import SimpleNamespace
from unittest.mock import AsyncMock
import pytest
m=importlib.import_module('healer.app')
class Context:
    async def __aenter__(self):return self
    async def __aexit__(self,*args):return False
class Iterator(Context):
    def __init__(self):self.count=0
    def __aiter__(self):return self
    async def __anext__(self):
        self.count+=1
        if self.count==1:return 'message'
        raise StopAsyncIteration
class Queue:
    def __init__(self):self.bind=AsyncMock()
    def iterator(self):return Iterator()
class Connection(Context):
    def __init__(self,channel):self.channel=AsyncMock(return_value=channel);self.close=AsyncMock();self.is_closed=False

def test_consumer_topology_and_durable_processing(monkeypatch):
    queues=[]
    async def declare(*args,**kwargs):q=Queue();queues.append(q);return q
    channel=SimpleNamespace(set_qos=AsyncMock(),declare_exchange=AsyncMock(return_value='exchange'),declare_queue=AsyncMock(side_effect=declare))
    connection=Connection(channel);monkeypatch.setenv('RABBITMQ_URL','configured')
    monkeypatch.setattr(m.aio_pika,'connect_robust',AsyncMock(return_value=connection));monkeypatch.setattr(m,'process_message',AsyncMock())
    app=SimpleNamespace(state=SimpleNamespace())
    asyncio.run(m.consume(app));assert app.state.rabbit is connection
    m.process_message.assert_awaited_once_with(app,'message','exchange')
    assert [call.args[0] for call in channel.declare_queue.await_args_list]==['healer.trigger','healer.dead']
    assert queues[0].bind.await_count==2 and queues[1].bind.await_count==1
def test_supervision_reconnect_and_cancellation(monkeypatch):
    monkeypatch.setattr(m,'consume',AsyncMock(side_effect=[ConnectionError('lost'),asyncio.CancelledError()]))
    monkeypatch.setattr(m.asyncio,'sleep',AsyncMock())
    with pytest.raises(asyncio.CancelledError):asyncio.run(m.supervise(object()))
    m.asyncio.sleep.assert_awaited_once_with(3)
def test_shutdown_cancels_consumer_closes_connection(monkeypatch):
    monkeypatch.delenv('DATABASE_URL',raising=False);monkeypatch.setenv('RABBITMQ_URL','configured')
    connection=Connection(None)
    async def consumer(app):
        app.state.rabbit=connection
        await asyncio.Event().wait()
    monkeypatch.setattr(m,'supervise',consumer)
    async def run():
        app=SimpleNamespace(state=SimpleNamespace())
        async with m.lifespan(app):
            await asyncio.sleep(.01)
            assert app.state.rabbit is connection
    asyncio.run(run());connection.close.assert_awaited_once()
