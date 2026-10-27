# Phase 3 Verification — Tracing, Candidate Voting, and Secret Masking

Verified **27 October 2026** against the rebuilt local Docker stack. Earlier UI and Phase 2 reports below describe their own historical revisions.

## Implemented behavior

- OpenTelemetry SDK tracing in Go gateway/runner and Python healer, exported via OTLP HTTP to Jaeger 2.22.0. W3C context survives HTTP, transactional outbox storage, RabbitMQ delivery, runner steps, bounded candidate sandboxes and the healer-result consumer. Header propagation allowlists `traceparent`/`tracestate` and excludes baggage and authorization.
- Up to three distinct supported repair candidates. Each executes the failing test and regression suite in isolated containers; a global semaphore defaults to three concurrent sandboxes, including baseline runs. Failed proposals score zero; verified proposals rank by AST edit cost, complexity and stable identity. Candidate evidence, selected diff and audit state persist transactionally. Reviewed/published patches cannot switch candidate.
- Stateful runner redaction precedes all output storage and delivery. Healer verification output has an additional output-only redaction boundary. Source snapshots and reviewed code/diffs intentionally remain intact for executable repair.
- Worker Canvas charts use actual host-scoped samples with bounded retention, real zero values and missing-value gaps. Parser-derived AST helper dependencies and ranked candidate evidence are exposed in Healing Studio.

## Source inventory

Current source files and lines, including tests and excluding installed dependencies/generated output:

| Area | Source files | Lines |
| --- | ---: | ---: |
| `services/gateway-api` | 17 | 3,368 |
| `services/runner-daemon` | 10 | 2,080 |
| `services/healer-service` | 22 | 2,623 |
| `web-console/src` | 18 | 10,312 |

## Terminal and integration results

| Gate | Actual result |
| --- | --- |
| Gateway `go test -v -race ./...` | Passed; 77 top-level tests |
| Runner `go test -race ./...` | Passed; 60 top-level tests, including concurrent masking and W3C durable/wire propagation |
| `go vet ./...` in both Go services | Passed |
| Frontend `npm run build`, `npx tsc --noEmit` | Passed; final assets `xQ6V0W_o.js` / `agktfe6i.css` |
| Frontend `npm test -- --run` | 82 passed |
| Healer `pytest -v --cov` | 164 passed; 96% total coverage on frozen final source (231.26 seconds) |
| Chromium routes and interactions | All six routes at desktop/mobile (12 loads) passed; zero console exceptions or HTTP errors. Final wire-adapter build passed an additional complete interaction run |
| Live visualization evidence | Actual Python AST graph: two functions, one call edge, API/DOM agreement; zoom and selection passed. Both Canvas charts painted actual data across two heartbeat samples |
| Final-image bounded load | 1,026 successful health requests over 20.06 seconds; zero errors; eight clients; p50 41.29 ms, p95 106.93 ms during concurrent local verification |
| Publication recovery | Passed against the previously failing approved artifact: resumed only a baseline-only branch and created actual commit `624ec5e`; exact approved bytes preserved |
| Final Docker health | All eight running services healthy; migration and sandbox image initialization exited successfully ([evidence](docs/phase3/container-health.json)) |
| Versioned migration 007 | Applied to the existing PostgreSQL volume; trace headers and candidate evidence schema available |
| Jaeger readiness | Stable `/api/v3/services` query returns success; initial legacy-endpoint readiness failure corrected |
| Live distributed trace and candidate selection | Passed; 60 exported spans, 59 internal parent links, three service names; three distinct verified proposals with AST costs 4/5/9; lowest cost selected automatically, alternate selected atomically before approval |
| Five real bug scenarios | Passed: nil pointer, off-by-one, data race, async KeyError and context timeout; each approved patch produced a PASSED rerun and a real local Git commit ([evidence](docs/phase3/five-scenarios.json)) |
| Live runner secret probe | Passed; all 11 credential fragments absent from 16 stored lines and 16 authenticated WebSocket lines, including PEM, YAML block and triple-quoted values |
| Healer output secret probe | Passed; all three candidates and selected patch test output redacted, plus PostgreSQL/WebSocket output probes |
| 90-second load | 8 concurrent health clients; 6,410 successful requests, zero errors; p50 5.69 ms, p95 26.30 ms; observed peak three candidate containers |
| 45-second isolation/load probe | 3,268 successful requests, zero errors; p50 4.46 ms, p95 20.50 ms; seven actual container configurations inspected |

