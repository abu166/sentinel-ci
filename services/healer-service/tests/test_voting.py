import asyncio
import ast
import pytest
from healer import engine, voting
from healer.models import HealRequest


def request():
    return HealRequest(logs='IndexError', files={'app.py':'def last(items):\n    return items[len(items)]\n'},test_command='python3 -m unittest -v',language='python')


def proposals():
    r=request()
    return voting.diverse(engine.candidates('app.py',r.files['app.py'],r.logs,'python'),'python')


def test_boundary_variants_are_structurally_diverse():
    items=proposals()
    assert len(items)==3 and len({ast.dump(ast.parse(item.patched_code)) for item in items})==3
    for item in items:
        scope={};exec(item.patched_code,scope)
        assert scope['last']([1,2,3])==3
    assert voting.diverse([], 'python')==[]


def test_mapping_nil_and_mutex_variants():
    source='def event_id(event):\n    return event["id"]\n'
    items=voting.diverse(engine.candidates('app.py',source,'KeyError','python'),'python')
    assert len(items)==3
    for item in items:
        scope={};exec(item.patched_code,scope);assert scope['event_id']({}) is None
    source='package demo\ntype Payment struct { Amount int }\nfunc amount(p *Payment) int { return p.Amount }\n'
    assert len(voting.diverse(engine.candidates('app.go',source,'panic','go'),'go'))==3
    source='package demo\nimport "sync"\nvar mu sync.Mutex\nvar value int\nfunc inc(){value++}\n'
    items=voting.diverse(engine.candidates('app.go',source,'DATA RACE','go'),'go')
    assert len(items)==2 and items[1].variant=='explicit-unlock'
    assert 'defer' not in items[1].patched_code


@pytest.mark.parametrize('language,before,same,changed',[('python','x=1','x = 1\n','x=2'),('go','package p\nvar x=1','package p\nvar x = 1\n','package p\nvar x=2')])
def test_ast_cost_counts_structural_changes(language,before,same,changed):
    assert voting.ast_cost(before,same,language)==0
    assert voting.ast_cost(before,changed,language)>0


def test_actual_parallel_subprocesses_and_regression_rejection():
    r=request()
    r.files['test_app.py']='import time, unittest\nfrom app import last\nclass TestLast(unittest.TestCase):\n def test_last(self):\n  time.sleep(.08)\n  self.assertEqual(last([1,2]),2)\n def test_empty(self):\n  with self.assertRaises(IndexError): last([])\n'
    active=0;peak=0
    async def execute(files,command,timeout):
        nonlocal active,peak
        active+=1;peak=max(active,peak)
        try:return await engine.run_bundle(files,command,timeout)
        finally:active-=1
    evidence=asyncio.run(voting.verify(proposals(),r,execute,r.test_command,lambda c:{}))
    assert peak==3 and sum(item.test_success for item in evidence)==2
    assert evidence[0].selected and evidence[0].ast_cost==min(item.ast_cost for item in evidence if item.test_success)
    assert evidence[-1].variant=='guarded-last-element' and evidence[-1].score==0
    assert 'AssertionError' in evidence[-1].test_output


def test_global_quota_across_requests(monkeypatch):
    monkeypatch.setenv('HEALER_CANDIDATE_CONCURRENCY','2')
    active=0;peak=0
    async def execute(*args):
        nonlocal active,peak
        active+=1;peak=max(peak,active)
        try:await asyncio.sleep(.01);return 0,'tests passed'
        finally:active-=1
    async def both():return await asyncio.gather(*(voting.verify(proposals(),request(),execute,'regression',lambda c:{}) for _ in range(2)))
    results=asyncio.run(both())
    assert peak==2 and all(len(result)==3 for result in results)


