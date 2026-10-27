"""Real API-key revocation and signed webhook dispatch checks; never prints tokens."""
import hashlib
import hmac
import json
import time
import urllib.error
import urllib.request
from uuid import uuid4
from simulate import BASE, login, request


def raw(path, method="GET", body=None, headers=None):
    req = urllib.request.Request(BASE+path, data=body, headers=headers or {}, method=method)
    with urllib.request.urlopen(req, timeout=30) as response:
        return response.status, json.load(response)


def main():
    login()
    key = request("/api/v1/auth/keys", "POST", {"action":"create", "name":"phase2-viewer-probe", "role":"viewer"})
    status, repos = raw("/api/v1/repos", headers={"X-Sentinel-Token":key["token"]})
    assert status == 200 and repos
    request("/api/v1/auth/keys", "POST", {"action":"revoke", "id":key["id"]})
    try:
        raw("/api/v1/repos", headers={"X-Sentinel-Token":key["token"]})
    except urllib.error.HTTPError as error:
        assert error.code == 401
    else:
        raise AssertionError("revoked API token remained valid")
    suffix = uuid4().hex[:8]
    url = "https://example.invalid/sentinel-security-"+suffix
    repo = request("/api/v1/repos", "POST", {"name":"security-probe-"+suffix,"repo_url":url,"default_branch":"main"})
    secret = uuid4().hex+uuid4().hex
    request("/api/v1/repos/"+repo["id"], "PATCH", {"webhook_secret":secret})
    payload = {"ref":"refs/heads/main","after":"7f3a9c2"+"0"*33,"repository":{"clone_url":url,"html_url":url},"head_commit":{"message":"test(security): verify signed webhook dispatch"}}
    body = json.dumps(payload).encode()
    delivery = str(uuid4())
    headers = {"X-GitHub-Event":"push", "Content-Type":"application/json", "X-GitHub-Delivery":delivery, "X-Hub-Signature-256":"sha256="+hmac.new(secret.encode(),body,hashlib.sha256).hexdigest()}
    tampered = body+b" "
    try:
        raw("/api/v1/webhooks/github", "POST", tampered, headers)
    except urllib.error.HTTPError as error:
        assert error.code == 401
    else:
        raise AssertionError("tampered webhook accepted")
    status, accepted = raw("/api/v1/webhooks/github", "POST", body, headers)
    assert status == 202, accepted
    status, duplicate = raw("/api/v1/webhooks/github", "POST", body, headers)
    assert status == 202 and duplicate["id"] == accepted["id"], duplicate
    # The intentionally non-resolving test repository must fail safely after dispatch.
    deadline = time.monotonic()+45
    while time.monotonic()<deadline:
        detail = request("/api/v1/pipelines/"+accepted["id"])
        if detail["status"]=="FAILED":
            break
        time.sleep(1)
    assert detail["status"]=="FAILED", detail
    actions = {row["action"] for row in request("/api/v1/audit-logs")}
    assert {"api_key.create","api_key.revoke","repository.configure","pipeline.trigger"} <= actions, actions
    print(json.dumps({"security_probe":"passed","api_key_revoked":True,"signed_webhook_deduplicated":True,"tampered_webhook_rejected":True}))


if __name__ == "__main__":
    main()