The full browser report measured a maximum layout shift of **0.001439** when a live pipeline row was inserted; the other eleven route windows measured zero. The preceding strict-zero diagnostic caught that live insertion and was not reported as a pass. The final WebSocket adapter revision changed data normalization only; the same CSS passed a separate final interaction audit, including graph edges, candidate selection and two live chart samples. Evidence: [browser-complete.json](docs/phase3/browser-complete.json), [browser-final-wire.json](docs/phase3/browser-final-wire.json).

Go variants are formatted before verification and AST scoring, with external formatting offloaded from the ASGI event loop. The publisher preserves already reviewed source bytes, handles abandoned baseline-only refs, and uses deterministic suffixes for actual content conflicts. Actual reviewed commits are never overwritten. The five-scenario run passed before this final compatibility refinement; focused real-Git recovery tests and a final deployed trace/voting/load smoke test cover the refinement.

The seven observed verification containers had read-only roots, disabled networking, positive memory/CPU quotas, PID limits at most 128, all capabilities dropped and `no-new-privileges`. The first 90-second run observed concurrency but containers exited too quickly for inspection; the separate held-fixture run established the configuration evidence. Measurements are local, unthrottled observations, not production capacity claims.

Reproducible scripts: `e2e/phase3.py`, `e2e/phase3_masking.py`, and `e2e/phase3_load.py`. Portable evidence is retained in [docs/phase3](docs/phase3). Jaeger data is in memory and disappears when its container is replaced; the report retains trace IDs and measured structure, not credentials or raw secret probes.

# Historical Staff UI/UX Redesign Verification

Latest revision: **27 October 2026**. This revision changes the React console and its browser QA tooling; the backend hardening report below is retained as a historical record.

## Design system and workbench changes

- Deep obsidian foundation (`#09090b`), elevated `#121215` surfaces, translucent white hairlines, indigo accents, and semantic success/error/warning/running tokens. Muted text remains readable; inset highlights and frosted overlays add depth without decorative dashboard charts.
- Self-hosted Inter 400/500/600 and JetBrains Mono 400/500. A visible, accessible font-readiness boot state precedes the application; font failures still release the application into its fallback stack. No external font CDN is required.
- A 44px command bar with workspace breadcrumbs, integrated command search, connection state and an avatar role menu; a 240/64px collapsible sidebar; and a 48px telemetry strip with observed build/queue series.
- Modular layout and page components in `web-console/src/components/layout/` and `web-console/src/pages/`. Overview retains live master/detail selection, real seven-character SHAs and relative execution ages.
- Healing Studio has layered IDE chrome, a connected SVG DAG, stream tabs, filtered clipboard copy, source links, source-accurate split/unified diff gutters, AST diagnostics and a frosted review-action dock. Regression verification remains separate from the optional Go race detector.
- Supporting views retain real execution matrix filters and inspection drawers, repository review policies and cryptographic secret generation, worker resource evidence, and broker/outbox state. Generated secrets are not saved until submission; provider branch protection is not fabricated by the UI.
- The grouped command palette supports fuzzy matching, keyboard selection, focus wrapping and restoration. Profile role choices explicitly distinguish Admin and Viewer; Maintainer is labeled accurately. Reduced-motion preferences are respected.

## Current frontend verification

| Gate | Result |
| --- | --- |
| `npm run build` | Passed |
| `npx tsc --noEmit` | Passed |
| `npm test -- --run` | 50 tests passed |
| `docker compose -f deploy/docker-compose.yml up -d --build web-console` | Passed; redesigned frontend deployed |
| Chromium routes, interactions and layout-shift measurements | Passed: all 12 desktop/mobile route windows measured CLS **0**, with zero console/HTTP errors |

The reproducible browser audit is `node e2e/browser-ui-redesign.mjs --zero-shifts`. It starts its own temporary Chromium profile, observes layout shifts from document start, checks actual font loading and local font requests, and exercises the live application without mocking API responses. `CHROME_PATH`, `SENTINEL_URL` and `QA_OUTPUT_DIR` can select the browser, deployment and artifact destination.

