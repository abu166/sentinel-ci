"""Live Phase 3 proof: real exported traces, candidate voting and pending selection.

Run after ``docker compose up --build --wait``. No patch is approved/published.
This gate deliberately fails if Jaeger has IDs without the required exported SDK spans.
"""
import argparse
import json
import os
import secrets
import time
import urllib.request
import urllib.error

from simulate import BASE, login, request
from phase2 import wait_detail


def trigger(source, trace_id):
    token = login()
    payload = json.dumps({"scenario": "off-by-one"}).encode()
    req = urllib.request.Request(BASE + f"/api/v1/pipelines/{source}/simulate-failure", data=payload,
        headers={"Content-Type": "application/json", "Authorization": "Bearer " + token,
                 "traceparent": f"00-{trace_id}-{secrets.token_hex(8)}-01"}, method="POST")
    with urllib.request.urlopen(req, timeout=30) as response:
        return json.load(response)


def normalize_trace(payload):
    """Jaeger stable API v3 returns OTLP resourceSpans, not legacy UI JSON."""
    result = payload.get("result", payload)
    spans, processes = [], {}
    for index, resource in enumerate(result.get("resourceSpans", [])):
        process = str(index)
        attrs = resource.get("resource", {}).get("attributes", [])
        service = next((a.get("value", {}).get("stringValue") for a in attrs if a.get("key") == "service.name"), "unreported")
        processes[process] = {"serviceName": service}
        for scope in resource.get("scopeSpans", []):
            for span in scope.get("spans", []):
                spans.append({"spanID": span["spanId"], "operationName": span["name"], "processID": process,
                              "references": [{"spanID": span["parentSpanId"], "refType": "CHILD_OF"}] if span.get("parentSpanId") else []})
    return {"spans": spans, "processes": processes}


def exported_trace(trace_id, timeout=60):
    deadline = time.monotonic() + timeout
    last = {}
    while time.monotonic() < deadline:
        try:
            with urllib.request.urlopen(os.getenv("JAEGER_URL", "http://localhost:16686") + "/api/v3/traces/" + trace_id, timeout=10) as response:
                raw = response.read().decode()
            # Streaming HTTP API may emit multiple JSON objects on separate lines.
            combined = {"result": {"resourceSpans": []}}
            for line in raw.splitlines():
                payload = json.loads(line)
                combined["result"]["resourceSpans"].extend(payload.get("result", payload).get("resourceSpans", []))
            last = normalize_trace(combined)
        except urllib.error.HTTPError as error:
            if error.code != 404:
                raise
        spans = last.get("spans", [])
        services = {p["serviceName"] for p in last.get("processes", {}).values()}
        operations = {s["operationName"] for s in spans}
        if len(services) >= 3 and "healer.candidate.verify" in operations and any("result" in n for n in operations):
            return last
        time.sleep(1)
    raise AssertionError("Missing exported distributed spans: " + json.dumps(last))


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", default="/tmp/sentinel-phase3.json")
    args = parser.parse_args()
    assert request("/api/v1/health")["status"] == "healthy"
    source = request("/api/v1/pipelines")[0]["id"]
    trace_id = secrets.token_hex(16)
    job = trigger(source, trace_id)
    detail = wait_detail(job["pipeline_id"], lambda d: bool(d.get("patches")) and any(p["verification_status"] == "VERIFIED" for p in d["patches"]))
    patch = next(p for p in detail["patches"] if p["verification_status"] == "VERIFIED")
    path = f"/api/v1/patches/{patch['id']}/candidates"
    result = request(path)
    candidates = result["candidates"]
    assert len(candidates) == 3, candidates
    verified = [c for c in candidates if c["test_success"] and c["status"] == "VERIFIED"]
    assert verified and len({c["patched_code"] for c in candidates}) == 3
    winner = next(c for c in candidates if c["id"] == result["selected_candidate_id"])
    assert winner["test_success"] and winner["ast_cost"] == min(c["ast_cost"] for c in verified)
    assert sum(bool(c["selected"]) for c in candidates) == 1
    assert all(c.get("diagnostics", {}).get("trace_id") == trace_id for c in candidates)
    alternate = next(c for c in verified if c["id"] != winner["id"])
    changed = request(f"/api/v1/patches/{patch['id']}/select-candidate", "POST", {"candidate_id": alternate["id"]})
    assert changed["selected_candidate_id"] == alternate["id"]
    refreshed = request(f"/api/v1/pipelines/{job['pipeline_id']}")
    selected = next(p for p in refreshed["patches"] if p["id"] == patch["id"])
    assert selected["patched_code"] == alternate["patched_code"]
    assert selected["approval_status"] == "PENDING"
    assert not any(p["patch_id"] == patch["id"] for p in request("/api/v1/pulls"))
    trace = exported_trace(trace_id)
    spans = trace["spans"]
    ids = {s["spanID"] for s in spans}
    internal_links = sum(any(r.get("spanID") in ids and r.get("refType") == "CHILD_OF" for r in s.get("references", [])) for s in spans)
    assert internal_links >= 5, "SDK spans must form an actual parent-child graph"
    output = {"result": "passed", "pipeline_id": job["pipeline_id"], "patch_id": patch["id"], "trace_id": trace_id,
              "services": sorted({p["serviceName"] for p in trace["processes"].values()}),
              "operations": sorted({s["operationName"] for s in spans}), "span_count": len(spans),
              "internal_parent_links": internal_links, "candidates": [{k: c[k] for k in ("id", "variant", "ast_cost", "status")} for c in candidates],
              "alternate_selected": alternate["id"], "approval_status": "PENDING"}
    with open(args.output, "w") as handle:
        json.dump(output, handle, indent=2)
    print(json.dumps(output, indent=2))


if __name__ == "__main__":
    main()
