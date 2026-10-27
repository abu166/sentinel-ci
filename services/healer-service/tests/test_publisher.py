import asyncio
import difflib
import io
import json
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import AsyncMock
import pytest
from healer import publisher as p


def record():
    before = 'def last(items):\n    return items[len(items)]\n'
    after = 'def last(items):\n    return items[len(items) - 1]\n'
    return {'id': 'c9d081a2-7b1e-4156-8eae-065f0137f4d0', 'pipeline_id': 'ccf5c280-b3e5-4ea5-a529-407bca558453', 'repository_id': 'd21ad21a-1111-4111-8111-111111111111', 'file_path': 'app.py', 'original_code': before, 'patched_code': after, 'unified_diff': ''.join(difflib.unified_diff(before.splitlines(True), after.splitlines(True), fromfile='a/app.py', tofile='b/app.py')), 'bug_category': 'boundary', 'root_cause_analysis': 'Off-by-one index', 'test_output': '3 tests passed', 'repo_url': 'https://github.com/org/repo', 'base_branch': 'main', 'source_sha': 'abcdef1', 'verification_status': 'VERIFIED'}


def test_real_local_bare_worktree_commit(tmp_path, monkeypatch):
    monkeypatch.delenv('GIT_SIGNING_KEY', raising=False)
    r = record()
    result = p.commit_patch(r, {'app.py': r['original_code']}, tmp_path, 'local')
    assert result['branch'].startswith('sentinel/fix-boundary-')
    assert len(result['commit_sha']) == 40
    assert not result['signed']
    assert '+    return items[len(items) - 1]' in result['diff']
    bare = Path(result['bare_repository'])
    assert p.git(['--git-dir', str(bare), 'show', result['commit_sha'] + ':app.py'], tmp_path) == r['patched_code'].strip()
    body = p.git(['--git-dir', str(bare), 'log', '-1', '--format=%B', result['commit_sha']], tmp_path)
    assert 'Signed-off-by: SentinelCI' in body and 'Sentinel-Patch:' in body
    assert 'Off-by-one index' in body
    assert len(list(tmp_path.glob('*.git'))) == 1
    repeated=p.commit_patch(r, {'app.py': r['original_code']}, tmp_path, 'local')
    assert repeated['branch']==result['branch'] and repeated['commit_sha']==result['commit_sha']


@pytest.mark.parametrize('files', [{}, {'../escape': 'x'}, {'.git/config': 'x'}, {'/etc/passwd': 'x'}, {'a\\b': 'x'}, {'.': 'x'}, {'a/./b': 'x'}, {'huge': 'x' * 4_000_001}, {str(i): 'x' for i in range(101)}])
def test_reject_unsafe_bundle(files):
    with pytest.raises(p.PublishError):
        p.safe_bundle(files)


def test_original_must_match_snapshot(tmp_path):
    with pytest.raises(p.PublishError, match='original'):
        p.commit_patch(record(), {'app.py': 'wrong'}, tmp_path, 'local')


def test_git_command_failure_timeout_and_oserror(tmp_path, monkeypatch):
    with pytest.raises(p.PublishError, match='failed'):
        p.git(['not-a-command'], tmp_path)
    for error in [OSError('absent'), p.subprocess.TimeoutExpired('git', 1)]:
        monkeypatch.setattr(p.subprocess, 'run', lambda *a, **k: (_ for _ in ()).throw(error))
        with pytest.raises(p.PublishError, match='timed out'):
            p.git(['status'], tmp_path)


@pytest.mark.parametrize('provider', ['github', 'gitlab'])
def test_remote_api_adapter(provider, monkeypatch):
    monkeypatch.setenv('GITHUB_TOKEN', 'secret-token')
    monkeypatch.setenv('GITLAB_TOKEN', 'secret-token')
    payload = {'html_url': 'https://github.com/org/repo/pull/7', 'number': 7, 'web_url': 'https://gitlab.com/org/repo/-/merge_requests/7', 'iid': 7}
    captured = []
    def urlopen(req, timeout):
        captured.append(req)
        return io.BytesIO(json.dumps([] if req.get_method() == 'GET' else payload).encode())
    monkeypatch.setattr(p.urllib.request, 'urlopen', urlopen)
    r = record()
    if provider == 'gitlab': r['repo_url'] = 'https://gitlab.com/org/repo.git'
    result = p.open_remote(r, {'title': 'fix', 'body': 'RCA', 'branch': 'sentinel/fix-boundary-aabbccd'}, provider)
    assert result['remote_number'] == 7 and result['status'] == 'OPEN'
    assert 'secret-token' not in captured[0].full_url
    assert captured[0].get_method() == 'GET'
    assert json.loads(captured[1].data)['title'] == 'fix'