The audit measures cumulative layout-shift entries excluding recent user input through font boot, authentication, API hydration and an additional 10-second polling window. It tests all six routes at desktop 1440×1000 and mobile 390×844. A zero result applies to those observed windows, not every possible session or network condition.

### Final browser results

| Route | Desktop 1440×1000 CLS | Mobile 390×844 CLS |
| --- | ---: | ---: |
| `/overview` | 0 | 0 |
| `/pipelines` | 0 | 0 |
| `/repositories` | 0 | 0 |
| `/healing-studio` | 0 | 0 |
| `/workers` | 0 | 0 |
| `/queue` | 0 | 0 |

All twelve cold-load windows covered document start through approximately **10.12–10.16 seconds**, including font boot, authentication, hydration and polling. Each route was connected to the real gateway. All five font faces loaded from successful local requests; no external font CDN was used. Sampled body, heading and primary-control contrast passed the 4.5:1 gate (7.76:1, 18.10:1 and 6.29:1 respectively). The audit used headless Chrome 156 on macOS, disabled the browser cache, and applied no CPU or network throttling.

Actual interaction checks passed: settled sidebar widths **240/64px**, grouped fuzzy search (`wrk nd` → Worker nodes), command-dialog Tab/Shift+Tab wrapping and Escape focus restoration, profile outside-click/Escape dismissal, Admin/Viewer switching and Viewer write restrictions, terminal text/stream filtering, exact clipboard contents with ANSI controls removed, auto-scroll, both diff modes, distinct cryptographically generated 64-character secrets without saving them, and matrix status filtering/inspection drawers. Mobile scrolling stayed inside the intended panes; the document remained 390px wide.

The audit identified and fixed moving connection labels, branch/author/footer slots, the font-loading label, auto-sized matrix columns and filters, infrastructure section geometry, a missing loading-state queue row, and retained numeric unit suffixes. Missing resource values now remain unreported rather than appearing as fabricated zeroes. These fixes preserve actual data and reachable scroll regions; the zero threshold was not relaxed.

The final served assets were `index-Bxhe_Mx8.js` and `index-MF8p5qBU.css`. The frontend container was rebuilt and restarted successfully; the build, explicit TypeScript check and all **50 tests** passed. Runtime code remained frozen during the final browser pass. The audit closes its own Chromium process and removes its temporary browser profile.

[Final workbench screenshot](docs/workbench.png) · [Portable measured evidence](docs/ui-verification.json)

The previous Phase 2 inventory and frontend test counts below describe the earlier revision, not this redesign.

---

## Prior Phase 2 Architecture & Verification Report

Verified on **27 October 2026** against the local rebuilt Docker deployment. The complete UI redesign and the four Phase 1 feature gaps are implemented and verified as described below.

## Four production gaps: closed

| Gap | Delivered control | Verification evidence |
| --- | --- | --- |
| Authentication/RBAC/webhooks | Expiring signed JWTs; hashed/revocable API keys; Admin/Maintainer/Viewer middleware; authenticated WebSockets; HMAC-SHA256 pushes; transactional audit history; production secret validation | JWT/role/HMAC unit tests; live anonymous401, Viewer403, key revocation401, tampered webhook401, signed dispatch202 and duplicate webhook returning the original pipeline |
| Transactional outbox/deduplication | Atomic state/snapshot/audit/outbox writes; SKIP LOCKED relay; publisher confirms; bounded retry/DEAD state; renewable Redis owner leases and durable unique step claims | Transaction/rollback/backoff tests; 20 actual duplicate broker deliveries left execution count3 and log count9 unchanged on a completed pipeline |
| Per-job isolation/quotas | Required Docker execution backend; disposable nonroot step/verification containers; read-only root; network disabled; no controller socket/secrets; CPU/RAM/swap/PID and process limits; deadline cleanup | An app-dispatched probe passed, with actual running HostConfig inspected: UID 65532, read-only root enabled, network disabled, 768 MiB memory, 1 CPU, PID limit 128, capabilities dropped and no-new-privileges; no socket/controller secrets inside |
| Git branch/PR publication | Managed bare/worktree store; `git apply`; structured root-cause/test-evidence commits; approval→durablegit.publish; local Git fallback; GitHub/GitLab push/PR adapters; idempotent publication/recovery | All five live repairs produced actual local branches/commits and passing reruns; actual Git tests exercise worktrees/diffs/commit retry; provider adapter tests cover existing PR/MR and ambiguous accepted-POST recovery |

