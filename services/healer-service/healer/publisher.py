"""Real Git worktree publication with local and authenticated remote PR adapters."""
import asyncio
from contextlib import contextmanager
import fcntl
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import subprocess
import tempfile
import urllib.error
import urllib.parse
import urllib.request
from uuid import uuid4


class PublishError(ValueError):
    """A publication could not be completed safely."""


def git(args: list[str], cwd: Path, data: str | None = None) -> str:
    environment = {key: value for key, value in os.environ.items() if key in {"PATH", "LANG", "GNUPGHOME", "SSH_AUTH_SOCK"}}
    environment.update({"HOME": str(cwd), "GIT_TERMINAL_PROMPT": "0", "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null", "GIT_AUTHOR_NAME": "SentinelCI", "GIT_AUTHOR_EMAIL": "sentinel@localhost", "GIT_COMMITTER_NAME": "SentinelCI", "GIT_COMMITTER_EMAIL": "sentinel@localhost"})
    try:
        result = subprocess.run(["git", "-c", "core.hooksPath=/dev/null", *args], cwd=cwd, env=environment, input=data, text=True, capture_output=True, timeout=90, check=False)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise PublishError("Git command unavailable or timed out") from error
    if result.returncode:
        raise PublishError(f"Git {args[0]} failed: {result.stderr[-1500:]}")
    return result.stdout.strip()


def safe_bundle(files: dict[str, str]) -> None:
    if not files or len(files) > 100 or sum(len(v) for v in files.values()) > 4_000_000:
        raise PublishError("source snapshot must contain 1..100 files within 4 MB")
    for name in files:
        path = PurePosixPath(name)
        if path.is_absolute() or any(part in {"..", ".git"} for part in path.parts) or "\\" in name or "\x00" in name or str(path) != name or name == ".":
            raise PublishError("unsafe Git source path")


@contextmanager
def repository_lock(store: Path, key: str):
    store.mkdir(parents=True, exist_ok=True, mode=0o700)
    with (store / f"{key}.lock").open("a") as handle:
        fcntl.flock(handle, fcntl.LOCK_EX)
        try:
            yield
        finally:
            fcntl.flock(handle, fcntl.LOCK_UN)


