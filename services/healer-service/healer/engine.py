"""Deterministic candidates, optional LLM adapter, and bounded process verification.

Temporary directories prevent source contamination. They are not a security boundary:
test commands must come from trusted repositories and run in a restricted container.
"""
import ast
import asyncio
from contextlib import suppress
import difflib
import json
import os
from pathlib import Path
import re
import signal
import tempfile
import urllib.request
from .analysis import parse_trace, slice_context, function_contexts, go_tree
from .models import Candidate, HealRequest, HealResult
from . import sandbox, voting, telemetry
from .redaction import mask_output


def sqlite_parameterization(node: ast.Call) -> tuple[str, ast.expr] | None:
    """Recognize one interpolated SQLite string value, leaving SQL structure static."""
    if not isinstance(node.func, ast.Attribute) or node.func.attr != "execute" or len(node.args) != 1 or node.keywords:
        return None
    expression = node.args[0]
    if not isinstance(expression, ast.BinOp) or not isinstance(expression.op, ast.Add):
        return None
    left, suffix = expression.left, expression.right
    if not isinstance(left, ast.BinOp) or not isinstance(left.op, ast.Add):
        return None
    prefix, value = left.left, left.right
    if not isinstance(prefix, ast.Constant) or not isinstance(prefix.value, str) or not isinstance(suffix, ast.Constant) or not isinstance(suffix.value, str):
        return None
    if not isinstance(value, (ast.Name, ast.Attribute)):
        return None
    if not re.fullmatch(r"\s*SELECT\b.+\bWHERE\b.+(?:=|LIKE)\s*'", prefix.value, re.IGNORECASE | re.DOTALL) or not suffix.value.startswith("'"):
        return None
    query = prefix.value[:-1] + "?" + suffix.value[1:]
    if query.count("?") != 1:
        return None
    return query, value


