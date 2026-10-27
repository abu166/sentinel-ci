"""Authorized transactional outbox job tests persisted and Redis-backed WS logs.

Uses only Python standard library and Docker Compose. Deliberately generated probe
credentials never leave this process in reports; unredacted job source is expected.
"""
import argparse
import base64
import hashlib
import json
import os
import secrets
import socket
import struct
import subprocess
import threading
import time
import uuid
from urllib.parse import urlparse, quote

from simulate import BASE, login, request


def sql(statement):
    completed = subprocess.run(["docker", "compose", "-f", "deploy/docker-compose.yml", "exec", "-T", "postgres", "psql", "-U", "sentinel", "-d", "sentinel", "-At", "-v", "ON_ERROR_STOP=1"], input=statement, text=True, capture_output=True)
    if completed.returncode:
        raise AssertionError("QA database transaction failed (SQL output withheld)")
    return completed.stdout.strip()


def literal(value):
    delimiter = "$qa_" + secrets.token_hex(8) + "$"
    return delimiter + value + delimiter


def read_exact(connection, count):
    data = b""
    while len(data) < count:
        chunk = connection.recv(count-len(data))
        if not chunk:
            raise EOFError("WebSocket closed")
        data += chunk
    return data


def websocket(pipeline, token):
    base = urlparse(BASE)
    assert base.scheme == "http", "local WS probe expects HTTP base"
    connection = socket.create_connection((base.hostname, base.port or 80), timeout=15)
    key = base64.b64encode(os.urandom(16)).decode()
    path = f"/ws/pipelines/{pipeline}/logs?token=" + quote(token)
    connection.sendall((f"GET {path} HTTP/1.1\r\nHost: {base.netloc}\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: {key}\r\nSec-WebSocket-Version: 13\r\n\r\n").encode())
    header = b""
    while not header.endswith(b"\r\n\r\n"):
        header += read_exact(connection, 1)
    assert header.startswith(b"HTTP/1.1 101"), "WS authentication/upgrade failed"
    accept = base64.b64encode(hashlib.sha1((key+"258EAFA5-E914-47DA-95CA-C5AB0DC85B11").encode()).digest())
    assert accept.lower() in header.lower()
    return connection


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--heal", action="store_true")
    parser.add_argument("--hold-tests", type=int, default=0)
    options = parser.parse_args()
    token = login()
    pipeline, step = str(uuid.uuid4()), str(uuid.uuid4())
    repo = request("/api/v1/repos")[0]["id"]
    values = ["probe-" + secrets.token_hex(12), "ghp_" + secrets.token_hex(18), "9fQaA2pMzU7kRt4xNv8cLy5wHs1eJg6d", "first-private-line", "second-private-line", "multiline-first", "multiline-second", "yaml-private-first", "yaml-private-second", "triple-private-first", "triple-private-second"]
    lines = ["password="+values[0], values[1], "opaque "+values[2], "-----BEGIN PRIVATE KEY-----", values[3], values[4], "-----END PRIVATE KEY-----", 'api_key="'+values[5], values[6]+'"', "password: |", "  "+values[7], "  "+values[8], "PUBLIC-YAML-END", 'token = """'+values[9], values[10]+'"""', "SAFE-END-OF-PROBE"]
    source = "import sys\n" + "\n".join(f"print({line!r}, file=sys.{'stdout' if index%2 == 0 else 'stderr'})" for index,line in enumerate(lines[:3])) + "\n" + "\n".join(f"print({line!r})" for line in lines[3:]) + "\n"
    files = {"probe.py": source}
    command = "python3 probe.py"
    if options.heal:
        files["probe.py"] += "def last(items):\n    return items[len(items)]\n"
        files["test_probe.py"] = "import unittest\nfrom probe import last\nclass Tests(unittest.TestCase):\n def test_last(self): self.assertEqual(last([1,2]),2)\n"
        command = "python3 -m unittest -v"
        if options.hold_tests:
            assert 0 < options.hold_tests <= 10
            files["test_probe.py"] = files["test_probe.py"].replace("import unittest", "import time, unittest").replace("self.assertEqual", f"time.sleep({options.hold_tests}); self.assertEqual")

    job = {"pipeline_id":pipeline,"commit_sha":"a"*40,"branch":"qa/phase3-masking","files":files,"steps":[{"id":step,"name":"masking-probe","stage":"test","command":command,"depends_on":[]}]}
    payload = literal(json.dumps(job))
    sql(f"BEGIN; INSERT INTO pipelines(id,repo_id,commit_sha,branch,status,triggered_by,started_at) VALUES('{pipeline}','{repo}','{'a'*40}','qa/phase3-masking','PENDING','qa',now()); INSERT INTO pipeline_steps(id,pipeline_id,name,stage,command,status) VALUES('{step}','{pipeline}','masking-probe','test','{command}','PENDING'); INSERT INTO job_snapshots(pipeline_id,payload) VALUES('{pipeline}',{payload}::jsonb); COMMIT;")
    connection = websocket(pipeline, token)
    connection.settimeout(1)
    received, errors, stop = [], [], threading.Event()
    def consume():
        while not stop.is_set():
            try:
                first, second = read_exact(connection, 2)
                length = second & 127
                if length == 126: length = struct.unpack('!H',read_exact(connection,2))[0]
                elif length == 127: length = struct.unpack('!Q',read_exact(connection,8))[0]
                assert length < 4*1024*1024
                mask = read_exact(connection,4) if second & 128 else None
                body = read_exact(connection,length)
                if mask: body = bytes(b^mask[i%4] for i,b in enumerate(body))
                if first & 15 == 1: received.append(json.loads(body))
                elif first & 15 == 8: break
            except socket.timeout:
                continue
            except Exception as error:
                if not stop.is_set(): errors.append(type(error).__name__)
                break
    reader = threading.Thread(target=consume,daemon=True)
    reader.start()
    try:
        sql(f"INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload,idempotency_key,status,attempts,next_retry_at,headers) VALUES('{uuid.uuid4()}','pipeline','{pipeline}','jobs.dispatch',{payload}::jsonb,'qa:{pipeline}','PENDING',0,now(),'{{}}'::jsonb);")
        deadline = time.monotonic()+120
        while time.monotonic()<deadline:
            detail = request(f"/api/v1/pipelines/{pipeline}")
            if detail["status"] == ("HEALED" if options.heal else "PASSED"): break
            assert detail["status"] != "FAILED", "Probe execution failed"
            time.sleep(.5)
        else: raise AssertionError("Probe execution timed out")
        time.sleep(.5)
    finally:
        stop.set()
        reader.join(2)
        connection.close()
    assert not errors, errors
    persisted = [item["content"] for item in detail["logs"]]
    streamed = [item.get("content", "") for item in received]
    for name, logs in [("PostgreSQL",persisted),("Redis-backed WebSocket",streamed)]:
        text = "\n".join(logs)
        assert "SAFE-END-OF-PROBE" in text, name + " did not receive complete output"
        assert not any(value in text for value in values), name + " leaked probe credential"
        assert text.count("[REDACTED]") >= 14, name + " missed redaction evidence"
    candidates_checked = 0
    if options.heal:
        patch = next(p for p in detail["patches"] if p["verification_status"] == "VERIFIED")
        evidence = request(f"/api/v1/patches/{patch['id']}/candidates")["candidates"]
        assert len(evidence) == 3
        for output in [patch["test_output"]] + [c["test_output"] for c in evidence]:
            assert not any(value in output for value in values), "Fresh healer verification output leaked probe credential"
            assert "[REDACTED]" in output
        candidates_checked = len(evidence)
    result = {"result":"passed","healer_candidates_checked":candidates_checked,"pipeline_id":pipeline,"persisted_lines":len(persisted),"ws_lines":len(streamed),"secret_values_tested":len(values),"channels":["PostgreSQL log_chunks","Redis-backed authenticated WebSocket"],"scope":"Output only; source snapshots intentionally retain executable source"}
    print(json.dumps(result,indent=2))
    with open('/tmp/sentinel-phase3-healer-masking.json' if options.heal else '/tmp/sentinel-phase3-masking.json','w') as handle: json.dump(result,handle,indent=2)

if __name__ == '__main__': main()