def commit_patch(record: dict, files: dict[str, str], store: Path, provider: str) -> dict:
    safe_bundle(files)
    path = record["file_path"]
    if path not in files or files[path] != record["original_code"]:
        raise PublishError("reviewed patch original does not match captured source")
    category = re.sub(r"[^a-z0-9-]", "-", record["bug_category"].lower()).strip("-")[:40] or "repair"
    key = hashlib.sha256(str(record["repository_id"]).encode()).hexdigest()[:32]
    title = f"fix({category}): apply verified SentinelCI repair"
    body = f"{record['root_cause_analysis']}\n\nVerification:\n{record['test_output'][-12000:]}\n\nSentinel-Patch: {record['id']}\nSentinel-Pipeline: {record['pipeline_id']}"
    with repository_lock(store, key):
        bare = store / f"{key}.git"
        if not bare.exists():
            git(["init", "--bare", str(bare)], store)
        with tempfile.TemporaryDirectory(prefix="sentinel-publish-", dir=store) as temporary:
            workspace = Path(temporary)
            baseline = workspace / "baseline"
            baseline.mkdir()
            remote_url = record.get("repo_url", "")
            base_ref = "sentinel/base-" + str(record["pipeline_id"])
            previous_base = git(["--git-dir", str(bare), "for-each-ref", "--format=%(objectname)", f"refs/heads/{base_ref}"], store)
            if provider == "local":
                git(["init", "-b", "main"], baseline)
                for name, content in files.items():
                    target = baseline / name
                    target.parent.mkdir(parents=True, exist_ok=True)
                    target.write_text(content)
                git(["add", "--all"], baseline)
                git(["commit", "-m", "chore: capture reproducible CI source snapshot"], baseline)
            else:
                parsed = urllib.parse.urlparse(remote_url)
                expected = "github.com" if provider == "github" else os.getenv("GITLAB_HOST", "gitlab.com")
                if parsed.scheme != "https" or parsed.hostname != expected or parsed.username or parsed.password:
                    raise PublishError("remote repository must use an approved HTTPS provider host")
                git(["clone", "--no-checkout", "--", remote_url, "."], baseline)
                revision = record.get("source_sha") or record["base_branch"]
                if revision.startswith("-") or not re.fullmatch(r"[A-Za-z0-9_./-]+", revision):
                    raise PublishError("unsafe source revision")
                git(["checkout", "--detach", revision], baseline)
                if (baseline / path).read_bytes() != record["original_code"].encode():
                    raise PublishError("remote source no longer matches reviewed patch")
            base_sha = previous_base or git(["rev-parse", "HEAD"], baseline)
            if not previous_base:
                git(["push", str(bare), f"HEAD:refs/heads/{base_ref}"], baseline)
            primary = f"sentinel/fix-{category}-{base_sha[:7]}"
            legacy = primary + "-" + str(record["id"])[:8]
            fingerprint = hashlib.sha256(json.dumps([str(record["id"]), path, record["patched_code"], record["unified_diff"]], ensure_ascii=False, separators=(",", ":")).encode()).hexdigest()
            expected_blob = git(["hash-object", "--stdin"], store, record["patched_code"])
            # A database rollback, or a new pending candidate selected before publication,
            # can leave a real Git branch with the same patch ID and different content.
            # Preserve it and use a content-addressed branch instead of force-pushing.
            resume_baseline = False
            for branch in (primary, legacy, legacy + "-" + fingerprint):
                existing = git(["--git-dir", str(bare), "for-each-ref", "--format=%(objectname)", f"refs/heads/{branch}"], store)
                if not existing:
                    break
                if existing == base_sha:
                    resume_baseline = True
                    existing = ""
                    break
                message = git(["--git-dir", str(bare), "show", "-s", "--format=%B", existing], store)
                try:
                    actual_blob = git(["--git-dir", str(bare), "rev-parse", f"{existing}:{path}"], store)
                except PublishError:
                    actual_blob = ""
                if f"Sentinel-Patch: {record['id']}" in message.splitlines() and actual_blob == expected_blob:
                    break
            else:
                raise PublishError("deterministic repair branches conflict; no reviewed branch was overwritten")
            worktree = workspace / "worktree"
            created_branch = not existing and not resume_baseline
            args = [str(worktree), branch] if resume_baseline else ["--detach", str(worktree), existing] if existing else ["-b", branch, str(worktree), base_sha]
            git(["--git-dir", str(bare), "worktree", "add", *args], store)
            try:
                if not existing:
                    git(["apply", "--check", "-"], worktree, record["unified_diff"])
                    git(["apply", "--whitespace=nowarn", "-"], worktree, record["unified_diff"])
                    if (worktree / path).read_bytes() != record["patched_code"].encode():
                        raise PublishError("applied diff does not match verified source")
                    git(["add", "--all"], worktree)
                    signing = os.getenv("GIT_SIGNING_KEY")
                    args = (["-c", f"user.signingkey={signing}"] if signing else []) + ["commit", "--signoff"] + (["-S"] if signing else []) + ["-m", title, "-m", body]
                    git(args, worktree)
                elif (worktree / path).read_bytes() != record["patched_code"].encode():
                    raise PublishError("existing repair branch conflicts with verified source")
                sha = git(["rev-parse", "HEAD"], worktree)
                actual_diff = git(["diff", base_sha, sha], worktree)
                if provider != "local":
                    push_remote(record, worktree, branch, provider)
            finally:
                git(["--git-dir", str(bare), "worktree", "remove", "--force", str(worktree)], store)
                if created_branch:
                    remaining = git(["--git-dir", str(bare), "for-each-ref", "--format=%(objectname)", f"refs/heads/{branch}"], store)
                    if remaining == base_sha:
                        git(["--git-dir", str(bare), "update-ref", "-d", f"refs/heads/{branch}", base_sha], store)
    return {"branch": branch, "commit_sha": sha, "title": title, "body": body, "diff": actual_diff, "signed": bool(os.getenv("GIT_SIGNING_KEY")), "bare_repository": str(bare)}