def candidates(path: str, source: str, logs: str, language: str) -> list[Candidate]:
    variants: list[tuple[str, str, str]] = []
    if language == "python":
        tree = ast.parse(source)
        sqlite_module = any(isinstance(node, ast.Import) and any(alias.name == "sqlite3" for alias in node.names)
                            or isinstance(node, ast.ImportFrom) and node.module == "sqlite3" for node in ast.walk(tree))
        for node in ast.walk(tree):
            if sqlite_module and isinstance(node, ast.Call) and sqlite_parameterization(node):
                copy = ast.parse(source)
                target = next(item for item in ast.walk(copy) if isinstance(item, ast.Call) and item.lineno == node.lineno and item.col_offset == node.col_offset and item.end_lineno == node.end_lineno and item.end_col_offset == node.end_col_offset)
                query, value = sqlite_parameterization(target)
                target.args = [ast.Constant(query), ast.Tuple(elts=[value], ctx=ast.Load())]
                variants.append((ast.unparse(ast.fix_missing_locations(copy)) + "\n", "sql-injection", "A user-controlled value is concatenated into SQL text. Bind it through SQLite's parameter tuple so quotes and SQL operators remain data."))
            if isinstance(node, ast.Subscript) and isinstance(node.value, ast.Name) and isinstance(node.slice, ast.Call) and isinstance(node.slice.func, ast.Name) and node.slice.func.id == "len" and len(node.slice.args) == 1 and ast.dump(node.slice.args[0]) == ast.dump(node.value):
                copy = ast.parse(source)
                target = next(item for item in ast.walk(copy) if isinstance(item, ast.Subscript) and item.lineno == node.lineno and item.col_offset == node.col_offset)
                target.slice = ast.BinOp(left=target.slice, op=ast.Sub(), right=ast.Constant(1))
                variants.append((ast.unparse(ast.fix_missing_locations(copy)) + "\n", "boundary-off-by-one", "Index len(sequence) exceeds the final valid index len(sequence)-1."))
            if isinstance(node, ast.For) and isinstance(node.iter, ast.Call) and isinstance(node.iter.func, ast.Name) and node.iter.func.id == "range" and len(node.iter.args) == 1 and isinstance(node.iter.args[0], ast.BinOp) and isinstance(node.iter.args[0].op, ast.Add) and isinstance(node.iter.args[0].right, ast.Constant) and node.iter.args[0].right.value == 1:
                copy = ast.parse(source)
                target = next(item for item in ast.walk(copy) if isinstance(item, ast.For) and item.lineno == node.lineno)
                target.iter.args[0] = target.iter.args[0].left
                variants.append((ast.unparse(copy) + "\n", "boundary-off-by-one", "Inclusive range bound visits one element beyond the sequence."))
            if isinstance(node, ast.FunctionDef) and node.args.args and "NoneType" in logs:
                copy = ast.parse(source)
                target = next(item for item in ast.walk(copy) if isinstance(item, ast.FunctionDef) and item.lineno == node.lineno)
                arg = target.args.args[0].arg
                guard = ast.If(test=ast.Compare(left=ast.Name(id=arg, ctx=ast.Load()), ops=[ast.Is()], comparators=[ast.Constant(None)]), body=[ast.Return(value=ast.Constant(None))], orelse=[])
                target.body.insert(1 if isinstance(target.body[0], ast.Expr) and isinstance(target.body[0].value, ast.Constant) and isinstance(target.body[0].value.value, str) else 0, guard)
                variants.append((ast.unparse(ast.fix_missing_locations(copy)) + "\n", "none-guard", f"The function dereferences {arg} without a None guard."))
        if "KeyError" in logs:
            for node in ast.walk(tree):
                if isinstance(node, ast.Subscript) and isinstance(node.ctx, ast.Load) and isinstance(node.slice, ast.Constant) and isinstance(node.slice.value, str):
                    copy = ast.parse(source)
                    target = next(item for item in ast.walk(copy) if isinstance(item, ast.Subscript) and item.lineno == node.lineno and item.col_offset == node.col_offset)
                    replacement = ast.Call(func=ast.Attribute(value=target.value, attr="get", ctx=ast.Load()), args=[target.slice], keywords=[])
                    class Replace(ast.NodeTransformer):
                        def visit_Subscript(self, item):
                            return ast.copy_location(replacement, item) if item is target else self.generic_visit(item)
                    variants.append((ast.unparse(ast.fix_missing_locations(Replace().visit(copy))) + "\n", "missing-key-guard", "Optional mapping field must be accessed without raising KeyError."))
        missing = re.search(r"NameError: name '([A-Za-z_]\w*)' is not defined", logs)
        if missing and missing[1] in {"math", "json", "re", "datetime", "collections", "itertools", "statistics"}:
            variants.append((f"import {missing[1]}\n" + source, "missing-import", f"Module {missing[1]} is referenced without an import."))
        if "TypeError" in logs and "str" in logs:
            for node in ast.walk(tree):
                if isinstance(node, ast.BinOp) and isinstance(node.op, ast.Add) and isinstance(node.left, ast.Constant) and isinstance(node.left.value, str):
                    copy = ast.parse(source)
                    target = next(item for item in ast.walk(copy) if isinstance(item, ast.BinOp) and item.lineno == node.lineno and item.col_offset == node.col_offset)
                    target.right = ast.Call(func=ast.Name(id="str", ctx=ast.Load()), args=[target.right], keywords=[])
                    variants.append((ast.unparse(ast.fix_missing_locations(copy)) + "\n", "type-conversion", "String concatenation requires an explicit string conversion."))
    else:
        for match in re.finditer(r'(\w+)\[len\(\1\)\]', source):
            variants.append((source[:match.start()] + f"{match[1]}[len({match[1]})-1]" + source[match.end():], "boundary-off-by-one", "Go slice index len(slice) is outside the valid range."))
        for match in re.finditer(r'<=\s*len\((\w+)\)', source):
            variants.append((source[:match.start()] + "< len(" + match[1] + ")" + source[match.end():], "boundary-off-by-one", "Loop must stop before the slice length."))
        if "DATA RACE" in logs:
            mutex = re.search(r'var\s+(\w+)\s+sync\.(?:RW)?Mutex', source)
            if mutex:
                for match in re.finditer(r'func\s+\w+\([^)]*\)(?:\s+\w+)?\s*\{', source):
                    variants.append((source[:match.end()] + f"\n{mutex[1]}.Lock()\ndefer {mutex[1]}.Unlock()\n" + source[match.end():], "missing-mutex", "Shared state is accessed without holding its declared mutex."))
        for match in re.finditer(r'func\s+\w+\(\s*(\w+)\s+\*\w+\s*\)\s+(\*\w+|int|string|bool)\s*\{', source):
            zero = {"int": "0", "string": '""', "bool": "false"}.get(match[2], "nil")
            variants.append((source[:match.end()] + f"\nif {match[1]} == nil {{ return {zero} }}\n" + source[match.end():], "nil-guard", "Pointer argument is dereferenced without validating nil."))
        for match in re.finditer(r'(\w+),\s*_\s*:=\s*(\w+\([^\n]*\))', source):
            variants.append((source[:match.start()] + f"{match[1]}, err := {match[2]}\nif err != nil {{ return 0, err }}" + source[match.end():], "unhandled-error", "Discarded error must propagate to the caller."))
        if "context.Context" in source and "select" in source and "ctx.Done()" not in source:
            match = re.search(r"select\s*\{", source)
            if match and re.search(r"\(int,\s*error\)", source):
                variants.append((source[:match.end()] + "\ncase <-ctx.Done():\n return 0, ctx.Err()\n" + source[match.end():], "context-cancellation", "The blocking select ignores context cancellation, retaining a goroutine after its deadline."))
    seen: set[str] = set()
    output = []
    for patched, category, cause in variants:
        if patched != source and patched not in seen:
            output.append(Candidate(file_path=path, original_code=source, patched_code=patched, bug_category=category, root_cause_analysis=cause, confidence_score=0.9))
            seen.add(patched)
    return output


