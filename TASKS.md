# Task Breakdown (v1)

## Phase 1 — Scaffolding
- [x] SS-001: Initialize Go module and logging
  - Create `go.mod` with module path from repository
  - Add `cmd/swarm-sentinel/main.go` with a minimal `main` entrypoint
  - Create `internal/logging` package with a small zerolog wrapper
  - Add `.gitignore` for Go build artifacts
  - Add `Makefile` targets: `build`, `test`, `lint` (placeholders ok)
- [x] SS-002: Configuration loading
  - Define config schema (env + `.env`): poll interval, compose URL, Slack webhook, Docker API URL (`SS_DOCKER_PROXY_URL`)
  - Add `internal/config` package to load config with defaults and validation
  - Load `.env` via `github.com/joho/godotenv` if present, but prefer injected env vars (env wins over `.env`)
  - Ensure config loading is the first step in `main` and logs effective settings (redact secrets)
  - Tests: table-driven unit tests for parsing/validation (missing/invalid values, defaults)
  - Tests: integration-style test for `.env` loading with env overrides
- [x] SS-003: Main execution loop
  - Define core loop cadence using the configured poll interval
  - Add a `Runner` (or similar) struct to orchestrate fetch/compare/notify steps
  - Ensure graceful shutdown hooks (context cancellation / signal handling)
  - Add a single-cycle `RunOnce` method to simplify testing
  - Tests: verify loop timing behavior with a mock clock or injected ticker
  - Tests: verify shutdown stops the loop cleanly
- [x] SS-003.5: Wire runner to main
  - Add SIGINT/SIGTERM signal handling in `main.go` for graceful shutdown
  - Instantiate `Runner` with loaded config values (poll interval, logger)
  - Call `runner.Run(ctx)` with a cancellable context
  - Validate optional `SlackWebhookURL` format when provided

## Phase 2 — Inputs
- [x] SS-004: Fetch remote compose file
  - Define a `ComposeFetcher` (or similar) interface to decouple HTTP retrieval from callers
  - Implement HTTP GET fetch with configurable URL and timeout
  - Support optional `ETag` / `If-None-Match` handling to avoid redundant downloads
  - Validate response: 200 OK with non-empty body, reject other status codes
  - Limit read size to a sane max (e.g., a few MB) to avoid runaway payloads
  - Return raw bytes plus metadata (etag, last-modified) for later fingerprinting
  - Tests: table-driven unit tests for status handling, empty body, size limit, and ETag reuse
- [x] SS-005: Fingerprinting desired state
  - Add a fingerprint helper that computes a SHA-256 hash of the compose bytes
  - Return the fingerprint as a hex string alongside compose metadata
  - Store the last fingerprint in-memory to detect changes between polls
  - Skip downstream processing when the fingerprint matches the previous value
  - Log the fingerprint on change for traceability
  - Tests: same input yields same fingerprint; different input yields different fingerprint; empty input rejected
- [x] SS-006: Compose parsing
  - Use `compose-go` for Compose spec parsing
  - Parse `services` entries and required `image`
  - Parse `deploy.replicas` with default (e.g., 1); decide behavior for `deploy.mode: global`
  - Parse service `configs` and `secrets` (short and long syntax)
  - Normalize config/secret names via top-level `configs`/`secrets` (handle `external: true` and `name`)
  - Decide behavior for missing/unknown config/secret references (error vs ignore)
  - Return a normalized desired-state struct (service name -> image/replicas/configs/secrets)
  - Tests: invalid YAML, missing `services`, missing `image`, replicas defaults
  - Tests: configs/secrets short+long syntax, external name mapping, inline definitions

## Phase 3 — Swarm Integration
> **Note:** Validate Docker connection (SS-007) early to fail fast before compose fetching.

