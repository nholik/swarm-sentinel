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
- [ ] SS-009: Health evaluation engine
  - Evaluate replica count: desired vs running
  - Evaluate image: expected vs deployed
  - Evaluate config/secret attachment (see SS-009a)
  - Aggregate service health into stack health
- [ ] SS-009a: Config/secret drift detection
  - Compare desired config names (from compose) vs actual (from Swarm API)
  - Compare desired secret names (from compose) vs actual (from Swarm API)
  - Detect drift types: `VERSION_MISMATCH`, `MISSING`, `EXTRA`
  - Flag drift in service health status
  - Include drift details in alert payloads
  - Tests: unit tests for each drift type scenario
  - Tests: integration test with mock Swarm API responses
- [ ] SS-010: State persistence
- [ ] SS-011: Transition detection

## Phase 5 — Outputs
- [ ] SS-012: Slack notifier
- [ ] SS-013: Logging and errors

## Phase 6 — Packaging
- [ ] SS-014: Dockerfile
- [ ] SS-015: Swarm deployment example
- [ ] SS-016: Documentation
- [ ] SS-017: Health endpoint
  - Add HTTP `/healthz` endpoint for Swarm's own health checks on the sentinel service
  - Return 200 OK when the service is running and can reach dependencies