async def run_bundle(files: dict[str, str], command: str, timeout: int) -> tuple[int, str]:
    if os.getenv("SANDBOX_MODE", "process") == "docker":
        code, output = await sandbox.run(files, command, timeout)
        return code, mask_output(output)
    with tempfile.TemporaryDirectory(prefix="sentinel-verify-") as directory:
        root = Path(directory)
        for name, content in files.items():
            path = root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(content)
        environment = {key: value for key, value in os.environ.items() if key in {"PATH", "SYSTEMROOT", "TMPDIR", "LANG"}}
        environment.update({"HOME": directory, "GOCACHE": str(root / ".gocache"), "GOPATH": str(root / ".gopath"), "GOTOOLCHAIN": "local", "GOPROXY": "off", "GOMAXPROCS": "2", "GOFLAGS": "-p=2", "PYTHONDONTWRITEBYTECODE": "1"})
        process = await asyncio.create_subprocess_exec("/bin/sh", "-c", command, cwd=directory, env=environment, stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.STDOUT, start_new_session=True)
        collected = bytearray()
        async def collect() -> None:
            assert process.stdout is not None
            while data := await process.stdout.read(8192):
                if len(collected) < 1_000_000:
                    collected.extend(data[:1_000_000 - len(collected)])
        reader = asyncio.create_task(collect())
        async def completed() -> None:
            await process.wait()
            await reader
        try:
            await asyncio.wait_for(completed(), timeout)
        except TimeoutError:
            with suppress(ProcessLookupError):
                os.killpg(process.pid, signal.SIGKILL)
            await process.wait()
            await asyncio.gather(reader, return_exceptions=True)
            return 124, mask_output(collected.decode(errors="replace") + "\nVerification timed out")
        except asyncio.CancelledError:
            with suppress(ProcessLookupError):
                os.killpg(process.pid, signal.SIGKILL)
            await process.wait()
            await asyncio.gather(reader, return_exceptions=True)
            raise
        return process.returncode or 0, mask_output(collected.decode(errors="replace"))


def llm_candidate(request: HealRequest, path: str) -> Candidate | None:
    key = os.getenv("LLM_API_KEY")
    if not key:
        return None
    payload = {"model": os.getenv("LLM_MODEL", "gpt-4.1-mini"), "messages": [{"role": "system", "content": "Return JSON with patched_code only. Repair the supplied source using the failing test logs; preserve behavior."}, {"role": "user", "content": json.dumps({"source": request.files[path], "logs": request.logs})}], "response_format": {"type": "json_object"}}
    http_request = urllib.request.Request(os.getenv("LLM_API_URL", "https://api.openai.com/v1/chat/completions"), data=json.dumps(payload).encode(), headers={"Authorization": f"Bearer {key}", "Content-Type": "application/json"})
    with urllib.request.urlopen(http_request, timeout=30) as response:
        content = json.loads(response.read(2_000_000))["choices"][0]["message"]["content"]
    patched = json.loads(content)["patched_code"]
    if not isinstance(patched, str) or len(patched) > 1_000_000:
        raise ValueError("invalid LLM source response")
    return Candidate(file_path=path, original_code=request.files[path], patched_code=patched, bug_category="llm-assisted", root_cause_analysis="Optional model-generated candidate accepted only after full command verification.", confidence_score=0.7)


