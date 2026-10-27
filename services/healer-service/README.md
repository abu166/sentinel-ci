# Healer service

The FastAPI service exposes health at `GET /api/v1/health` and authenticated internal operations at `POST /api/v1/analyze-and-heal` and `POST /api/v1/publish`. Deploy with `INTERNAL_TOKEN` and send `X-Sentinel-Internal-Token`; a configured database without an internal token fails closed. Compose exposes the service only on its internal network.

## Localization and verification

Analyze requests contain `logs`, a safe relative-path `files` map, `test_command`, and `language` (`python` or `go`). Optional `pipeline_id` and `step_id` associate the repair with stored executions. Python `ast` and Go Tree-sitter find enclosing functions, same-module helpers and branch complexity. Ochiai and Tarantula functions accept coverage counts; trace-only context scores remain heuristic rather than measured coverage evidence.

Deterministic candidates cover supported boundary indexes, inclusive loop limits, missing Python imports, string conversion, None/nil argument guards, declared Go mutex omissions, discarded errors, optional mapping fields and context-cancellation cases. Test sources are never modified. The engine first requires a failing baseline, then a passing candidate command and a separate regression command. Set `regression_command` explicitly for a repository's complete suite; defaults run the appropriate unittest, pytest or Go suite (preserving `-race`). At most twenty candidate attempts run within a five-minute overall deadline. Unsupported cases are rejected with diagnostic output.

`SANDBOX_MODE=docker` verifies candidates inside disposable non-root containers: read-only image filesystem, independent writable tmpfs workspace, no network, no capabilities, no new privileges, memory/CPU/process quotas and bounded output. Source arrives as a tar stream on stdin. Job containers receive neither host mounts nor the Docker socket or service credentials. The trusted controller needs Docker access and must remain private. `SANDBOX_MODE=process` is the local unit-test/development path; its temporary directories are not a security boundary. Configure `SANDBOX_IMAGE`, `SANDBOX_MEMORY`, and `SANDBOX_CPUS` as needed.

Set `LLM_API_KEY`, optionally `LLM_API_URL` and `LLM_MODEL`, for the optional OpenAI-compatible candidate adapter. Model proposals pass the same real verification gates. No model request occurs without credentials.

## Durable events

With `DATABASE_URL` and `RABBITMQ_URL`, the consumer reads durable `healer.trigger` messages on exchange `sentinel`. Patch records, terminal pipeline/step state and `healer.result` outbox events are committed in one PostgreSQL transaction; the Go outbox relay publishes them. Stable patch/event IDs make persistence idempotent under redelivery. Transient processing failures receive up to three confirmed retries; invalid or exhausted requests enter `healer.dead` for operator inspection and replay.

## Real Git publication

Publish accepts `{ "patch_id": "uuid", "approved": true, "open_pr": true, "provider": "local" }`; provider is optional and may also be `github` or `gitlab`. Metadata, captured source and verified diff are loaded from PostgreSQL. Auto-publication runs only when repository policy disables manual approval and enables auto-commit or PR creation.

The managed bare store uses isolated worktrees, `git apply --check`, structured sign-off commits, per-repository filesystem locks and per-patch PostgreSQL advisory locks. The branch format begins `sentinel/fix-<category>-<base-sha>`. Repeated publication returns the existing pull record; a retried local Git operation reuses its prior repair commit. Source-controlled Git hooks and global configuration are disabled in the controller.

Local fallback creates a real branch and commit. `GITHUB_TOKEN` or `GITLAB_TOKEN` enable remote branch pushes and PR/MR adapters. HTTPS provider hosts are constrained; `GITLAB_HOST` selects a self-hosted GitLab domain. Push credentials use a temporary askpass environment rather than command arguments or URLs. `GIT_SIGNING_KEY` enables cryptographic commit signing; otherwise commits carry sign-off and structured RCA metadata without claiming a cryptographic signature. Persist `GIT_STORE_PATH` across restarts.

## Tests

```sh
python3 -m venv .venv
.venv/bin/pip install -r requirements.txt
.venv/bin/pytest -v --cov=healer
```

Tests execute real buggy Go/Python fixtures and real local Git bare/worktree workflows, with isolated API-adapter and broker failure tests. Runtime coverage includes all healer modules.

## Candidate voting and traces

A supported failure now produces up to three distinct code/AST transformations. Unsupported rules do not receive fabricated alternatives. Candidate test and regression commands run concurrently in separate disposable sandboxes, bounded both per request and by `HEALER_CANDIDATE_CONCURRENCY` (default three, including baseline jobs). Verification success is mandatory. Passing candidates rank by actual structural AST edit cost, then complexity increase, then stable proposal ID. The displayed dimensionless ranking score is `0.8 + 0.2 / (1 + ast_cost)` for a verified candidate and zero for a rejected one; it is not a calibrated correctness probability.

Every candidate retains its actual stdout, diff, source, diagnostics and selected status. Output-only masking covers known credentials, token formats, high-entropy values, credential URLs, quoted/multiline assignments, YAML block values and private-key blocks before evidence leaves the engine. Source snapshots and reviewed diffs remain unchanged. Masking is heuristic and does not guarantee detection of every possible secret encoding.

Candidates and the winner reference are committed atomically with patch state and the result outbox event. Redelivery does not overwrite a reviewer-selected candidate. The gateway exposes candidate inspection and guarded selection while approval is pending. Diagnostics include actual Python/Go helper-call graph nodes and edges from their respective parsers, never proximity-based decorative connections.

OpenTelemetry SDK spans cover HTTP requests, RabbitMQ consumers, repair selection, parallel candidates and sandboxes. Only W3C `traceparent`/`tracestate` propagate through outbox `headers`, publications and retries. Configure `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` (full `/v1/traces` URL) or `OTEL_EXPORTER_OTLP_ENDPOINT` (base collector URL) for the OTLP HTTP exporter. Without an exporter endpoint, SDK trace context still exists but no claim of exported/retained traces is made. The implementation follows the [OpenTelemetry Python instrumentation](https://opentelemetry.io/docs/languages/python/instrumentation/) and [OTLP HTTP exporter](https://opentelemetry-python.readthedocs.io/en/latest/exporter/otlp/otlp.html) APIs.
