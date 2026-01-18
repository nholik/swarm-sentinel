# Architecture Overview

## Purpose

**swarm-sentinel** is a lightweight, read-only monitoring service for Docker Swarm.  
Its sole responsibility is to detect divergence between **desired state** and **actual runtime state**, and emit alerts when health transitions occur.

The system is intentionally polling-based, state-aware, and decoupled from deployment tooling.

## High-Level Architecture

swarm-sentinel only needs a Docker API host. A socket proxy is optional and configured externally (https://github.com/Tecnativa/docker-socket-proxy).

```text
┌───────────────────────────────────────────────────────────────┐
│                     Docker Swarm Cluster                      │
│                                                               │
│  ┌─────────────── Manager Node ─────────────────┐             │
│  │                                              │             │
│  │  ┌──────────────────────────────┐            │             │
│  │  │ Docker Daemon                │            │             │
│  │  │                              │            │             │
│  │  │  /var/run/docker.sock        │ ◄──────┐   │             │
│  │  └──────────────────────────────┘        │   │             │
│  │                                          │   │             │
│  │  ┌───────────────────────────────┐       │   │             │
│  │  │ docker-socket-proxy (optional)│       │   │             │
│  │  │ (read-only API filter)        │       │   │             │
│  │  │                               │       │   │             │
│  │  │  SERVICES=1                   │       │   │             │
│  │  │  TASKS=1                      │       │   │             │
│  │  │  INFO=1                       │       │   │             │
│  │  │  POST=0                       │       │   │             │
│  │  │                               │       │   │             │
│  │  │  tcp://proxy:2375             │───────┘   │             │
│  │  └───────────────────────────────┘           │             │
│  │               ▲                              │             │
│  │               │ HTTP (private overlay net)   │             │
│  │               │                              │             │
│  │  ┌──────────────────────────────┐            │             │
│  │  │ swarm-sentinel               │            │             │
│  │  │ (Go static binary)           │            │             │
│  │  │                              │            │             │
│  │  │  - Polls desired state       │            │             │
│  │  │  - Reads Swarm state         │            │             │
│  │  │  - Evaluates health          │            │             │
│  │  │  - Persists last state       │            │             │
│  │  │  - Sends Slack alerts        │            │             │
│  │  └──────────────────────────────┘            │             │
│  │                                              │             │
│  └──────────────────────────────────────────────┘             │
│                                                               │
└───────────────────────────────────────────────────────────────┘
```

## Design Principles

### 1. Polling over Events

- swarm-sentinel polls desired state and actual state periodically
- Actual state is collected every cycle, even when desired state is unchanged
- No webhooks, CI listeners, or push-based triggers
- Avoids missed events and brittle integrations

### 2. Rendered Compose as the Contract

- swarm-sentinel consumes `docker-compose.yml`
- It does not parse templates, manifests, or generators
- The compose file is fully rendered; no environment interpolation is performed
- Upstream complexity is handled at deploy time

### 3. Read-Only by Construction

- Uses read-only Docker API endpoints only
- A read-only socket proxy is recommended but external to the project (https://github.com/Tecnativa/docker-socket-proxy)
- Direct `/var/run/docker.sock` access is not required

### 4. Single Observer

- Exactly one replica
- Manager-only placement
- Swarm handles failover

### 5. Stateful Only Where Necessary

- Persist last-known health + desired-state fingerprint
- Alert only on transitions
- Avoid alert storms on restart

---

## Desired State Model (v1)

Desired state is a remotely stored `docker-compose.yml`, typically produced during deploy and stored as an artifact.

**Example source:** GitLab job artifacts (`latest/docker-compose.yml`)

**swarm-sentinel behavior:**

1. Downloads the file
2. Computes a SHA-256 fingerprint
3. Parses a limited subset of compose fields
4. Treats it as the authoritative desired state

### Tracked Fields per Service

| Field       | Description                                      |
|-------------|--------------------------------------------------|
| `image`     | Expected image reference (registry/name:tag)     |
| `replicas`  | Desired replica count                            |
| `configs`   | List of config names attached to the service     |
| `secrets`   | List of secret names attached to the service     |

### Config and Secret Naming Convention

swarm-sentinel relies on an **external naming convention** for configs and secrets:

```
<name>_v<version>
```

**Examples:**
- `app_config_v3`
- `db_password_v12`

This convention is **not enforced** by swarm-sentinel but is expected to be followed by deployment tooling. swarm-sentinel compares the **exact name** from the compose file against the names attached to running services in Swarm.

---

## Actual State Model

- Read from Docker Swarm API via a configurable Docker API host (proxy optional)
- Services and tasks only
- No mutation or exec capabilities
- Optional stack scoping via `SS_STACK_NAME` (empty means all services)

---

## Health Evaluation

Each service is classified as one of:

| Status     | Description                          |
|------------|--------------------------------------|
| `OK`       | All replicas running as expected     |
| `DEGRADED` | Partial availability                 |
| `FAILED`   | No healthy replicas                  |

Rule ordering is deterministic and explicit.

**Overall stack health:**

- Any `FAILED` → stack `FAILED`
- Else any `DEGRADED` → stack `DEGRADED`
- Else `OK`

---

## Config and Secret Drift Detection

### Problem

Docker Swarm does not automatically update configs/secrets on running services when new versions are deployed. A service may continue running with stale configuration even after a stack deploy if the config/secret reference wasn't updated.

### Approach

swarm-sentinel compares:

| Source         | Data                                          |
|----------------|-----------------------------------------------|
| **Desired**    | Config/secret names from `docker-compose.yml` |
| **Actual**     | Config/secret names attached to Swarm tasks   |

### Drift Types

| Type              | Description                                           |
|-------------------|-------------------------------------------------------|
| `VERSION_MISMATCH`| Attached config/secret has different version suffix   |
| `MISSING`         | Expected config/secret not attached to service        |
| `EXTRA`           | Unexpected config/secret attached (not in compose)    |

### Health Impact

Config/secret drift contributes to service health:

- Any drift → service marked as `DEGRADED` (configurable)
- Critical secrets missing → service marked as `FAILED` (optional)

### Limitations

- swarm-sentinel does **not** inspect config/secret **content**
- Only names are compared
- Relies on naming convention being followed upstream

---

## Alerting

Alerts are sent **only on health transitions**:

| Transition                   | Alert Type |
|------------------------------|------------|
| `OK` → `DEGRADED` / `FAILED` | Failure    |
| `FAILED` / `DEGRADED` → `OK` | Recovery   |

No repeated alerts for unchanged state.

---

## See Also

- [ROADMAP.md](ROADMAP.md) – Planned features and milestones
- [README.md](README.md) – Quick start and usage
