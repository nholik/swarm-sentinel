# Task Breakdown (v1)

## Phase 1 — Scaffolding
- [x] SS-001: Initialize Go module and logging
  - Create `go.mod` with module path from repository
  - Add `cmd/swarm-sentinel/main.go` with a minimal `main` entrypoint
  - Create `internal/logging` package with a small zerolog wrapper
  - Add `.gitignore` for Go build artifacts
  - Add `Makefile` targets: `build`, `test`, `lint` (placeholders ok)
- [x] SS-002: Configuration loading
  - Define config schema (env + `.env`): poll interval, compose URL, Slack webhook, Docker proxy URL
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
- [ ] SS-004: Fetch remote compose file
- [ ] SS-005: Fingerprinting desired state
- [ ] SS-006: Compose parsing
  - Parse service definitions: `image`, `replicas`
  - Parse `configs` block per service (list of config names)
  - Parse `secrets` block per service (list of secret names)
  - Normalize external vs inline config/secret references
  - Tests: validate parsing of configs/secrets from sample compose files

## Phase 3 — Swarm Integration
> **Note:** Consider validating Docker connection (SS-007) early to fail fast before compose fetching.
- [ ] SS-007: Docker client via socket proxy
- [ ] SS-008: Actual state collection

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
