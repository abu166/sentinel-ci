from fastapi.testclient import TestClient
from healer.app import app
import asyncio
from healer.app import save_result
from healer.models import HealResult
import json


def test_health_and_request_validation(monkeypatch):
    monkeypatch.delenv("DATABASE_URL", raising=False)
    monkeypatch.delenv("RABBITMQ_URL", raising=False)
    with TestClient(app) as client:
        assert client.get("/api/v1/health").json()["status"] == "ok"
        response = client.post("/api/v1/analyze-and-heal", json={"logs": "IndexError", "files": {"app.py": "def last(items):\n return items[len(items)]\n", "test_app.py": "from app import last\nimport unittest\nclass TestLast(unittest.TestCase):\n def test_last(self):\n  assert last([7]) == 7\n"}, "test_command": "python3 -m unittest -v test_app"})
        assert response.status_code == 200
        assert response.json()["verification_status"] == "VERIFIED"
        assert client.post("/api/v1/analyze-and-heal", json={}).status_code == 422


def test_persist_transaction_is_idempotent():
    statements = []
    class Connection:
        async def __aenter__(self):
            return self
        async def __aexit__(self, *args):
            return False
        def transaction(self):
            return self
        async def execute(self, sql, *args):
            statements.append((sql, args))
    class Pool:
        def acquire(self):
            return Connection()
    result = HealResult(pipeline_id="c9d081a2-7b1e-4156-8eae-065f0137f4d0", step_id="ccf5c280-b3e5-4ea5-a529-407bca558453", verification_status="VERIFIED", root_cause_analysis="boundary", unified_diff="diff", diagnostics={"complexity_before": 1, "complexity_after": 2, "score_basis": "trace-only; not coverage spectrum"})
    asyncio.run(save_result(Pool(), result))
    first_id = result.patch_id
    asyncio.run(save_result(Pool(), result))
    assert result.patch_id == first_id
    assert "ON CONFLICT" in statements[0][0]
    assert statements[1][1][1] == "HEALED"
    assert statements[2][1][1] == "HEALED"
    assert "diagnostics" in statements[0][0]
    assert json.loads(statements[0][1][13]) == result.diagnostics
