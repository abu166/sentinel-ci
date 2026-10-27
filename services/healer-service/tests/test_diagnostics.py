"""Diagnostics are computed from parsed source, never decorative UI estimates."""
import ast
import asyncio
from pathlib import Path
import pytest
from healer.analysis import function_contexts, parse_trace
from healer.engine import candidates, first_ast_change, first_go_change, heal, repair_diagnostics
from healer.analysis import go_tree
from healer.models import HealRequest


def test_python_complexity_and_same_module_helpers_exclude_nested_function_body():
    source = '''def helper(value):
    return value
def choose(values):
    def nested(value):
        if value:
            return value
    return [helper(value) for value in values if value]
async def worker(value):
    return value if value else 0
'''
    functions = {item.name: item for item in function_contexts(source)}
    assert set(functions) == {"helper", "choose", "nested", "worker"}
    assert functions["choose"].complexity == 3
    assert functions["nested"].complexity == 2 and functions["worker"].complexity == 2
    assert functions["choose"].helpers == ["helper"]
    assert all(item.ochiai == 0 for item in functions.values())


def test_go_complexity_uses_branch_nodes_not_comment_or_string_keywords():
    source = '''package app
func helper() int { return 1 }
func decide(x int) int {
    // if for case && || must not affect complexity
    text := "if for case && ||"
    _ = text
    if x > 0 && x < 10 { x = helper() }
    for x < 0 { x++ }
    switch x { case 1: return 1; case 2: return 2; default: return 0 }
}
func typed(v any) { switch v.(type) { case int: return; default: return } }
func receive(ch <-chan int) { select { case <-ch: return; default: return } }
'''
    functions = {item.name: item for item in function_contexts(source, "go")}
    assert functions["decide"].complexity == 6
    assert functions["decide"].helpers == ["helper"]
    assert functions["typed"].complexity == 2 and functions["receive"].complexity == 2


@pytest.mark.parametrize("fixture", ["python_none", "go_nil"])
def test_verified_guard_reports_real_complexity_delta_and_trace_rankings(fixture):
    request = HealRequest.model_validate_json((Path(__file__).parent / "fixtures" / f"{fixture}.json").read_text())
    result = asyncio.run(heal(request))
    assert result.verification_status == "VERIFIED", result.test_output
    assert result.diagnostics["complexity_after"] == result.diagnostics["complexity_before"] + 1
    assert result.diagnostics["score_basis"] == "trace-only; not coverage spectrum"
    assert result.bug_category in result.diagnostics["mutation"]
    assert "→" in result.diagnostics["mutation"]
    assert all(item["ochiai"] in {0, 1} for item in result.diagnostics["rankings"])
    assert result.model_dump()["diagnostics"] == result.diagnostics


@pytest.mark.parametrize("language", ["python", "go"])
def test_rankings_cover_all_functions_and_score_only_trace_observations(language):
    source = "def helper():\n return 1\ndef broken(items):\n return items[len(items)]\n" if language == "python" else "package app\nfunc helper() int { return 1 }\nfunc broken(items []int) int { return items[len(items)] }\n"
    candidate = candidates("app.py" if language == "python" else "app.go", source, "IndexError", language)[0]
    trace = parse_trace('  File "app.py", line 4, in broken\n' if language == "python" else " app.go:3: failure\n")
    diagnostics = repair_diagnostics(candidate, trace, language)
    rankings = {item["name"]: item for item in diagnostics["rankings"]}
    assert rankings["broken"]["ochiai"] == 1 and rankings["broken"]["trace_covered"]
    assert rankings["helper"]["ochiai"] == 0 and not rankings["helper"]["trace_covered"]
    assert diagnostics["complexity_before"] == diagnostics["complexity_after"] == 1


def test_ast_mutation_compares_actual_nodes_and_list_growth():
    assert first_ast_change(ast.parse("value=1"), ast.parse("value='x'")) == "int → str"
    assert "list length" in first_ast_change(ast.parse("value=1"), ast.parse("value=1\nother=2"))
    assert first_ast_change(ast.parse("value=1"), ast.parse("value=1")) == ""
    assert first_go_change(go_tree("package app").root_node, go_tree("package app").root_node) == ""
    assert "child count" in first_go_change(go_tree("package app").root_node, go_tree("package app\nfunc f() {}").root_node)