def provider_token(provider: str) -> str:
    token = os.getenv("GITHUB_TOKEN" if provider == "github" else "GITLAB_TOKEN", "")
    if not token:
        raise PublishError(f"{provider} credentials are not configured")
    return token


def push_remote(record: dict, worktree: Path, branch: str, provider: str) -> None:
    # The token is sent through a short-lived askpass file, never placed in argv or URLs.
    token = provider_token(provider)
    with tempfile.TemporaryDirectory(prefix="sentinel-askpass-") as temporary:
        askpass = Path(temporary) / "askpass"
        askpass.write_text('#!/bin/sh\ncase "$1" in *Username*) printf "%s" "oauth2";; *) printf "%s" "$SENTINEL_GIT_CREDENTIAL";; esac\n')
        askpass.chmod(0o700)
        environment = {"PATH": os.environ.get("PATH", "/usr/bin:/bin"), "HOME": str(worktree), "GIT_TERMINAL_PROMPT": "0", "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null", "GIT_ASKPASS": str(askpass), "SENTINEL_GIT_CREDENTIAL": token}
        try:
            result = subprocess.run(["git", "-c", "core.hooksPath=/dev/null", "push", record["repo_url"], f"HEAD:refs/heads/{branch}"], cwd=worktree, env=environment, capture_output=True, text=True, timeout=90)
        except (OSError, subprocess.TimeoutExpired) as error:
            raise PublishError("remote Git push unavailable or timed out") from error
        if result.returncode:
            raise PublishError("remote Git push failed")


def remote_result(data: dict, provider: str) -> dict:
    url = data["html_url"] if provider == "github" else data["web_url"]
    number = data["number"] if provider == "github" else data["iid"]
    if not isinstance(url, str) or not url.startswith("https://") or not isinstance(number, int) or isinstance(number, bool):
        raise ValueError("invalid remote pull request response")
    state = data.get("state", "open")
    status = "MERGED" if data.get("merged_at") or state == "merged" else "CLOSED" if state == "closed" else "OPEN"
    return {"url": url, "remote_number": number, "status": status}


def find_remote(endpoint: str, headers: dict, record: dict, commit: dict, provider: str) -> dict | None:
    """Resolve deterministic source/target branches before attempting a remote write."""
    if provider == "github":
        owner = urllib.parse.urlparse(record["repo_url"]).path.strip("/").split("/")[0]
        query = {"head": f"{owner}:{commit['branch']}", "base": record["base_branch"], "state": "all", "per_page": 100}
    else:
        query = {"source_branch": commit["branch"], "target_branch": record["base_branch"], "state": "all", "scope": "all", "per_page": 100}
    req = urllib.request.Request(endpoint + "?" + urllib.parse.urlencode(query), headers=headers, method="GET")
    try:
        with urllib.request.urlopen(req, timeout=30) as response:
            results = json.loads(response.read(2_000_000))
        if not isinstance(results, list):
            raise ValueError("remote lookup did not return a list")
        for data in results:
            if not isinstance(data, dict):
                raise ValueError("invalid remote lookup item")
            if provider == "github":
                source = data["head"]["ref"]
                target = data["base"]["ref"]
            else:
                source = data["source_branch"]
                target = data["target_branch"]
            if source == commit["branch"] and target == record["base_branch"]:
                return remote_result(data, provider)
        return None
    except (OSError, ValueError, KeyError, TypeError) as error:
        raise PublishError("remote pull request lookup failed; publication was not retried") from error