- [x] SS-007: Docker client via official Go SDK
  - Create `internal/swarm` package for Docker client interactions
  - Define `SwarmClient` interface for testability (mock Swarm API in tests)
  - Initialize Docker client with configurable host (`SS_DOCKER_PROXY_URL`)
  - Use the Docker Go SDK with API version negotiation
  - Validate connection on startup (ping or info call) — fail fast if unreachable
  - Set reasonable timeouts for API calls
  - Tests: connection validation success/failure
  - Tests: mock client for unit testing without Docker dependency
  - Add TLS/https support:
    - Add TLS config options (e.g., `SS_DOCKER_TLS_CA`, `SS_DOCKER_TLS_CERT`, `SS_DOCKER_TLS_KEY`) and wire `WithTLSClientConfig`
    - Allow `https://` hosts when TLS is configured, mapping to `tcp://` with `WithScheme("https")`
    - Honor `DOCKER_CERT_PATH`/`DOCKER_TLS_VERIFY` for compatibility with Docker tooling

- [x] SS-008: Actual state collection
  - Implement service + task collection using Docker Go SDK APIs (`ServiceList`, `TaskList`)
  - Scope services using `SS_STACK_NAME` (label filter `com.docker.stack.namespace` or name prefix mapping); empty means all services
  - Replica source: use `Spec.Mode.Replicated.Replicas` when replicated; for global, use `ServiceStatus.DesiredTasks`
  - Image source: prefer `Spec.TaskTemplate.ContainerSpec.Image` for the expected image
  - Config/secret names: read `TaskSpec.ContainerSpec.Configs[].ConfigName` and `Secrets[].SecretName`
  - Task filtering: use SDK task filters (`filters.Arg("service", service.ID)`)
  - Define `ActualState` struct mirroring `DesiredState` structure:
    - Service name → image, running replicas, attached configs/secrets
  - Filter tasks by state (only count `running` tasks as healthy)
  - Extract config/secret names from task spec (for drift detection in Phase 4)
  - Handle pagination if service/task lists are large
  - Add `SwarmClient` dependency to `Runner`
  - Call `GetActualState()` each cycle; do not skip actual state when compose is unchanged
  - Log actual state summary (service count, total replicas)
  - Store `lastActualState` for Phase 4 comparison
  - Keep compose short-circuiting for parsing, not for actual-state collection
  - Tests: mock API responses for various service states
  - Tests: task filtering logic (running vs failed/pending)
  - Tests: config/secret extraction from task spec

## Phase 3.5 — Multi-Stack Support
- [x] SS-007.5: Multi-stack compose mapping configuration
  - Add optional `SS_COMPOSE_MAPPING_FILE` environment variable
  - Create `internal/config/mapping.go` to parse YAML mapping files
  - Implement smart path detection: `SS_COMPOSE_MAPPING_FILE` env var → `/run/configs/compose-mapping.yaml` (Swarm) → `/run/secrets/compose-mapping.yaml` → `./compose-mapping.yaml`
  - Define `StackMapping` struct: `name`, `compose_url`, optional `timeout`
  - Validate: unique stack names, non-empty names/URLs, valid URLs
  - Backward compatible: if no mapping file found, use single `SS_COMPOSE_URL` + `SS_STACK_NAME` mode
  - Tests: valid/invalid YAML, missing/duplicate names, file not found (fallback to single mode)
  - Document YAML schema and Swarm deployment patterns in code comments

- [x] SS-008.5: Coordinator for multi-runner orchestration
  - Create `internal/coordinator/coordinator.go` with `Coordinator` type
  - Implement runner spawn logic: one `Runner` per stack mapping (or single runner if single-stack mode)
  - Implement `Run(ctx)` lifecycle: parallel runner execution, context propagation, error handling
  - Implement `Stop()` for graceful shutdown: wait for all runners, cleanup
  - Add per-runner logging with stack name field for traceability
  - Reuse single `SwarmClient` across all runners (shared Docker API connection)
  - Handle partial failures: log errors per runner, continue other runners
  - Update `main.go` to detect mode and instantiate either `Runner` (single) or `Coordinator` (multi)
  - Tests: single-stack mode (backward compat), multi-stack coordination, graceful shutdown, error isolation