def test_remote_api_failure_and_missing_credentials(monkeypatch):
    monkeypatch.delenv('GITHUB_TOKEN', raising=False)
    with pytest.raises(p.PublishError, match='credentials'):
        p.provider_token('github')
    monkeypatch.setenv('GITHUB_TOKEN', 'token')
    monkeypatch.setattr(p.urllib.request, 'urlopen', lambda *a, **k: (_ for _ in ()).throw(OSError('offline')))
    with pytest.raises(p.PublishError, match='lookup failed'):
        p.open_remote(record(), {'title': 'fix', 'body': 'RCA', 'branch': 'b'}, 'github')


def remote_fixture(provider, branch='sentinel/fix-boundary-aabbccd', state='open'):
    return {'html_url': 'https://github.com/org/repo/pull/7', 'number': 7,
            'web_url': 'https://gitlab.com/org/repo/-/merge_requests/7', 'iid': 7,
            'head': {'ref': branch}, 'base': {'ref': 'main'},
            'source_branch': branch, 'target_branch': 'main', 'state': state}


@pytest.mark.parametrize('provider', ['github', 'gitlab'])
@pytest.mark.parametrize('state,expected', [('open', 'OPEN'), ('closed', 'CLOSED'), ('merged', 'MERGED')])
def test_remote_existing_branch_pair_is_reused(provider, state, expected, monkeypatch):
    monkeypatch.setenv('GITHUB_TOKEN', 'token')
    monkeypatch.setenv('GITLAB_TOKEN', 'token')
    calls = []
    def urlopen(req, timeout):
        calls.append(req)
        assert req.get_method() == 'GET'
        return io.BytesIO(json.dumps([remote_fixture(provider, state=state)]).encode())
    monkeypatch.setattr(p.urllib.request, 'urlopen', urlopen)
    result = p.open_remote(record(), {'title': 'fix', 'body': 'RCA', 'branch': 'sentinel/fix-boundary-aabbccd'}, provider)
    assert result['status'] == expected and result['remote_number'] == 7
    assert len(calls) == 1
    query = p.urllib.parse.parse_qs(p.urllib.parse.urlparse(calls[0].full_url).query)
    assert query['state'] == ['all']


@pytest.mark.parametrize('provider', ['github', 'gitlab'])
def test_remote_post_lost_response_reconciles_without_second_post(provider, monkeypatch):
    monkeypatch.setenv('GITHUB_TOKEN', 'token')
    monkeypatch.setenv('GITLAB_TOKEN', 'token')
    methods = []
    created = False
    def urlopen(req, timeout):
        nonlocal created
        methods.append(req.get_method())
        if req.get_method() == 'POST':
            created = True
            raise OSError('response lost after remote accepted request')
        return io.BytesIO(json.dumps([remote_fixture(provider)] if created else []).encode())
    monkeypatch.setattr(p.urllib.request, 'urlopen', urlopen)
    commit = {'title': 'fix', 'body': 'RCA', 'branch': 'sentinel/fix-boundary-aabbccd'}
    assert p.open_remote(record(), commit, provider)['remote_number'] == 7
    assert methods == ['GET', 'POST', 'GET']
    assert p.open_remote(record(), commit, provider)['remote_number'] == 7
    assert methods == ['GET', 'POST', 'GET', 'GET']


@pytest.mark.parametrize('response', [{'unexpected': 'shape'}, [None], [{'head': {}}]])
def test_invalid_remote_lookup_never_creates_request(response, monkeypatch):
    monkeypatch.setenv('GITHUB_TOKEN', 'token')
    calls = []
    def urlopen(req, timeout):
        calls.append(req.get_method())
        return io.BytesIO(json.dumps(response).encode())
    monkeypatch.setattr(p.urllib.request, 'urlopen', urlopen)
    with pytest.raises(p.PublishError, match='lookup failed'):
        p.open_remote(record(), {'title': 'fix', 'body': 'RCA', 'branch': 'b'}, 'github')
    assert calls == ['GET']


