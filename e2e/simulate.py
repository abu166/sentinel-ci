"""Authenticated requests to the running SentinelCI gateway."""
import json
import os
import urllib.request

BASE = os.environ.get("SENTINEL_URL", "http://localhost:8080")
_session_token = os.environ.get("SENTINEL_TOKEN", "")


def login(role="admin"):
    global _session_token
    body = {"demo": True, "role": role}
    if os.environ.get("SENTINEL_ADMIN_PASSWORD"):
        body = {"username": os.environ.get("SENTINEL_ADMIN_USERNAME", "admin"), "password": os.environ["SENTINEL_ADMIN_PASSWORD"]}
    req = urllib.request.Request(BASE + "/api/v1/auth/login", data=json.dumps(body).encode(), headers={"Content-Type": "application/json"}, method="POST")
    with urllib.request.urlopen(req, timeout=15) as response:
        _session_token = json.load(response)["token"]
    return _session_token


def request(path: str, method: str = "GET", payload=None, token=None):
    if token is None:
        if not _session_token and path != "/api/v1/health":
            login()
        token = _session_token
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    data = json.dumps(payload).encode() if payload is not None else None
    req = urllib.request.Request(BASE + path, data=data, headers=headers, method=method)
    with urllib.request.urlopen(req, timeout=120) as response:
        return json.load(response)


if __name__ == "__main__":
    pipelines = request("/api/v1/pipelines")
    if not pipelines:
        raise SystemExit("No pipeline exists; run make seed first")
    result = request(f"/api/v1/pipelines/{pipelines[0]['id']}/simulate-failure", "POST", {"scenario": os.getenv("SCENARIO", "off-by-one")})
    print(json.dumps(result, indent=2))