These are implemented controls, not a claim of unlimited hostile-tenant security or semantic correctness. Hosted GitHub/GitLab credentials were not supplied: live publication used actual local Git repositories. Remote HTTP integration was verified with controlled adapter responses, including retry recovery. Cryptographic signing is optional with supplied signing configuration; verified local commits are structured and signed off, not falsely described as cryptographically signed.

## Final gates

| Executed gate | Result |
| --- | --- |
| Gateway `go test -v -race ./...` | **59 top-level tests passed** |
| Runner `go test -v -race ./...` | **42 top-level tests passed** |
| Both services `go vet ./...` | Passed |
| Healer `pytest -v --cov` | **118 cases passed;96% total coverage** |
| Console `npm run build && npm test -- --run` | Passed;**32 tests passed** |
| Console `tsc --noEmit` | Passed |
| Compose configuration validation | Passed |
| `docker compose -f deploy/docker-compose.yml up -d --build --wait` | Passed;all seven runtime services healthy |
| Versioned upgrade migrations | Passed against the existing Phase 1 database, including006 AST diagnostics |
| `python3 e2e/phase2.py` | All five required real failure/repair/publication/rerun scenarios passed |
| `python3 e2e/phase2.py --security-only` | API-token revocation and HMAC tamper/deduplication checks passed |
| `python3 e2e/phase2.py --extras` | SQL-injection and deadlock palette scenarios passed end to end |
| `python3 e2e/phase2.py --diagnostics` | Fresh persisted rankings/mutation/complexity before+after, real Git publication and passing rerun all passed |
| Browser gate against rebuilt `localhost:3000` | All six routes connected and rendered;zero current JavaScript exceptions or HTTP errors |
| Authored-source TODO/skipped-test audit | No matches |

The Python run emitted four dependency deprecation warnings under host Python 3.14, with no failures/skips. Coverage by module: analysis 99%, app 96%, engine 93%, models 100%, publisher 96%, sandbox 100%. Empty package initialization is the only preexisting coverage omission. Test code verifies actual failure and repair; production paths do not substitute successful test output.

The Compose migration and sandbox-image preparation services exit successfully after startup; PostgreSQL, Redis, RabbitMQ, gateway, runner, healer and web console remain running and healthy. Controllers have Docker daemon access; job containers do not.

## Live scenario results

Each row includes baseline failure, verified candidate and regression suite, approval, a real local Git commit, and a distinct passing pipeline rerun. Times cover the whole test flow on this local deployment, including cold compilers.

| Required scenario | End-to-end time | Actual Git commit |
| --- | ---: | --- |
| Go nil-pointer dereference | 70.43s | `eeaf8d799eff50397459bddb0fc647034022261e` |
| Python off-by-one | 6.05s | `f28ffa6511f3e8eb341dad16925d2924b7f0a82c` |
| Go map/data race | 96.66s | `6334284374399bc1c861b30261ead5a98e140072` |
| Async Python KeyError | 4.04s | `6be9aedbce137a7b3e0908e7e86f454883ee687f` |
| Go context timeout/cancellation | 70.33s | `609565b79595bd1ca6e120a7bdb6c20c4ada08ca` |

Additional palette scenarios passed: SQLite SQL-injection repair in6.08s (`afdfad71789c3cf59c218b3ae817b929bf4c2cb6`) and blocked-channel/deadlock cancellation repair in68.36s (`e3d5ea7129fc353f34c5700dd300a08bbc3a3d0d`). The SQL fixture tests ordinary names, missing names, quoted names and malicious inputs; repaired queries bind parameters.

The final diagnostics smoke repaired pipeline `1c84e20a-136d-4783-ae63-055508edff3f`, passed rerun `2e640b73-1c0c-43e5-ba32-7c451a28c196`, and published commit `1349c771b39d31fb24629aa3c72d8a101d130f1b`. Diagnostics are computed from actual Python/Go ASTs, stored as JSONB and exposed by the API. Complexity changes follow the named function across inserted lines; Go comments/string literals do not create fake branches. Rankings explicitly use trace-only coverage assumptions rather than invented test-coverage spectra.