## Phase 4 — Core Logic
- [x] SS-009: Service health evaluation + config/secret drift detection
  - Create `internal/health` package with models:
    - `ServiceStatus` (`OK`, `DEGRADED`, `FAILED`)
    - `ServiceHealth` (service name, status, reasons, drift details)
    - `StackHealth` (summary status + per-service map)
    - `DriftDetail` (kind: `MISSING`, `EXTRA`, `EXTRA_SERVICE`)
  - Evaluate per-service health by comparing `compose.DesiredState` vs `swarm.ActualState`:
    - Service in desired but missing in actual → `FAILED` (reason: missing service)
    - Service in actual but missing in desired (only when stack-scoped) → `DEGRADED` + `EXTRA_SERVICE` drift detail
      - When not stack-scoped, ignore services not in the compose file
    - Replicas: replicated mode compare desired vs running; global mode compare desired (Swarm) vs running
      - Running == 0 and desired > 0 → `FAILED`
      - Running < desired and > 0 → `DEGRADED`
      - Running > desired → `DEGRADED`
    - Image: compare desired vs actual using `swarm.NormalizeImage`; mismatch → `DEGRADED`
    - Configs/secrets: compare desired list vs actual list (exact name match only)
      - Missing config/secret → `FAILED`
      - Extra config/secret → `DEGRADED`
  - Aggregate stack health (FAILED > DEGRADED > OK) for logging/summary (alerts are service-level)
  - Include drift details in health output for later alert payload shaping
  - Tests: per-rule unit tests (replicas, image, missing/extra services, missing configs/secrets)
  - Tests: drift-type classification for missing/extra config/secret
- [x] SS-010: State persistence
  - Add `SS_STATE_PATH` config (default `/var/lib/swarm-sentinel/state.json`)
  - Create `internal/state` package with a store interface designed for future SQLite backing
  - Persist per-stack snapshots: desired fingerprint, service health map, last evaluation time
  - JSON file store implementation:
    - Ensure directory exists; write atomically (temp + rename)
    - Handle missing/corrupt state by starting fresh (log warning)
  - Tests: read/write roundtrip, missing file, corrupted JSON, multi-stack separation
- [x] SS-011: Transition detection
  - Compare current service health against persisted snapshots; emit events for transitions
  - Service-level transitions only (stack health used for summary/logging)
  - First run: emit alerts only for non-OK services; OK services are silent
  - Include change details in transition payloads (replica deltas, image mismatch, drift details)
  - Tests: first-run behavior, no-op transitions, mixed-service transitions

## Phase 5 — Outputs
- [x] SS-012: Slack notifier
  - Add `internal/notify` package with a `Notifier` interface and Slack implementation
  - Treat missing/empty `SS_SLACK_WEBHOOK_URL` as a no-op notifier (log once at startup)
  - Build Slack payloads from transition events:
    - Include stack name, service name, status, reasons, and timestamps
    - Include change details: replica delta, image mismatch, config/secret drift
    - Use blocks with a fallback `text` field for clients that do not render blocks
  - Batch transitions per stack per cycle to reduce message volume; include a count summary
  - Use `github.com/slack-go/slack` to build webhook payloads
  - Use `github.com/cenkalti/backoff/v4` for capped exponential backoff on transient failures
  - Use `github.com/hashicorp/go-retryablehttp` for HTTP client timeouts and retryable request handling
  - Handle 429 with `Retry-After`, using the backoff policy when no retry hint is present
  - Add a `golang.org/x/time/rate` limiter to throttle outbound messages per stack
  - Log Slack delivery failures but do not fail the cycle
  - Tests: payload formatting, batching, retry/backoff, rate-limit handling
- [x] SS-013: Logging and errors
  - Define log levels and standard fields for runner cycles (stack, fingerprint, counts)
  - Emit per-cycle summary log: services evaluated, OK/DEGRADED/FAILED counts, transition count
  - Emit per-transition log entries with service name, status change, and drift summary
  - Categorize errors: fatal (config/initialization) vs runtime (fetch/parse/swarm/notifier/state) and continue on runtime errors
  - Wrap errors with context and surface root causes with `errors.Is/As`
  - Tests: error classification behavior and log output for runtime failures (table-driven)

