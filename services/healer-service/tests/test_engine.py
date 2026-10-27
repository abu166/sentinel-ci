import asyncio
import json
from pathlib import Path
import pytest
from pydantic import ValidationError
from healer.analysis import parse_trace, slice_context, spectrum
from healer.engine import candidates, heal, run_bundle
from healer.models import HealRequest


@pytest.mark.parametrize("fixture", sorted(Path(__file__).parent.joinpath("fixtures").glob("*.json")), ids=lambda path: path.stem)
def test_real_bug_is_reproduced_and_verified(fixture):
    request = HealRequest.model_validate_json(fixture.read_text())
    result = asyncio.run(heal(request))
    assert result.verification_status == "VERIFIED", result.test_output
    assert result.unified_diff.startswith("--- a/")
    assert "+" in result.unified_diff and "-" in result.unified_diff
    assert result.original_code != result.patched_code
    assert result.confidence_score > 0.5


def test_stack_parser_python_go_race():
    trace = parse_trace('Traceback:\n  File "app.py", line 7, in last\nIndexError: out\n--- FAIL: TestLast (0.0s)\n    app.go:9: bad\nWARNING: DATA RACE')
    assert [(frame.file_path, frame.line) for frame in trace.frames] == [("app.py", 7), ("app.go", 9)]
    assert trace.exception_type == "IndexError"
    assert trace.failing_tests == ["TestLast"]
    assert parse_trace("WARNING: DATA RACE").exception_type == "DataRace"
    assert parse_trace("panic: invalid pointer").exception_type == "panic"


def test_python_ast_slices_helpers_and_complexity():
    context = slice_context('def helper(x):\n    return x\ndef main(x):\n    if x and x > 1:\n        return helper(x)\n    return 0\n', 5)
    assert context.name == "main"
    assert context.helpers == ["helper"]
    assert context.complexity == 3
    assert context.start_line == 3
    with pytest.raises(ValueError):
        slice_context("x = 3\n", 1)


def test_go_ast_slice():
    context = slice_context('package app\nfunc helper(x int) int { return x }\nfunc main() { if helper(1) == 1 { return } }\n', 3, "go")
    assert context.name == "main"
    assert context.helpers == ["helper"]
    assert context.complexity == 2


def test_coverage_formulas():
    assert spectrum(1, 0, 1, 4) == (1.0, 1.0)
    assert spectrum(0, 0, 0, 0) == (0.0, 0.0)
    assert spectrum(1, 1, 2, 2)[1] == 0.5
    with pytest.raises(ValueError):
        spectrum(3, 0, 2, 0)


@pytest.mark.parametrize("path", ["/etc/shadow", "../escape.py", "a/../../escape.py", "a\\b.py", "a\x00.py", ".", "a/"])
def test_reject_traversal(path):
    with pytest.raises(ValidationError):
        HealRequest(logs="", files={path: "x"}, test_command="true")


def test_reject_conflicting_and_oversized_bundles():
    for files in ({"a": "file", "a/b.py": "x"}, {}, {"a.py": "x" * 4_000_001}):
        with pytest.raises(ValidationError):
            HealRequest(logs="", files=files, test_command="true")


def test_never_modifies_test_source():
    request = HealRequest(logs="IndexError", files={"tests/test_app.py": "def last(items):\n return items[len(items)]\nassert last([1]) == 1\n"}, test_command="python3 tests/test_app.py")
    result = asyncio.run(heal(request))
    assert result.verification_status == "REJECTED"


def test_timeout_covers_inherited_pipes():
    code, output = asyncio.run(run_bundle({"app.py": "x = 1"}, "sleep 20 & exit 0", 1))
    assert code == 124


