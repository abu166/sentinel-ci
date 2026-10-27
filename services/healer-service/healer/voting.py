"""Diverse proposals, bounded parallel verification and evidence-based ranking."""
import ast
import asyncio
import difflib
import os
import re
import subprocess
from uuid import NAMESPACE_URL, uuid5
from weakref import WeakKeyDictionary
from .analysis import go_tree, descendants, function_contexts
from .models import Candidate, CandidateEvidence, HealRequest
from . import telemetry
from .redaction import mask_output

_limits: WeakKeyDictionary = WeakKeyDictionary()


def concurrency_limit() -> int:
    value = int(os.getenv("HEALER_CANDIDATE_CONCURRENCY", "3"))
    if not 1 <= value <= 8:
        raise ValueError("HEALER_CANDIDATE_CONCURRENCY must be 1..8")
    return value


async def run_limited(executor, files: dict[str, str], command: str, timeout: int):
    """The process-wide event loop limits baseline and candidate sandboxes alike."""
    loop = asyncio.get_running_loop()
    if loop not in _limits:
        _limits[loop] = asyncio.Semaphore(concurrency_limit())
    async with _limits[loop]:
        with telemetry.tracer.start_as_current_span("healer.sandbox.run", attributes={"sandbox.mode": os.getenv("SANDBOX_MODE", "process"), "sandbox.timeout_seconds": timeout}):
            return await executor(files, command, timeout)


def diverse(proposals: list[Candidate], language: str) -> list[Candidate]:
    """Add genuinely different transformations only where a repair rule supports them."""
    variants = list(proposals)
    for proposal in proposals:
        source = proposal.original_code
        if language == "python":
            try:
                tree = ast.parse(source)
            except SyntaxError:
                continue
            for node in ast.walk(tree):
                if proposal.bug_category == "boundary-off-by-one" and isinstance(node, ast.Subscript) and isinstance(node.value, ast.Name) and isinstance(node.slice, ast.Call) and isinstance(node.slice.func, ast.Name) and node.slice.func.id == "len" and len(node.slice.args) == 1 and ast.dump(node.slice.args[0]) == ast.dump(node.value):
                    replacements = [("negative-index", f"{node.value.id}[-1]"), ("guarded-last-element", f"{node.value.id}[-1] if {node.value.id} else None")]
                elif proposal.bug_category == "missing-key-guard" and isinstance(node, ast.Subscript) and isinstance(node.ctx, ast.Load) and isinstance(node.slice, ast.Constant) and isinstance(node.slice.value, str):
                    value, key = ast.unparse(node.value), ast.unparse(node.slice)
                    replacements = [("membership-guard", f"{value}[{key}] if {key} in {value} else None")]
                    copy = ast.parse(source)
                    class CatchKey(ast.NodeTransformer):
                        def visit_Return(self, item):
                            if isinstance(item.value, ast.Subscript) and item.value.lineno == node.lineno and item.value.col_offset == node.col_offset:
                                return ast.copy_location(ast.Try(body=[item], handlers=[ast.ExceptHandler(type=ast.Name(id="KeyError", ctx=ast.Load()), name=None, body=[ast.Return(value=ast.Constant(None))])], orelse=[], finalbody=[]), item)
                            return self.generic_visit(item)
                    patched = ast.unparse(ast.fix_missing_locations(CatchKey().visit(copy))) + "\n"
                    variants.append(proposal.model_copy(update={"patched_code": patched, "variant": "exception-boundary"}))
                else:
                    continue
                for label, expression in replacements:
                    copy = ast.parse(source)
                    class Replace(ast.NodeTransformer):
                        def visit_Subscript(self, item):
                            if item.lineno == node.lineno and item.col_offset == node.col_offset:
                                return ast.copy_location(ast.parse(expression, mode="eval").body, item)
                            return self.generic_visit(item)
                    patched = ast.unparse(ast.fix_missing_locations(Replace().visit(copy))) + "\n"
                    variants.append(proposal.model_copy(update={"patched_code": patched, "variant": label}))
        elif proposal.bug_category == "nil-guard":
            match = re.search(r'func\s+\w+\(\s*(\w+)\s+\*(\w+)\s*\)\s+(int|string|bool)\s*\{', source)
            if match:
                arg, kind = match[1], match[2]
                body = source[match.end():]
                end = body.find("}")
                if end >= 0 and re.fullmatch(r"\s*return\s+(?:\*\w+|\w+\.\w+)\s*", body[:end]):
                    original_body = body[:end]
                    zero = {"int": "0", "string": '""', "bool": "false"}[match[3]]
                    alternatives = [("zero-value-initialization", f"\nif {arg} == nil {{ {arg} = new({kind}) }}\n" + original_body), ("positive-pointer-guard", f"\nif {arg} != nil {{ {original_body} }}\nreturn {zero}\n")]
                    for label, replacement in alternatives:
                        variants.append(proposal.model_copy(update={"patched_code": source[:match.end()] + replacement + body[end:], "variant": label}))
        elif proposal.bug_category == "missing-mutex":
            # Both forms serialize the same protected function, but have different AST edits.
            patched = re.sub(r'defer (\w+)\.Unlock\(\)', '', proposal.patched_code, count=1)
            mutex = re.search(r'(\w+)\.Lock\(\)', patched)
            if mutex:
                end = patched.rfind('}')
                if end >= 0:
                    patched = patched[:end] + f"\n{mutex[1]}.Unlock()\n" + patched[end:]
                    variants.append(proposal.model_copy(update={"patched_code": patched, "variant": "explicit-unlock"}))
    unique = []
    seen = set()
    for candidate in variants:
        if language == "go":
            try:
                formatted = subprocess.run(["gofmt"], input=candidate.patched_code, text=True, capture_output=True, timeout=5, check=False)
            except (OSError, subprocess.TimeoutExpired) as error:
                raise RuntimeError("Go formatter unavailable or timed out") from error
            if formatted.returncode:
                continue
            candidate = candidate.model_copy(update={"patched_code": formatted.stdout})
        key = (candidate.file_path, candidate.patched_code)
        if key not in seen and candidate.patched_code != candidate.original_code:
            unique.append(candidate)
            seen.add(key)
    return unique[:3]


