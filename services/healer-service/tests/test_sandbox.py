import asyncio, io, tarfile
from unittest.mock import AsyncMock
import pytest
from healer import sandbox, engine

def test_archive_and_security_arguments():
    with tarfile.open(fileobj=io.BytesIO(sandbox.archive({'app.py':'print(7)'}))) as tar:
        member=tar.getmember('app.py');assert member.uid==65532 and tar.extractfile(member).read()==b'print(7)'
    args=sandbox.arguments('job','python3 app.py')
    assert all(x in args for x in ['--read-only','none','ALL','no-new-privileges','--memory','--cpus','--pids-limit','65532:65532'])
    assert args[-1]=='python3 app.py'
class Input:
    def __init__(self,fail=False):self.closed=False;self.fail=fail
    def write(self,data):
        if self.fail:raise BrokenPipeError()
    async def drain(self):return None
    def close(self):self.closed=True
class Output:
    def __init__(self,slow=False):self.count=0;self.slow=slow
    async def read(self,size):
        if self.slow:await asyncio.sleep(5)
        self.count+=1;return b'x'*1_100_000 if self.count==1 else b''
class Process:
    def __init__(self,code=0,slow=False,fail=False):self.stdin=Input(fail);self.stdout=Output(slow);self.returncode=code
    async def wait(self):return self.returncode
@pytest.mark.parametrize('code,broken',[(0,False),(1,True)])
def test_bounded_capture(monkeypatch,code,broken):
    process=Process(code,fail=broken)
    monkeypatch.setattr(sandbox.asyncio,'create_subprocess_exec',AsyncMock(return_value=process));monkeypatch.setattr(sandbox,'remove',AsyncMock())
    result=asyncio.run(sandbox.run({'app.py':'x=1'},'true',2))
    assert result[0]==code and len(result[1])==1_000_000 and process.stdin.closed
    sandbox.remove.assert_awaited_once()
def test_timeout_and_cancellation(monkeypatch):
    monkeypatch.setattr(sandbox.asyncio,'create_subprocess_exec',AsyncMock(return_value=Process(slow=True)));monkeypatch.setattr(sandbox,'remove',AsyncMock())
    assert asyncio.run(sandbox.run({'a':'x'},'sleep 10',.01))[0]==124
    async def cancelled():
        task=asyncio.create_task(sandbox.run({'a':'x'},'sleep 10',10));await asyncio.sleep(.01);task.cancel()
        with pytest.raises(asyncio.CancelledError):await task
    asyncio.run(cancelled());assert sandbox.remove.await_count==4
def test_remove_and_engine_routing(monkeypatch):
    monkeypatch.setattr(sandbox.asyncio,'create_subprocess_exec',AsyncMock(return_value=Process()));asyncio.run(sandbox.remove('job'))
    monkeypatch.setattr(sandbox.asyncio,'create_subprocess_exec',AsyncMock(side_effect=OSError()));asyncio.run(sandbox.remove('job'))
    monkeypatch.setenv('SANDBOX_MODE','docker');monkeypatch.setattr(sandbox,'run',AsyncMock(return_value=(0,'output')))
    assert asyncio.run(engine.run_bundle({'a':'x'},'true',1))==(0,'output')