def open_remote(record: dict, commit: dict, provider: str) -> dict:
    parsed = urllib.parse.urlparse(record["repo_url"])
    project = parsed.path.strip("/").removesuffix(".git")
    token = provider_token(provider)
    if provider == "github":
        endpoint = f"https://api.github.com/repos/{project}/pulls"
        payload = {"title": commit["title"], "body": commit["body"], "head": commit["branch"], "base": record["base_branch"]}
        headers = {"Authorization": f"Bearer {token}", "Accept": "application/vnd.github+json"}
    else:
        endpoint = f"https://{os.getenv('GITLAB_HOST', 'gitlab.com')}/api/v4/projects/{urllib.parse.quote(project, safe='')}/merge_requests"
        payload = {"title": commit["title"], "description": commit["body"], "source_branch": commit["branch"], "target_branch": record["base_branch"]}
        headers = {"PRIVATE-TOKEN": token}
    existing = find_remote(endpoint, headers, record, commit, provider)
    if existing:
        return existing
    req = urllib.request.Request(endpoint, data=json.dumps(payload).encode(), headers={**headers, "Content-Type": "application/json"}, method="POST")
    try:
        with urllib.request.urlopen(req, timeout=30) as response:
            data = json.loads(response.read(2_000_000))
        return remote_result(data, provider)
    except (OSError, ValueError, KeyError, TypeError) as error:
        # A server can accept a POST even when its response is lost. Reconcile the
        # deterministic branch pair before allowing an outer delivery retry.
        existing = find_remote(endpoint, headers, record, commit, provider)
        if existing:
            return existing
        raise PublishError("remote pull request creation failed") from error


async def publish_patch(pool, patch_id: str, approved: bool, open_pr: bool = True, provider: str | None = None) -> dict:
    if not approved:
        raise PublishError("explicit approval or authorized repository policy required")
    async with pool.acquire() as connection:
        async with connection.transaction():
            await connection.execute("SELECT pg_advisory_xact_lock(hashtext($1))", patch_id)
            existing = await connection.fetchrow("SELECT * FROM pull_requests WHERE patch_id=$1::uuid", patch_id)
            if existing:
                prior = dict(existing)
                if open_pr and prior["provider"] != "local" and prior["status"] == "BRANCH":
                    repository = await connection.fetchrow("SELECT repo_url FROM repositories WHERE id=$1::uuid", str(prior["repository_id"]))
                    remote = await asyncio.to_thread(open_remote, {**prior, "repo_url": repository["repo_url"]}, prior, prior["provider"])
                    updated = await connection.fetchrow("UPDATE pull_requests SET status=$2,url=$3,remote_number=$4,updated_at=NOW() WHERE id=$1::uuid RETURNING *", str(prior["id"]), remote["status"], remote["url"], remote["remote_number"])
                    return dict(updated)
                return prior
            row = await connection.fetchrow("""SELECT h.*,p.repo_id AS repository_id,p.commit_sha AS source_sha,r.repo_url,r.default_branch AS base_branch,r.manual_approval,j.payload
                FROM healing_patches h JOIN pipelines p ON p.id=h.pipeline_id JOIN repositories r ON r.id=p.repo_id
                JOIN job_snapshots j ON j.pipeline_id=p.id WHERE h.id=$1::uuid FOR UPDATE OF h""", patch_id)
            if not row or row["verification_status"] != "VERIFIED" or row.get("approval_status") == "REJECTED" or (row.get("manual_approval", False) and row.get("approval_status") != "APPROVED"):
                raise PublishError("verified patch and captured source required")
            record = dict(row)
            payload = json.loads(record["payload"]) if isinstance(record["payload"], str) else record["payload"]
            files = payload.get("files", {})
            selected = provider or ("github" if os.getenv("GITHUB_TOKEN") and "github.com/" in record["repo_url"] else "gitlab" if os.getenv("GITLAB_TOKEN") and os.getenv("GITLAB_HOST", "gitlab.com") in record["repo_url"] else "local")
            if selected not in {"local", "github", "gitlab"}:
                raise PublishError("unsupported Git provider")
            commit = await asyncio.to_thread(commit_patch, record, files, Path(os.getenv("GIT_STORE_PATH", "/tmp/sentinel-git-store")), selected)
            remote = await asyncio.to_thread(open_remote, record, commit, selected) if selected != "local" and open_pr else {"status": "LOCAL" if selected == "local" else "BRANCH", "url": "", "remote_number": None}
            new_id = str(uuid4())
            saved = await connection.fetchrow("""INSERT INTO pull_requests(id,patch_id,pipeline_id,repository_id,provider,branch,base_branch,commit_sha,title,body,diff,status,url,remote_number)
                VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING *""", new_id, patch_id, str(record["pipeline_id"]), str(record["repository_id"]), selected, commit["branch"], record["base_branch"], commit["commit_sha"], commit["title"], commit["body"], commit["diff"], remote["status"], remote["url"], remote["remote_number"])
            return dict(saved)