def test_remote_rejected_creation_with_no_matching_request(monkeypatch):
    monkeypatch.setenv('GITHUB_TOKEN', 'token')
    calls = []
    def urlopen(req, timeout):
        calls.append(req.get_method())
        if req.get_method() == 'POST':
            raise OSError('rejected')
        return io.BytesIO(b'[]')
    monkeypatch.setattr(p.urllib.request, 'urlopen', urlopen)
    with pytest.raises(p.PublishError, match='creation failed'):
        p.open_remote(record(), {'title': 'fix', 'body': 'RCA', 'branch': 'b'}, 'github')
    assert calls == ['GET', 'POST', 'GET']


@pytest.mark.parametrize('data', [{'html_url': 'http://unsafe', 'number': 1}, {'html_url': 'https://host', 'number': True}])
def test_remote_response_requires_https_and_integer_number(data):
    with pytest.raises(ValueError):
        p.remote_result(data, 'github')


def test_push_hides_credentials_and_handles_errors(monkeypatch, tmp_path):
    monkeypatch.setenv('GITHUB_TOKEN', 'private')
    def run(args, **kwargs):
        assert 'private' not in repr(args)
        assert kwargs['env']['SENTINEL_GIT_CREDENTIAL'] == 'private'
        assert Path(kwargs['env']['GIT_ASKPASS']).is_file()
        return SimpleNamespace(returncode=0)
    monkeypatch.setattr(p.subprocess, 'run', run)
    p.push_remote(record(), tmp_path, 'branch', 'github')
    monkeypatch.setattr(p.subprocess, 'run', lambda *a, **k: SimpleNamespace(returncode=1))
    with pytest.raises(p.PublishError, match='push failed'):
        p.push_remote(record(), tmp_path, 'branch', 'github')
    monkeypatch.setattr(p.subprocess, 'run', lambda *a, **k: (_ for _ in ()).throw(OSError('missing')))
    with pytest.raises(p.PublishError, match='timed out'):
        p.push_remote(record(), tmp_path, 'branch', 'github')


class Connection:
    def __init__(self, existing=None, row=None): self.existing=existing; self.row=row; self.calls=[]
    async def __aenter__(self): return self
    async def __aexit__(self,*args): return False
    def transaction(self): return self
    async def execute(self,*args): self.calls.append(args)
    async def fetchrow(self,sql,*args):
        self.calls.append((sql,*args))
        if sql.startswith('SELECT * FROM pull_requests'): return self.existing
        if sql.startswith('SELECT h.'): return self.row
        return {'id':args[0],'patch_id':args[1],'branch':args[5],'commit_sha':args[7],'status':args[11]}
class Pool:
    def __init__(self, connection): self.connection=connection
    def acquire(self): return self.connection


def test_database_publication_idempotency_and_approval(monkeypatch):
    with pytest.raises(p.PublishError, match='approval'):
        asyncio.run(p.publish_patch(None,'id',False))
    connection=Connection(existing={'id':'existing','provider':'local','status':'LOCAL'})
    assert asyncio.run(p.publish_patch(Pool(connection),'id',True)) == {'id':'existing','provider':'local','status':'LOCAL'}
    assert len(connection.calls)==2
    with pytest.raises(p.PublishError, match='verified'):
        asyncio.run(p.publish_patch(Pool(Connection()),'id',True))
    r=record();r['payload']=json.dumps({'files':{'app.py':r['original_code']}})
    with pytest.raises(p.PublishError, match='provider'):
        asyncio.run(p.publish_patch(Pool(Connection(row=r)),'id',True,provider='invalid'))
    monkeypatch.setattr(p,'commit_patch',lambda *a:{'branch':'b','commit_sha':'a'*40,'title':'fix','body':'rca','diff':'diff'})
    assert asyncio.run(p.publish_patch(Pool(Connection(row=r)),'id',True,provider='local'))['status']=='LOCAL'
    monkeypatch.setattr(p,'open_remote',lambda *a:{'status':'OPEN','url':'https://github.com/org/repo/pull/1','remote_number':1})
    assert asyncio.run(p.publish_patch(Pool(Connection(row=r)),'id',True,provider='github'))['status']=='OPEN'
    assert asyncio.run(p.publish_patch(Pool(Connection(row=r)),'id',True,open_pr=False,provider='github'))['status']=='BRANCH'