def regression_command(request: HealRequest) -> str:
    if request.regression_command:
        return request.regression_command
    if request.language == "go":
        return "go test -race ./..." if "-race" in request.test_command else "go test ./..."
    return "python3 -m pytest -q" if "pytest" in request.test_command else "python3 -m unittest discover -v"


def trace_matches(frame_path: str, source_path: str) -> bool:
    normalized = frame_path.replace("\\", "/")
    return normalized == source_path or normalized.endswith("/" + source_path)


def first_ast_change(before, after) -> str:
    if type(before) is not type(after):
        return f"{type(before).__name__} → {type(after).__name__}"
    if isinstance(before, ast.AST):
        for field, value in ast.iter_fields(before):
            change = first_ast_change(value, getattr(after, field))
            if change:
                return change
    elif isinstance(before, list):
        for left, right in zip(before, after):
            change = first_ast_change(left, right)
            if change:
                return change
        if len(before) != len(after):
            return f"AST node list length {len(before)} → {len(after)}"
    elif before != after:
        return f"{before!r} → {after!r}"
    return ""


def first_go_change(before, after) -> str:
    if before.type != after.type:
        return f"{before.type} → {after.type}"
    if not before.children and before.text != after.text:
        return f"{before.type} {before.text.decode()!r} → {after.text.decode()!r}"
    for left, right in zip(before.children, after.children):
        change = first_go_change(left, right)
        if change:
            return change
    if len(before.children) != len(after.children):
        return f"{before.type} child count {len(before.children)} → {len(after.children)}"
    return ""


def repair_diagnostics(candidate: Candidate, trace, language: str) -> dict:
    before = function_contexts(candidate.original_code, language)
    after = function_contexts(candidate.patched_code, language)
    trace_lines = [frame.line for frame in trace.frames if trace_matches(frame.file_path, candidate.file_path)]
    covered = [context for context in before if any(context.start_line <= line <= context.end_line for line in trace_lines)]
    changes = difflib.SequenceMatcher(a=candidate.original_code.splitlines(), b=candidate.patched_code.splitlines()).get_opcodes()
    changed_lines = [start + 1 for kind, start, _, _, _ in changes if kind != "equal"]
    changed = [context for context in before if any(context.start_line <= line <= context.end_line for line in changed_lines)]
    selected = min(covered or changed or before, key=lambda context: context.end_line - context.start_line) if before else None
    matching = next((context for context in after if selected and context.name == selected.name), None)
    if language == "go":
        mutation = first_go_change(go_tree(candidate.original_code).root_node, go_tree(candidate.patched_code).root_node)
    else:
        mutation = first_ast_change(ast.parse(candidate.original_code), ast.parse(candidate.patched_code))
    diagnostics = {
        "mutation": f"{candidate.bug_category}: {mutation}. {candidate.root_cause_analysis}",
        "score_basis": "trace-only; not coverage spectrum",
        "rankings": [{"name": context.name, "ochiai": 1.0 if context in covered else 0.0,
                      "complexity": context.complexity, "start_line": context.start_line, "end_line": context.end_line,
                      "trace_covered": context in covered} for context in sorted(before, key=lambda item: (item not in covered, item.start_line))],
    }
    node_ids = {(context.name, context.start_line): f"{candidate.file_path}:{context.start_line}:{context.name}" for context in before}
    helper_ids = {context.name: node_ids[(context.name, context.start_line)] for context in before
                  if sum(other.name == context.name for other in before) == 1}
    diagnostics["call_graph"] = {
        "nodes": [{"id": node_ids[(context.name, context.start_line)], "name": context.name, "file_path": candidate.file_path,
                   "start_line": context.start_line, "end_line": context.end_line, "complexity": context.complexity,
                   "suspiciousness": 1.0 if context in covered else 0.0} for context in before],
        "edges": [{"source": node_ids[(context.name, context.start_line)], "target": helper_ids[helper], "kind": "calls"}
                  for context in before for helper in context.helpers if helper in helper_ids],
        "root_id": node_ids[(selected.name, selected.start_line)] if selected else None,
        "provenance": "Go tree-sitter" if language == "go" else "Python ast",
    }
    if selected and matching:
        diagnostics["complexity_before"] = selected.complexity
        diagnostics["complexity_after"] = matching.complexity
        diagnostics["function"] = selected.name
    return diagnostics