def ast_cost(original: str, patched: str, language: str) -> int:
    if language == "python":
        def nodes(source):
            return [type(node).__name__ + ":" + repr([(name, value) for name, value in ast.iter_fields(node) if not isinstance(value, (ast.AST, list))]) for node in ast.walk(ast.parse(source))]
    else:
        def nodes(source):
            return [node.type + (":" + node.text.decode() if not node.children else "") for node in descendants(go_tree(source).root_node) if node.type and node.type != "comment"]
    return sum(max(end_a-start_a, end_b-start_b) for kind, start_a, end_a, start_b, end_b in difflib.SequenceMatcher(a=nodes(original), b=nodes(patched), autojunk=False).get_opcodes() if kind != "equal")


def complexity_delta(candidate: Candidate, language: str) -> int:
    before = sum(item.complexity for item in function_contexts(candidate.original_code, language))
    after = sum(item.complexity for item in function_contexts(candidate.patched_code, language))
    return max(0, after - before)


async def verify(proposals: list[Candidate], request: HealRequest, executor, regression: str, diagnostics) -> list[CandidateEvidence]:
    request_limit = asyncio.Semaphore(3)
    async def evaluate(index: int, candidate: Candidate):
        with telemetry.tracer.start_as_current_span("healer.candidate.verify", attributes={"candidate.variant": candidate.variant, "candidate.category": candidate.bug_category}) as span:
            item = await evaluate_candidate(index, candidate)
            span.set_attribute("candidate.proposal_id", item.id)
            span.set_attribute("candidate.test_success", item.test_success)
            context = span.get_span_context()
            if context.is_valid:
                item.diagnostics["trace_id"] = format(context.trace_id, "032x")
                item.diagnostics["span_id"] = format(context.span_id, "016x")
            return item
    async def evaluate_candidate(index: int, candidate: Candidate):
        identity = str(uuid5(NAMESPACE_URL, f"{request.pipeline_id}:{request.step_id}:{candidate.file_path}:{candidate.patched_code}"))
        diff = "".join(difflib.unified_diff(candidate.original_code.splitlines(True), candidate.patched_code.splitlines(True), fromfile="a/" + candidate.file_path, tofile="b/" + candidate.file_path))
        try:
            cost = ast_cost(candidate.original_code, candidate.patched_code, request.language)
        except (SyntaxError, ValueError):
            cost = 1_000_000
        async with request_limit:
            bundle = {**request.files, candidate.file_path: candidate.patched_code}
            try:
                code, output = await run_limited(executor, bundle, request.test_command, request.timeout_seconds)
                output = f"Failing-test command: {request.test_command}\n{output}"
                if code == 0:
                    code, regression_output = await run_limited(executor, bundle, regression, request.timeout_seconds)
                    output += f"\nRegression command: {regression}\n{regression_output}"
            except (OSError, RuntimeError) as error:
                code, output = 1, f"Sandbox execution failed: {type(error).__name__}: {error}"
        output = mask_output(output)
        passed = code == 0 and cost != 1_000_000
        try:
            detail = diagnostics(candidate)
            delta = complexity_delta(candidate, request.language)
        except (SyntaxError, ValueError) as error:
            detail, delta = {"parser_error": str(error)}, 0
        detail["root_cause_analysis"] = candidate.root_cause_analysis
        detail["complexity_delta"] = delta
        return CandidateEvidence(id=identity, rank=index + 1, category=candidate.bug_category, variant=candidate.variant, file_path=candidate.file_path, original_code=candidate.original_code, patched_code=candidate.patched_code, unified_diff=diff, ast_cost=cost, test_success=passed, score=round(0.8 + 0.2 / (1 + cost), 8) if passed else 0, test_output=output, status="VERIFIED" if passed else "REJECTED", selected=False, diagnostics=detail)
    evidence = await asyncio.gather(*(evaluate(index, candidate) for index, candidate in enumerate(proposals[:3])))
    evidence.sort(key=lambda item: (not item.test_success, item.ast_cost, item.diagnostics["complexity_delta"], item.id))
    for index, item in enumerate(evidence):
        item.rank = index + 1
        item.selected = index == 0 and item.test_success
    winner = next((item for item in evidence if item.selected), None)
    if winner:
        with telemetry.tracer.start_as_current_span("healer.candidate.select", attributes={"candidate.id": winner.id, "candidate.ast_cost": winner.ast_cost, "candidate.score": winner.score}):
            return evidence
    return evidence
