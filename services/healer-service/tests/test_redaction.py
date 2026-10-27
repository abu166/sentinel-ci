import asyncio
import pytest
from healer import engine, voting
from healer.redaction import mask_output, entropy, looks_secret
from test_voting import request, proposals


@pytest.mark.parametrize('text,secret',[
 ('password=hunter2','hunter2'),
 ('password: |\n  first fragment\n  second fragment\npublic=ok','fragment'),
 ('  api_key: >-\n    private folded value\n    more value\npublic=ok','private folded'),
 ('{"api_key": "short-but-private"}','short-but-private'),
 ('Authorization: Bearer abc12345678','abc12345678'),
 ('Bearer Zm9vOmJhcg==','Zm9vOmJhcg=='),
 ('key=ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890','ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890'),
 ('value sk-proj-AbCdEfGhIjKlMnOpQrStUvWxYz123456','sk-proj-AbCdEfGhIjKlMnOpQrStUvWxYz123456'),
 ('JWT eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ1c2VyIn0.signature123','eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ1c2VyIn0.signature123'),
 ('postgres://user:db-password@database/sentinel','db-password'),
 ('secret="first line\nsecond line"\npublic=ok','second line'),
 ("password='''first line\nsecond line'''\npublic=ok",'second line'),
 ('secret="unterminated private value','private value'),
 ('token:\nprivate-next-line\npublic=ok','private-next-line'),
 ('-----BEGIN RSA PRIVATE KEY-----\nprivate material\n-----END RSA PRIVATE KEY-----\npublic=ok','private material'),
 ('-----BEGIN PRIVATE KEY-----\nunfinished private material','private material'),
 ('random=A9bC2dE4fG6hI8jK0lM2nO4pQ6rS8tU0vW2xY4z','A9bC2dE4fG6hI8jK0lM2nO4pQ6rS8tU0vW2xY4z'),
])
def test_credentials_are_masked(text,secret):
    result=mask_output(text)
    assert secret not in result and '[REDACTED]' in result
    if 'public=ok' in text:assert 'public=ok' in result


def test_known_environment_values_and_url_passwords(monkeypatch):
    monkeypatch.setenv('INTERNAL_TOKEN','smallSecret')
    monkeypatch.setenv('DATABASE_URL','postgres://user:dbword@host/db')
    monkeypatch.setenv('MALFORMED_URL','https://[bad')
    assert mask_output('smallSecret dbword')=='[REDACTED] [REDACTED]'
    assert mask_output('token=[REDACTED]')=='token=[REDACTED]'


def test_public_identifiers_paths_and_manifests_are_preserved():
    text='a8f92d1\n0123456789abcdef0123456789abcdef01234567\npkg/inventory/stock.go:84\n14.2s\n'+'abcdef0123456789'*4+'  artifact.tar'
    assert mask_output(text)==text
    assert entropy('')==0 and not looks_secret('/public/path') and not looks_secret('[REDACTED]')
    assert mask_output('a'*5000)=='a'*5000


def test_escaped_values_and_many_assignments_fail_closed():
    text='secret="first\\"private still hidden"\npublic=ok'
    assert 'private' not in mask_output(text) and 'public=ok' in mask_output(text)
    text='\n'.join(f'token{i}=secret{i}' for i in range(80))
    assert 'secret79' not in mask_output(text)


def test_actual_sandbox_output_masking_preserves_source():
    source='print("password=hunter2")\n'
    files={'app.py':source}
    code,output=asyncio.run(engine.run_bundle(files,'python3 app.py',3))
    assert code==0 and 'hunter2' not in output and '[REDACTED]' in output
    assert files['app.py']==source


def test_candidate_target_and_regression_output_is_masked():
    async def execute(*args):return 0,'api_key=private-value\n-----BEGIN PRIVATE KEY-----\nmaterial\n-----END PRIVATE KEY-----'
    evidence=asyncio.run(voting.verify(proposals(),request(),execute,'regression',lambda c:{}))
    assert all('private-value' not in item.test_output and 'material' not in item.test_output for item in evidence)
    assert all(item.original_code==request().files['app.py'] for item in evidence)