async def _heal(request: HealRequest) -> HealResult:
    base = dict(pipeline_id=request.pipeline_id, step_id=request.step_id)
    baseline_code, baseline_output = await voting.run_limited(run_bundle, request.files, request.test_command, request.timeout_seconds)
    baseline_output = mask_output(baseline_output)
    if baseline_code == 0:
        return HealResult(**base, verification_status="REJECTED", root_cause_analysis="Baseline already passes; repair is not justified.", test_output=baseline_output)
    trace = parse_trace(request.logs + "\n" + baseline_output)
    paths = [name for name in request.files if name.endswith(".py" if request.language == "python" else ".go") and not Path(name).name.startswith("test_") and not name.endswith(("_test.go", "_test.py")) and not {"tests", "test"}.intersection(Path(name).parts)]
    paths.sort(key=lambda path: not any(trace_matches(frame.file_path, path) for frame in trace.frames))
    proposals = []
    summaries = {}
    last_output = baseline_output
    for path in paths[:20]:
        source = request.files[path]
        frame = next((item for item in reversed(trace.frames) if trace_matches(item.file_path, path)), None)
        try:
            proposals.extend(candidates(path, source, request.logs + baseline_output, request.language))
            contexts = function_contexts(source, request.language)
            if frame:
                summaries[path] = slice_context(source, frame.line, request.language).model_dump_json()
            elif contexts:
                summaries[path] = contexts[0].model_dump_json()
        except (SyntaxError, ValueError) as error:
            summaries[path] = str(error)
        if os.getenv("LLM_API_KEY"):
            try:
                proposal = await asyncio.to_thread(llm_candidate, request, path)
                if proposal:
                    proposals.append(proposal)
            except (OSError, ValueError, KeyError, TypeError) as error:
                last_output += f"\nLLM candidate unavailable: {type(error).__name__}"
        if len(proposals) >= 3:
            break
    proposals = await asyncio.to_thread(voting.diverse, proposals, request.language)
    evidence = await voting.verify(proposals, request, run_bundle, regression_command(request), lambda candidate: repair_diagnostics(candidate, trace, request.language))
    for item in evidence:
        item.diagnostics["ast_node_summary"] = summaries.get(item.file_path, "")
    winner = next((item for item in evidence if item.selected), None)
    if winner:
        candidate = next(item for item in proposals if item.file_path == winner.file_path and item.patched_code == winner.patched_code)
        return HealResult(**base, **candidate.model_dump(), verification_status="VERIFIED", unified_diff=winner.unified_diff,
                          ast_node_summary=summaries.get(winner.file_path, ""), test_output=winner.test_output,
                          diagnostics=winner.diagnostics, candidates=evidence, selected_candidate_id=winner.id)
    return HealResult(**base, verification_status="REJECTED", root_cause_analysis="No candidate passed the complete verification command.",
                      test_output="\n\n".join(item.test_output for item in evidence) or last_output, candidates=evidence)


async def heal(request: HealRequest) -> HealResult:
    try:
        async with asyncio.timeout(300):
            with telemetry.tracer.start_as_current_span("healer.repair", attributes={"pipeline.id": request.pipeline_id or "direct", "step.id": request.step_id or "direct"}):
                return await _heal(request)
    except TimeoutError:
        return HealResult(pipeline_id=request.pipeline_id, step_id=request.step_id, verification_status="REJECTED", root_cause_analysis="Overall five-minute repair budget exhausted; no patch accepted.")
