"""AST SQL repair accepts a bounded shape and preserves real SQLite query behavior."""
import ast
import asyncio
import json
from pathlib import Path
import pytest
from healer.engine import candidates, heal, run_bundle, sqlite_parameterization, trace_matches
from healer.models import HealRequest


@pytest.mark.parametrize("expression", [
    'db.execute("SELECT id FROM users WHERE name = \'" + name + "\'")',
    'db.execute("SELECT id FROM users WHERE name LIKE \'" + user.name + "\' ORDER BY id")',
])
def test_sql_ast_binds_one_value(expression):
    node = ast.parse(expression, mode="eval").body
    query, value = sqlite_parameterization(node)
    assert "?" in query and query.count("?") == 1
    assert isinstance(value, (ast.Name, ast.Attribute))


@pytest.mark.parametrize("expression", [
    'db.query("SELECT name FROM users")',
    'db.execute("SELECT name FROM users")',
    'db.execute("SELECT name FROM users", ())',
    'db.execute(sql="SELECT name FROM users")',
    'db.execute("SELECT name FROM users" + name)',
    'db.execute(prefix + name + "\'")',
    'db.execute("SELECT name FROM users WHERE name = \'" + name + suffix)',
    'db.execute("SELECT name FROM users WHERE name = \'" + str(name) + "\'")',
    'db.execute("DELETE FROM users WHERE name = \'" + name + "\'")',
    'db.execute("SELECT name FROM users WHERE name = \'" + name + "")',
    'db.execute("SELECT name FROM users WHERE tenant = ? AND name = \'" + name + "\'")',
])
def test_unsupported_sql_does_not_generate_candidate(expression):
    assert sqlite_parameterization(ast.parse(expression, mode="eval").body) is None


def test_parameter_style_requires_sqlite_module():
    body = 'def find(db,name):\n return db.execute("SELECT id FROM users WHERE name = \'" + name + "\'").fetchall()\n'
    assert not candidates("users.py", body, "AssertionError", "python")
    assert candidates("users.py", "from sqlite3 import connect\n" + body, "AssertionError", "python")[0].bug_category == "sql-injection"


@pytest.mark.parametrize("frame,source,match", [
    ("/tmp/run/users.py", "users.py", True),
    ("users.py", "users.py", True),
    ("/tmp/run/test_users.py", "users.py", False),
    ("C:\\run\\users.py", "users.py", True),
])
def test_trace_paths_do_not_confuse_source_with_test_file(frame, source, match):
    assert trace_matches(frame, source) is match


def test_sql_fixture_reproduces_injection_and_passes_security_and_query_regressions():
    fixture = Path(__file__).parent / "fixtures" / "python_sql_injection.json"
    request = HealRequest.model_validate(json.loads(fixture.read_text()))
    baseline, output = asyncio.run(run_bundle(request.files, request.test_command, 10))
    assert baseline != 0 and "test_injected_predicate_cannot_select_other_users" in output
    result = asyncio.run(heal(request))
    assert result.verification_status == "VERIFIED", result.test_output
    assert result.bug_category == "sql-injection"
    assert "(name,)" in result.patched_code
    assert "test_legitimate_name" in result.test_output and "test_quote_in_legitimate_name" in result.test_output
    assert "test_injection_cannot_drop_users" in result.test_output and "Ran 5 tests" in result.test_output
    assert result.test_output.count("OK") == 2