def test_llm_adapter_response_and_validation(monkeypatch):
    from healer.engine import llm_candidate
    monkeypatch.delenv("LLM_API_KEY", raising=False)
    request = HealRequest(logs="bad", files={"app.py": "x=0"}, test_command="false")
    assert llm_candidate(request, "app.py") is None
    monkeypatch.setenv("LLM_API_KEY", "test-key")
    class Response:
        def __enter__(self):
            return self
        def __exit__(self, *args):
            return False
        def read(self, limit):
            return json.dumps({"choices": [{"message": {"content": json.dumps({"patched_code": "x=1"})}}]}).encode()
    monkeypatch.setattr("urllib.request.urlopen", lambda request, timeout: Response())
    candidate = llm_candidate(request, "app.py")
    assert candidate.patched_code == "x=1"


def test_reject_passing_baseline_and_unrepairable():
    result = asyncio.run(heal(HealRequest(logs="", files={"app.py": "x = 1\n"}, test_command="python3 app.py")))
    assert result.verification_status == "REJECTED"
    assert "already passes" in result.root_cause_analysis
    result = asyncio.run(heal(HealRequest(logs="", files={"app.py": "raise RuntimeError('unrepairable')\n"}, test_command="python3 app.py")))
    assert result.verification_status == "REJECTED"


def test_timeout_kills_process_group_and_caps_output():
    code, output = asyncio.run(run_bundle({"app.py": "import time\nprint('start', flush=True)\ntime.sleep(20)\n"}, "python3 app.py", 1))
    assert code == 124 and "timed out" in output and "start" in output
    code, output = asyncio.run(run_bundle({"app.py": "print('x' * 1200000)\n"}, "python3 app.py", 5))
    assert code == 0 and len(output) <= 1_000_000


def test_candidate_cannot_break_regression_tests():
    request = HealRequest(logs="IndexError", files={"app.py": "def last(items):\n    return items[len(items)]\n", "test_app.py": "from app import last\nassert last([2]) == 2\nassert last([]) == 0\n"}, test_command="python3 test_app.py")
    assert asyncio.run(heal(request)).verification_status == "REJECTED"


def test_separate_regression_command_must_pass():
    request = HealRequest(logs="IndexError", files={"app.py": "def last(items):\n    return items[len(items)]\n", "test_app.py": "from app import last\nassert last([2]) == 2\n"}, test_command="python3 test_app.py", regression_command="python3 -c 'raise AssertionError(\"regression\")'")
    result = asyncio.run(heal(request))
    assert result.verification_status == "REJECTED"
    assert "Regression command" in result.test_output
    assert "regression" in result.test_output


def test_default_regression_commands():
    from healer.engine import regression_command
    request = HealRequest(logs="", files={"app.py": "x=1"}, test_command="pytest tests/test_app.py")
    assert regression_command(request) == "python3 -m pytest -q"
    assert regression_command(request.model_copy(update={"language": "go", "test_command": "go test -race -run TestX"})) == "go test -race ./..."


def test_go_mutex_and_error_candidates():
    source = 'package app\nimport "sync"\nvar mu sync.Mutex\nvar count int\nfunc Inc() { count++ }\n'
    assert any("mu.Lock()" in candidate.patched_code for candidate in candidates("a.go", source, "DATA RACE", "go"))
    source = 'package app\nfunc Value() (int, error) { x, _ := read()\nreturn x, nil }\n'
    assert any("return 0, err" in candidate.patched_code for candidate in candidates("a.go", source, "", "go"))


def test_optional_key_and_context_cancellation_rules():
    from healer.engine import candidates
    source='def event_id(event):\n    return event["id"]\n'
    proposal=candidates('app.py',source,"KeyError: 'id'",'python')[0]
    namespace={};exec(proposal.patched_code,namespace)
    assert namespace['event_id']({}) is None and namespace['event_id']({'id':7})==7
    source='package repair\nimport "context"\nfunc Await(ctx context.Context,input <-chan int)(int,error) {\n select {\n case value := <-input:\n return value,nil\n }\n}\n'
    proposal=candidates('await.go',source,'panic: test timed out','go')[0]
    assert 'case <-ctx.Done():' in proposal.patched_code and 'return 0, ctx.Err()' in proposal.patched_code