## Phase 5.5 — Production Hardening
> **Priority:** Must-have items block production deployment. Should-have items significantly improve operations.

- [x] SS-013.5: Docker API resilience *(must-have)*
  - Add retry wrapper for Swarm API calls (`ServiceList`, `TaskList`, `Ping`)
  - Use exponential backoff (1s, 2s, 4s) with max 3 attempts
  - Classify errors: retryable (timeout, connection reset, leader election) vs permanent (auth failure)
  - Log retry attempts with attempt count for debugging
  - Tests: mock transient failures, verify retry behavior

- [x] SS-013.6: Alert stabilization *(must-have)*
  - Add `SS_ALERT_STABILIZATION_CYCLES` config (default: 2)
  - Only emit transition after N consecutive cycles in same state
  - Prevents alert spam during rolling updates and transient issues
  - First-run behavior unchanged (alert immediately on non-OK)
  - Track `consecutiveCycles` per service in state snapshot
  - Tests: verify stabilization delays alerts, immediate alert on first run

- [x] SS-017: Health endpoint *(must-have, promoted from Phase 6)*
  - Add `SS_HEALTH_PORT` config (default: 8080, 0 to disable)
  - `GET /healthz`: Returns 200 if last cycle completed within 2× poll interval
  - `GET /readyz`: Returns 200 after first successful cycle completes
  - Response body: JSON with `last_cycle_time`, `cycle_duration_ms`, `stacks_evaluated`
  - Expose as Swarm healthcheck target
  - Tests: endpoint responses, failure conditions

- [x] SS-018: Prometheus metrics *(should-have)*
  - Add `SS_METRICS_PORT` config (default: 9090, 0 to disable; can share port with health)
  - Expose `/metrics` endpoint with:
    - `swarm_sentinel_cycle_duration_seconds` (histogram)
    - `swarm_sentinel_services_total{stack, status}` (gauge)
    - `swarm_sentinel_alerts_total{stack, severity}` (counter)
    - `swarm_sentinel_docker_api_errors_total` (counter)
    - `swarm_sentinel_last_successful_cycle_timestamp` (gauge)
  - Use `github.com/prometheus/client_golang`
  - Tests: verify metric registration and updates

- [x] SS-013.7: Update-aware health evaluation *(should-have)*
  - Check service `UpdateStatus.State` before evaluating replicas
  - During `updating`/`rollback_started`: suppress replica-count alerts
  - Still alert on: image drift, missing configs/secrets, complete failure (0 replicas)
  - Log update state for traceability
  - Tests: mock services mid-update, verify suppression

- [x] SS-019: Generic webhook notifier *(nice-to-have)*
  - Add `SS_WEBHOOK_URL` and `SS_WEBHOOK_TEMPLATE` config
  - Template supports Go text/template with transition data
  - Default template: JSON payload with all transition fields
  - Allows integration with PagerDuty, Opsgenie, custom systems
  - Share rate limiting and retry logic with Slack notifier
  - Tests: template rendering, delivery, error handling

- [x] SS-020: Dry-run mode *(nice-to-have)*
  - Add `SS_DRY_RUN` config (default: false)
  - When enabled: evaluate health, log transitions, skip actual notifications
  - Log "[DRY-RUN] Would notify: ..." for each suppressed alert
  - Useful for validating config before enabling alerts
  - Tests: verify no notifications sent in dry-run

## Phase 6 — Packaging
- [ ] SS-014: Dockerfile
- [ ] SS-015: Swarm deployment example
- [ ] SS-016: Documentation
  - Document scope: service-level monitoring only (networks/volumes out of scope)
  - Document config/secret comparison is name-based (not content-based)
  - Document image comparison uses tags (digest-pinned workflows may need enhancement)
