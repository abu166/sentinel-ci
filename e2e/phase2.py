"""Exercise real isolated repairs, approval publication, auth/RBAC and outbox dispatch."""
import json
import re
import time
import urllib.error
from simulate import login, request


def wait_detail(pipeline_id, predicate, timeout=600):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        detail = request(f"/api/v1/pipelines/{pipeline_id}")
        if predicate(detail):
            return detail
        if detail["status"] == "FAILED":
            raise AssertionError(json.dumps(detail, default=str))
        time.sleep(2)
    raise AssertionError(f"Pipeline timed out: {pipeline_id}: {detail}")


def main(selected_scenarios=None, require_diagnostics=False):
    admin = login()
    assert request("/api/v1/health")["status"] == "healthy"
    try:
        request("/api/v1/repos", token="")
    except urllib.error.HTTPError as error:
        assert error.code == 401
    else:
        raise AssertionError("anonymous registry access permitted")
    viewer = login("viewer")
    source = request("/api/v1/pipelines", token=admin)[0]["id"]
    try:
        request(f"/api/v1/pipelines/{source}/simulate-failure", "POST", {"scenario": "off-by-one"}, token=viewer)
    except urllib.error.HTTPError as error:
        assert error.code == 403
    else:
        raise AssertionError("viewer pipeline write permitted")
    login()
    catalog = request("/api/v1/simulations")
    assert len(catalog) >= 5
    scenarios = selected_scenarios or ["nil-pointer", "off-by-one", "data-race", "key-error", "context-timeout"]
    # Bound concurrency to avoid overwhelming local cold Go compilers.
    results = []
    for scenario in scenarios:
        started = time.monotonic()
        job = request(f"/api/v1/pipelines/{source}/simulate-failure", "POST", {"scenario": scenario})
        pipeline_id = job["pipeline_id"]
        detail = wait_detail(pipeline_id, lambda d: any(p["verification_status"] == "VERIFIED" for p in d.get("patches", [])))
        assert re.fullmatch(r"[0-9a-f]{7,40}", detail["commit_sha"]), detail
        patch = next(p for p in detail["patches"] if p["verification_status"] == "VERIFIED")
        if require_diagnostics:
            diagnostic = patch.get("diagnostics", {})
            assert diagnostic.get("rankings"), diagnostic
            assert isinstance(diagnostic.get("complexity_before"), int), diagnostic
            assert isinstance(diagnostic.get("complexity_after"), int), diagnostic
            assert diagnostic.get("mutation"), diagnostic
        executed = [s for s in detail["steps"] if s.get("exit_code") is not None]
        assert executed and all(s["sandbox_policy"].get("isolated") for s in executed), executed
        assert all(s["sandbox_policy"].get("network") == "none" for s in executed), executed
        approved = request(f"/api/v1/patches/{patch['id']}/apply", "POST")
        rerun = wait_detail(approved["pipeline_id"], lambda d: d["status"] == "PASSED")
        deadline = time.monotonic() + 120
        publication = None
        while time.monotonic() < deadline:
            publications = request("/api/v1/pulls")
            publication = next((p for p in publications if p["patch_id"] == patch["id"]), None)
            if publication:
                break
            time.sleep(1)
        assert publication, f"Git publication missing for {scenario}"
        assert re.fullmatch(r"[0-9a-f]{40}", publication["commit_sha"]), publication
        assert publication["branch"].startswith("sentinel/fix-"), publication
        assert "+" in publication["diff"], publication
        item = {"scenario": scenario, "pipeline_id": pipeline_id, "rerun_id": rerun["id"], "pull_id": publication["id"], "git_commit": publication["commit_sha"], "elapsed_seconds": round(time.monotonic()-started, 2)}
        results.append(item)
        print(json.dumps(item), flush=True)
    audit = request("/api/v1/audit-logs")
    assert audit, "audit trail missing"
    assert request("/api/v1/workers"), "worker telemetry missing"
    assert request("/api/v1/queue"), "queue telemetry missing"
    print(json.dumps({"result": "passed", "scenarios": results}, indent=2))


if __name__ == "__main__":
    import argparse
    parser=argparse.ArgumentParser()
    parser.add_argument('--security-only',action='store_true')
    parser.add_argument('--extras',action='store_true')
    parser.add_argument('--diagnostics',action='store_true')
    options=parser.parse_args()
    if options.security_only:
        from security_probe import main as security_main
        security_main()
    elif options.diagnostics:
        main(["off-by-one"], require_diagnostics=True)
    elif options.extras:
        main(["sql-injection","deadlock"])
    else:
        main()
