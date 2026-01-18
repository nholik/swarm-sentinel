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
