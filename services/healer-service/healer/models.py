from pathlib import PurePosixPath
from typing import Literal
from typing import Any
from pydantic import BaseModel, Field, field_validator


class HealRequest(BaseModel):
    pipeline_id: str | None = None
    step_id: str | None = None
    logs: str = Field(max_length=2_000_000)
    files: dict[str, str]
    test_command: str = Field(min_length=1, max_length=4096)
    regression_command: str | None = Field(default=None, min_length=1, max_length=4096)
    language: Literal["python", "go"] = "python"
    timeout_seconds: int = Field(default=30, ge=1, le=120)

    @field_validator("files")
    @classmethod
    def safe_files(cls, files: dict[str, str]) -> dict[str, str]:
        if not files or len(files) > 100:
            raise ValueError("between 1 and 100 files required")
        if sum(len(content) for content in files.values()) > 4_000_000:
            raise ValueError("source bundle exceeds 4 MB")
        for name in files:
            path = PurePosixPath(name)
            if path.is_absolute() or ".." in path.parts or "\\" in name or not name or "\x00" in name or str(path) == "." or name.endswith("/"):
                raise ValueError("file paths must be safe relative paths")
            if any(str(parent) in files for parent in path.parents if str(parent) != "."):
                raise ValueError("a source path cannot be both a file and directory")
        return files


class Frame(BaseModel):
    file_path: str
    line: int
    function: str = ""


class Trace(BaseModel):
    frames: list[Frame]
    exception_type: str
    failing_tests: list[str]


class Context(BaseModel):
    name: str
    source: str
    start_line: int
    end_line: int
    helpers: list[str]
    complexity: int
    ochiai: float
    tarantula: float


class Candidate(BaseModel):
    variant: str = "deterministic"
    file_path: str
    original_code: str
    patched_code: str
    bug_category: str
    root_cause_analysis: str
    confidence_score: float


class CandidateEvidence(BaseModel):
    id: str
    rank: int
    category: str
    variant: str
    file_path: str
    original_code: str
    patched_code: str
    unified_diff: str
    ast_cost: int = Field(ge=0)
    test_success: bool
    score: float = Field(ge=0, le=1)
    test_output: str
    status: Literal["VERIFIED", "REJECTED"]
    selected: bool
    diagnostics: dict[str, Any] = Field(default_factory=dict)


class HealResult(BaseModel):
    pipeline_id: str | None = None
    step_id: str | None = None
    patch_id: str | None = None
    verification_status: Literal["VERIFIED", "REJECTED"]
    file_path: str = ""
    bug_category: str = "unsupported"
    root_cause_analysis: str
    ast_node_summary: str = ""
    original_code: str = ""
    patched_code: str = ""
    unified_diff: str = ""
    confidence_score: float = 0
    test_output: str = ""
    diagnostics: dict[str, Any] = Field(default_factory=dict)
    candidates: list[CandidateEvidence] = Field(default_factory=list)
    selected_candidate_id: str | None = None