The duplicate-delivery probe targeted `21ebd57b-0496-4cb5-9fff-fa016bfaa6f7`:20 confirmed copies of its real job event produced **3→3 executions,9→9 logs,statusPASSED**. The isolated workload probe `17879aab-9737-4317-a21b-40d3cd614ba9` entered through a database transaction/outbox/broker and emitted its marker once. Inspection observed memory limit 805306368 bytes and CPU quota 1000000000 NanoCPUs, matching configured limits.

## Browser and UX verification

The final browser gate used actual Chromium against the rebuilt web console, with a fresh context to avoid historical exceptions from an earlier asset. These direct routes were all live, HTTP200, connected to the authenticated gateway and nonblank:

- `/overview`: compact master/detail workbench and 40px telemetry ribbon.
- `/pipelines`: full execution matrix with backend step summaries and durations.
- `/repositories`: dedicated registry/configuration view.
- `/healing-studio`: DAG, terminal/source inspector and repair workspace.
- `/workers`: structured policy payloads and resource trace render correctly.
- `/queue`: broker and transactional outbox state.

`Cmd+K` opened the palette; typing Worker nodes and pressingEnter navigated to `/workers`. Viewer mode disabled write controls. At 390px width, root/body width stayed 390px; infrastructure tables had reachable internal scrollports instead of clipping columns. Component tests also cover the two additional palette POST actions, role restrictions, source linking, diff modes and actual structured policy payloads.

Live testing exposed and fixed a Workers React crash caused by rendering a policy object directly. It also caught a64-character simulated revision identifier; revisions now satisfy the accepted hexadecimal SHA format and render as seven characters. Other fixes include production rejection of known local secrets, non-finite resource-value rejection, remote PR retry lookup/reconciliation, SQL AST parameterization, test/source path boundary matching, and computed diagnostic complexity deltas. The original decorative staircase charts and disconnected navigation views are removed.

## Measured authenticated read latency

Executed `python3 e2e/load.py --requests 100 --concurrency 8` against the running authenticated metrics endpoint:

| Measurement | Observed |
| --- | ---: |
| Requests/concurrency | 100/8 |
| Requests per second | 533.23 |
| Median latency | 8.02ms |
| p95 latency | 77.54ms |

This is a short local read probe on Docker Desktop, not a sustained CI throughput benchmark. Dashboard p95 uses persisted pipeline durations. Historical seed data contributes to aggregate KPIs. The MTTR field still measures elapsed time of healed pipelines; it is not separately instrumented failure-detection-to-recovery latency.

## Authored inventory

**99 authored files**, excluding Git metadata, dependencies, virtual environments, caches and generated bundles. Runtime sources and tests total **11,596 physical lines**, including whitespace/comments and console CSS:

| Subsystem | Runtime source | Tests/support | Total |
| --- | ---: | ---: | ---: |
| Go gateway | 1,668 | 931 | 2,599 |
| Go runner | 996 | 471 | 1,467 |
| Python healer | 1,010 | 803 | 1,813 |
| React console, including CSS | 5,224 | 493 | 5,717 |

SQL/contracts/deployment/scripts/documentation are included in the file count but excluded from these language totals. Work ran across the requested frontend, Go backend, Python healer and adversarial QA agents; recursive QA/explorer audits exercised integration/security failure cases.

## Operating assumptions

The four Phase 1 feature gaps are closed by the controls above. Important boundaries remain explicit: disposable containers share the host kernel; Docker controllers are privileged trusted infrastructure; command-resource measurements are observability data and never enforce quotas. Trace-only suspiciousness is heuristic; green tests do not prove arbitrary correctness. A crash with ambiguous external side effects is quarantined as interrupted rather than automatically executing the same step again. Dead events need authorized operator review/replay.

The bundled deployment is deliberately a local portfolio environment with demo role switching. Exposed deployments must disable demo mode, replace local secrets, protect controller access and terminate TLS at an ingress. Known local authentication secrets are rejected in non-demo mode. Git provider credentials and optional GPG material are deployment inputs. None of these inputs is passed into workload containers.

Console: **http://localhost:3000/overview**. Gateway health: **http://localhost:8080/api/v1/health**. `make clean` stops the stack while preserving volumes.