def test_failed_sandbox_and_regression_are_rejected():
    async def failed(*args):return 1,'assertion failed'
    evidence=asyncio.run(voting.verify(proposals(),request(),failed,'regression',lambda c:{}))
    assert all(not item.selected and item.status=='REJECTED' and item.score==0 for item in evidence)
    async def broken(*args):raise OSError('daemon unavailable')
    evidence=asyncio.run(voting.verify(proposals()[:1],request(),broken,'regression',lambda c:{}))
    assert 'Sandbox execution failed' in evidence[0].test_output
    async def regression(files,command,timeout):return (1,'regression failed') if command=='regression' else (0,'target passed')
    evidence=asyncio.run(voting.verify(proposals(),request(),regression,'regression',lambda c:{}))
    assert all(not item.selected for item in evidence)


def test_ties_are_deterministic_and_invalid_syntax_is_rejected():
    same=proposals()[0];alternative=same.model_copy(update={'patched_code':same.patched_code+'\n'})
    async def execute(*args):return 0,'passed'
    a=asyncio.run(voting.verify([same,alternative],request(),execute,'regression',lambda c:{}))
    b=asyncio.run(voting.verify([alternative,same],request(),execute,'regression',lambda c:{}))
    assert a[0].id==b[0].id
    bad=same.model_copy(update={'patched_code':'def ('})
    async def syntax_error(*args):return 1,'SyntaxError'
    evidence=asyncio.run(voting.verify([bad],request(),syntax_error,'regression',lambda c:engine.repair_diagnostics(c,engine.parse_trace(''),'python')))
    assert evidence[0].ast_cost==1_000_000 and 'parser_error' in evidence[0].diagnostics


@pytest.mark.parametrize('value',['0','9'])
def test_invalid_limit(value,monkeypatch):
    monkeypatch.setenv('HEALER_CANDIDATE_CONCURRENCY',value)
    with pytest.raises(ValueError):voting.concurrency_limit()


def test_engine_keeps_all_evidence_and_minimum_edit(monkeypatch):
    calls=0
    async def execute(files,command,timeout):
        nonlocal calls
        calls+=1
        return (1,'IndexError') if files['app.py']==request().files['app.py'] else (0,'verification stdout')
    monkeypatch.setattr(engine,'run_bundle',execute)
    result=asyncio.run(engine.heal(request()))
    assert result.verification_status=='VERIFIED' and len(result.candidates)==3 and calls==7
    assert result.selected_candidate_id==result.candidates[0].id
    assert result.candidates[0].ast_cost==min(item.ast_cost for item in result.candidates)


@pytest.mark.parametrize('language,source,path,logs',[('python','def helper(x):\n return x\ndef target(items):\n return helper(items[len(items)])\ndef untouched():\n return 9\n','pkg/app.py','File "pkg/app.py", line 4, in target'),('go','package repair\nfunc helper(x int) int { return x }\nfunc target(items []int) int { return helper(items[len(items)]) }\n','app.go','app.go:3')])
def test_graph_edges_come_from_actual_calls(language,source,path,logs):
    c=engine.candidates(path,source,'IndexError',language)[0]
    graph=engine.repair_diagnostics(c,engine.parse_trace(logs),language)['call_graph']
    names={node['id']:node['name'] for node in graph['nodes']}
    assert [(names[e['source']],names[e['target']]) for e in graph['edges']]==[('target','helper')]
    assert names[graph['root_id']]=='target'


def test_formatter_work_keeps_async_health_loop_responsive(monkeypatch):
    import threading
    import time
    started = threading.Event()
    finished = threading.Event()
    original = voting.diverse
    def slow_formatter(*args):
        started.set()
        time.sleep(.10)
        result = original(*args)
        finished.set()
        return result
    async def execute(files, command, timeout):
        return (1, 'IndexError') if 'len(items)]' in files['app.py'] else (0, 'tests passed')
    monkeypatch.setattr(voting, 'diverse', slow_formatter)
    monkeypatch.setattr(engine, 'run_bundle', execute)
    async def run():
        task = asyncio.create_task(engine.heal(request()))
        responsive_during_formatting = False
        while not task.done():
            await asyncio.sleep(.005)
            if started.is_set() and not finished.is_set():
                responsive_during_formatting = True
        result = await task
        assert result.verification_status == 'VERIFIED'
        assert responsive_during_formatting
    asyncio.run(run())
