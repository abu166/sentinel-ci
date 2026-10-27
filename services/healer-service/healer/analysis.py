"""Stack trace extraction and language-aware function slicing."""
import ast
import math
import re
from .models import Context, Frame, Trace


def parse_trace(logs: str) -> Trace:
    frames = [Frame(file_path=m[0], line=int(m[1]), function=m[2]) for m in re.findall(r'File "([^"]+)", line (\d+)(?:, in ([^\n]+))?', logs)]
    frames.extend(Frame(file_path=m[0], line=int(m[1])) for m in re.findall(r'(?m)^\s*([^\s:]+\.go):(\d+)(?::\d+)?', logs))
    frames.extend(Frame(file_path=m[0], line=int(m[1])) for m in re.findall(r'(?m)^([^\s:]+\.py):(\d+):', logs))
    errors = re.findall(r'(?m)^(?:E\s+)?([\w.]*(?:Error|Exception))(?::|\s|$)', logs)
    exception = errors[-1] if errors else ("DataRace" if "DATA RACE" in logs else "panic" if "panic:" in logs else "TestFailure")
    tests = re.findall(r'(?:--- FAIL: |FAIL:\s*|FAILED\s+)([^\s(]+)', logs)
    return Trace(frames=frames, exception_type=exception, failing_tests=list(dict.fromkeys(tests)))


def spectrum(failed_covered: int, passed_covered: int, total_failed: int, total_passed: int) -> tuple[float, float]:
    if min(failed_covered, passed_covered, total_failed, total_passed) < 0 or failed_covered > total_failed or passed_covered > total_passed:
        raise ValueError("invalid coverage spectrum")
    denominator = math.sqrt(total_failed * (failed_covered + passed_covered))
    ochiai = failed_covered / denominator if denominator else 0.0
    failed_ratio = failed_covered / total_failed if total_failed else 0.0
    passed_ratio = passed_covered / total_passed if total_passed else 0.0
    tarantula = failed_ratio / (failed_ratio + passed_ratio) if failed_ratio + passed_ratio else 0.0
    return ochiai, tarantula


def go_tree(source: str):
    import tree_sitter_go
    from tree_sitter import Language, Parser
    return Parser(Language(tree_sitter_go.language())).parse(source.encode())


def descendants(node):
    yield node
    for child in node.children:
        yield from descendants(child)


class PythonFunctionVisitor(ast.NodeVisitor):
    def __init__(self):
        self.complexity = 1
        self.calls: set[str] = set()

    def visit_FunctionDef(self, node):
        return None

    visit_AsyncFunctionDef = visit_FunctionDef
    visit_Lambda = visit_FunctionDef

    def generic_visit(self, node):
        if isinstance(node, (ast.If, ast.For, ast.AsyncFor, ast.While, ast.IfExp, ast.ExceptHandler)):
            self.complexity += 1
        elif isinstance(node, ast.comprehension):
            self.complexity += 1 + len(node.ifs)
        elif isinstance(node, ast.BoolOp):
            self.complexity += len(node.values) - 1
        elif isinstance(node, ast.match_case) and not isinstance(node.pattern, ast.MatchAs):
            self.complexity += 1
        if isinstance(node, ast.Call) and isinstance(node.func, ast.Name):
            self.calls.add(node.func.id)
        super().generic_visit(node)


def function_contexts(source: str, language: str = "python") -> list[Context]:
    """Enumerate parsed functions; unobserved functions receive no trace evidence."""
    lines = source.splitlines()
    contexts = []
    if language == "go":
        tree = go_tree(source)
        functions = [node for node in tree.root_node.children if node.type in {"function_declaration", "method_declaration"}]
        names = {node.child_by_field_name("name").text.decode() for node in functions if node.child_by_field_name("name") is not None}
        for node in functions:
            name_node = node.child_by_field_name("name")
            name = name_node.text.decode() if name_node else "anonymous"
            called = set()
            complexity = 1
            for child in descendants(node):
                if child.type in {"if_statement", "for_statement", "expression_case", "type_case", "communication_case"}:
                    complexity += 1
                if child.type == "binary_expression":
                    operator = child.child_by_field_name("operator")
                    if operator is not None and operator.text in {b"&&", b"||"}:
                        complexity += 1
                if child.type == "call_expression":
                    function = child.child_by_field_name("function")
                    if function is not None and function.type == "identifier":
                        called.add(function.text.decode())
            contexts.append(Context(name=name, source=source.encode()[node.start_byte:node.end_byte].decode(),
                                    start_line=node.start_point.row + 1, end_line=node.end_point.row + 1,
                                    helpers=sorted((called & names) - {name}), complexity=complexity, ochiai=0, tarantula=0))
        return contexts
    tree = ast.parse(source)
    functions = [node for node in ast.walk(tree) if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef))]
    names = {node.name for node in functions}
    for node in functions:
        visitor = PythonFunctionVisitor()
        for statement in node.body:
            visitor.visit(statement)
        contexts.append(Context(name=node.name, source="\n".join(lines[node.lineno - 1:node.end_lineno]), start_line=node.lineno,
                                end_line=node.end_lineno or node.lineno, helpers=sorted((visitor.calls & names) - {node.name}),
                                complexity=visitor.complexity, ochiai=0, tarantula=0))
    return contexts


def slice_context(source: str, line: int, language: str = "python") -> Context:
    matches = [context for context in function_contexts(source, language) if context.start_line <= line <= context.end_line]
    if not matches:
        raise ValueError(f"no {'Go' if language == 'go' else 'Python'} function at failing line")
    selected = min(matches, key=lambda context: context.end_line - context.start_line)
    return selected.model_copy(update={"ochiai": 1.0, "tarantula": 1.0})