@pytest.mark.parametrize('url,revision,reason', [('http://github.com/org/repo','main','approved HTTPS'),('https://evil.com/repo','main','approved HTTPS'),('https://user:pass@github.com/org/repo','main','approved HTTPS'),('https://github.com/org/repo','--upload-pack=evil','unsafe source')])
def test_reject_remote_host_and_revision(tmp_path,monkeypatch,url,revision,reason):
    r=record();r['repo_url']=url;r['source_sha']=revision
    original=p.git
    def fake(args,cwd,data=None):
        if args[0]=='clone': return ''
        return original(args,cwd,data)
    monkeypatch.setattr(p,'git',fake)
    with pytest.raises(p.PublishError,match=reason):p.commit_patch(r,{'app.py':r['original_code']},tmp_path,'github')


def test_snapshot_cannot_enable_controller_hooks(tmp_path):
    r=record()
    hooks=tmp_path/'hooks';hooks.mkdir();marker=tmp_path/'owned'
    hook=hooks/'pre-commit';hook.write_text('#!/bin/sh\ntouch '+str(marker)+'\n');hook.chmod(0o700)
    files={'app.py':r['original_code'],'.gitconfig':f'[core]\n hooksPath = {hooks}\n'}
    result=p.commit_patch(r,files,tmp_path,'local')
    assert len(result['commit_sha'])==40 and not marker.exists()


def test_publication_rejects_denied_patch_and_promotes_remote_branch(monkeypatch):
    r=record();r['approval_status']='PENDING';r['manual_approval']=True
    with pytest.raises(p.PublishError,match='verified'):
        asyncio.run(p.publish_patch(Pool(Connection(row=r)),'id',True))
    r['approval_status']='REJECTED'
    with pytest.raises(p.PublishError,match='verified'):
        asyncio.run(p.publish_patch(Pool(Connection(row=r)),'id',True))
    prior={'id':'existing','provider':'github','status':'BRANCH','repository_id':'repo','title':'fix','body':'RCA','base_branch':'main','branch':'sentinel/fix-aabbccd'}
    con=Connection(existing=prior)
    async def fetch(sql,*args):
        if sql.startswith('SELECT * FROM pull_requests'):return prior
        if sql.startswith('SELECT repo_url'):return {'repo_url':'https://github.com/org/repo'}
        return {**prior,'status':'OPEN'}
    con.fetchrow=fetch
    monkeypatch.setattr(p,'open_remote',lambda *a:{'status':'OPEN','url':'https://github.com/org/repo/pull/1','remote_number':1})
    assert asyncio.run(p.publish_patch(Pool(con),'id',True))['status']=='OPEN'


def test_selected_candidate_branch_conflicts_preserve_prior_git_history(tmp_path):
    r = record()
    files = {'app.py': r['original_code']}
    first = p.commit_patch(r, files, tmp_path, 'local')
    # The same pending patch may be re-selected after an earlier Git-side effect
    # survived a failed database publication transaction.
    r['patched_code'] = 'def last(items):\n    return items[-1]\n'
    r['unified_diff'] = ''.join(difflib.unified_diff(r['original_code'].splitlines(True), r['patched_code'].splitlines(True), fromfile='a/app.py', tofile='b/app.py'))
    second = p.commit_patch(r, files, tmp_path, 'local')
    assert second['branch'] != first['branch'] and second['commit_sha'] != first['commit_sha']
    assert second['branch'].startswith(first['branch'] + '-' + r['id'][:8])
    # Also occupy the legacy UUID-suffixed branch with a distinct verified variant.
    r['patched_code'] = 'def last(items):\n    return items[-1] if items else None\n'
    r['unified_diff'] = ''.join(difflib.unified_diff(r['original_code'].splitlines(True), r['patched_code'].splitlines(True), fromfile='a/app.py', tofile='b/app.py'))
    third = p.commit_patch(r, files, tmp_path, 'local')
    assert third['branch'].startswith(second['branch'] + '-')
    assert len(third['branch'].rsplit('-', 1)[-1]) == 64
    assert len({first['branch'], second['branch'], third['branch']}) == 3
    repeated = p.commit_patch(r, files, tmp_path, 'local')
    assert repeated['branch'] == third['branch'] and repeated['commit_sha'] == third['commit_sha']
    bare = Path(third['bare_repository'])
    for publication in (first, second, third):
        actual = p.git(['--git-dir', str(bare), 'rev-parse', 'refs/heads/' + publication['branch']], tmp_path)
        assert actual == publication['commit_sha']
    assert p.git(['--git-dir', str(bare), 'show', first['commit_sha'] + ':app.py'], tmp_path) == record()['patched_code'].strip()


