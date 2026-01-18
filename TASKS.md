# Task Breakdown (v1)

## Phase 1 — Scaffolding
- SS-001: Initialize Go module and logging
  - Create `go.mod` with module path from repository
  - Add `cmd/swarm-sentinel/main.go` with a minimal `main` entrypoint
  - Create `internal/logging` package with a small zerolog wrapper
  - Add `.gitignore` for Go build artifacts
  - Add `Makefile` targets: `build`, `test`, `lint` (placeholders ok)
- SS-002: Configuration loading
  - Define config schema (env + `.env`): poll interval, compose URL, Slack webhook, Docker proxy URL
  - Add `internal/config` package to load config with defaults and validation
  - Load `.env` via `github.com/joho/godotenv` if present, but prefer injected env vars (env wins over `.env`)
  - Ensure config loading is the first step in `main` and logs effective settings (redact secrets)
  - Tests: table-driven unit tests for parsing/validation (missing/invalid values, defaults)
  - Tests: integration-style test for `.env` loading with env overrides
- SS-003: Main execution loop
  - Define core loop cadence using the configured poll interval
  - Add a `Runner` (or similar) struct to orchestrate fetch/compare/notify steps
  - Ensure graceful shutdown hooks (context cancellation / signal handling)
  - Add a single-cycle `RunOnce` method to simplify testing
  - Tests: verify loop timing behavior with a mock clock or injected ticker
  - Tests: verify shutdown stops the loop cleanly

## Phase 2 — Inputs
- SS-004: Fetch remote compose file
- SS-005: Fingerprinting desired state
- SS-006: Compose parsing

## Phase 3 — Swarm Integration
- SS-007: Docker client via socket proxy
- SS-008: Actual state collection

## Phase 4 — Core Logic
- SS-009: Health evaluation engine
- SS-010: State persistence
- SS-011: Transition detection

## Phase 5 — Outputs
- SS-012: Slack notifier
- SS-013: Logging and errors

## Phase 6 — Packaging
- SS-014: Dockerfile
- SS-015: Swarm deployment example
- SS-016: Documentation
