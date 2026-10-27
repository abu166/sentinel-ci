"""Full-stack smoke test. Requires compose up; never substitutes fake services."""
import json
import argparse
import time
from simulate import request


def main():
    health = request("/api/v1/health")
    assert health["status"] == "healthy", health
    assert all(health["checks"].values()), health
    repos = request("/api/v1/repos")
    assert len(repos) >= 4, repos
    pipelines = request("/api/v1/pipelines")
    assert len(pipelines) >= 12, len(pipelines)
    result = request(f"/api/v1/pipelines/{pipelines[0]['id']}/simulate-failure", "POST")
    pipeline_id = result.get("pipeline_id", result["id"])
    deadline = time.monotonic() + 120
    detail = {}
    while time.monotonic() < deadline:
        detail = request(f"/api/v1/pipelines/{pipeline_id}")
        if detail.get("patches"):
            break
        time.sleep(1)
    verified = [p for p in detail.get("patches", []) if p["verification_status"] == "VERIFIED"]
    assert verified, detail
    assert detail["logs"], "runner failed to persist output"
    assert "IndexError" in "\n".join(log["content"] for log in detail["logs"]), detail
    approved = request(f"/api/v1/patches/{verified[0]['id']}/apply", "POST")
    rerun = approved.get("pipeline_id", approved["id"])
    assert rerun != pipeline_id, approved
    deadline = time.monotonic() + 120
    while time.monotonic() < deadline:
        detail = request(f"/api/v1/pipelines/{rerun}")
        if detail["status"] in ("PASSED", "FAILED"):
            break
        time.sleep(1)
    assert detail["status"] == "PASSED", detail
    print(json.dumps({"result": "passed", "healed_pipeline": pipeline_id, "verified_rerun": rerun}, indent=2))


def verify_seed_patches():
    patches = request("/api/v1/patches")
    seeded = [patch for patch in patches if patch["test_output"].startswith("Demo history:")]
    assert len(seeded) == 6, seeded
    reruns = []
    for patch in seeded:
        result = request(f"/api/v1/patches/{patch['id']}/apply", "POST")
        reruns.append((patch["file_path"], result["pipeline_id"]))
    deadline = time.monotonic() + 180
    pending = dict(reruns)
    while pending and time.monotonic() < deadline:
        for path, pipeline_id in list(pending.items()):
            detail = request(f"/api/v1/pipelines/{pipeline_id}")
            if detail["status"] == "PASSED":
                del pending[path]
            elif detail["status"] in ("FAILED", "HEALING"):
                raise AssertionError(f"Seeded patch rerun failed: {path}: {detail}")
        if pending:
            time.sleep(1)
    assert not pending, f"Seeded reruns did not complete: {pending}"
    print(json.dumps({"seeded_patch_reruns": "passed", "count": len(reruns)}, indent=2))


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--seed-patches", action="store_true")
    args = parser.parse_args()
    main()
    if args.seed_patches:
        verify_seed_patches()