def test_legacy_approved_whitespace_patch_preserves_exact_verified_bytes(tmp_path):
    r = record()
    r['patched_code'] = 'def last(items):\n    return items[-1] \n'
    r['unified_diff'] = ''.join(difflib.unified_diff(r['original_code'].splitlines(True), r['patched_code'].splitlines(True), fromfile='a/app.py', tofile='b/app.py'))
    result = p.commit_patch(r, {'app.py': r['original_code']}, tmp_path, 'local')
    bare = result['bare_repository']
    expected = p.git(['hash-object', '--stdin'], tmp_path, r['patched_code'])
    actual = p.git(['--git-dir', bare, 'rev-parse', result['commit_sha'] + ':app.py'], tmp_path)
    assert actual == expected
    repeated = p.commit_patch(r, {'app.py': r['original_code']}, tmp_path, 'local')
    assert repeated['commit_sha'] == result['commit_sha']


def test_aborted_baseline_only_branch_can_finish_without_losing_repair_commits(tmp_path):
    import hashlib
    r = record()
    key = hashlib.sha256(r['repository_id'].encode()).hexdigest()[:32]
    bare = tmp_path / (key + '.git')
    p.git(['init', '--bare', str(bare)], tmp_path)
    baseline = tmp_path / 'baseline'
    baseline.mkdir()
    p.git(['init', '-b', 'main'], baseline)
    (baseline / 'app.py').write_text(r['original_code'])
    p.git(['add', '--all'], baseline)
    p.git(['commit', '-m', 'source snapshot'], baseline)
    base_sha = p.git(['rev-parse', 'HEAD'], baseline)
    p.git(['push', str(bare), 'HEAD:refs/heads/sentinel/base-' + r['pipeline_id']], baseline)
    branch = 'sentinel/fix-boundary-' + base_sha[:7]
    p.git(['--git-dir', str(bare), 'update-ref', 'refs/heads/' + branch, base_sha], tmp_path)
    result = p.commit_patch(r, {'app.py': r['original_code']}, tmp_path, 'local')
    assert result['branch'] == branch and result['commit_sha'] != base_sha
    assert p.git(['--git-dir', str(bare), 'rev-parse', result['commit_sha'] + '^'], tmp_path) == base_sha
    repeated = p.commit_patch(r, {'app.py': r['original_code']}, tmp_path, 'local')
    assert repeated['commit_sha'] == result['commit_sha']


def test_failed_new_repair_leaves_no_baseline_only_branch(tmp_path, monkeypatch):
    r = record()
    original = p.git
    def fail_apply(args, cwd, data=None):
        if args[0] == 'apply':
            raise p.PublishError('simulated apply failure')
        return original(args, cwd, data)
    monkeypatch.setattr(p, 'git', fail_apply)
    with pytest.raises(p.PublishError, match='simulated'):
        p.commit_patch(r, {'app.py': r['original_code']}, tmp_path, 'local')
    bare = next(tmp_path.glob('*.git'))
    refs = original(['--git-dir', str(bare), 'for-each-ref', '--format=%(refname)', 'refs/heads/sentinel/'], tmp_path)
    assert 'sentinel/base-' in refs and 'sentinel/fix-' not in refs


def test_all_diverse_go_variants_are_formatted_and_publishable(tmp_path):
    from healer.engine import candidates
    from healer.voting import diverse
    from uuid import NAMESPACE_URL, uuid5
    source = 'package repair\ntype Payment struct { Amount int }\nfunc amount(p *Payment) int {\n return p.Amount\n}\n'
    variants = diverse(candidates('payment.go', source, 'panic: nil pointer', 'go'), 'go')
    assert len(variants) == 3
    for index, candidate in enumerate(variants):
        assert all(line == line.rstrip() for line in candidate.patched_code.splitlines())
        r = record()
        r.update(id=str(uuid5(NAMESPACE_URL, f'go-publish-{index}')), file_path='payment.go', original_code=source, patched_code=candidate.patched_code, bug_category='nil-guard')
        r['unified_diff'] = ''.join(difflib.unified_diff(source.splitlines(True), candidate.patched_code.splitlines(True), fromfile='a/payment.go', tofile='b/payment.go'))
        result = p.commit_patch(r, {'payment.go': source}, tmp_path, 'local')
        assert p.git(['--git-dir', result['bare_repository'], 'diff', '--check', result['commit_sha'] + '^', result['commit_sha']], tmp_path) == ''
