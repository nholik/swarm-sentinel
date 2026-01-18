# swarm-sentinel

**swarm-sentinel** is a lightweight health and drift sentinel for Docker Swarm.

It periodically compares:
- **Desired state** (from a rendered docker-compose file stored remotely)
- **Actual runtime state** (from the Docker Swarm API)

and emits alerts when the cluster diverges from what *should* be running.

## Core Contract (v1)

swarm-sentinel periodically polls a remotely stored docker-compose file that represents the desired state.
The compose file is fully rendered; no template or environment interpolation is performed by swarm-sentinel.

That compose file is the contract.

Swarm access uses the official Docker Go SDK against a configurable Docker API host.
A read-only socket proxy is recommended but configured externally (https://github.com/Tecnativa/docker-socket-proxy).

## Configuration

swarm-sentinel supports two modes: **single-stack** (simple) and **multi-stack** (advanced).

### Single-Stack Mode (Default)

Monitor one compose file mapped to one stack:

```bash
SS_COMPOSE_URL=https://example.com/prod/docker-compose.yml
SS_STACK_NAME=prod  # optional; empty means all services
```

Use this for simple deployments where a single sentinel instance watches one stack.
When `SS_STACK_NAME` is empty, swarm-sentinel still evaluates only the services
defined in the compose file; other Swarm services are ignored for health.

### Multi-Stack Mode (Swarm-Native)

Monitor multiple compose files mapped to multiple stacks using a YAML mapping file:

```bash
# Mapping file location (auto-detected in order):
# 1. SS_COMPOSE_MAPPING_FILE env var
# 2. /run/configs/compose-mapping.yaml (Swarm config mount)
# 3. /run/secrets/compose-mapping.yaml (Swarm secret mount)
# 4. ./compose-mapping.yaml (local development)
```

**Mapping file format** (`compose-mapping.yaml`):

```yaml
stacks:
  - name: prod
    compose_url: https://example.com/prod/compose.yml
    timeout: 20s  # optional; overrides SS_COMPOSE_TIMEOUT
    
  - name: staging
    compose_url: https://example.com/staging/compose.yml
    
  - name: monitoring
    compose_url: https://example.com/monitoring/compose.yml
```

Each stack runs independently with isolated health tracking and state management.

### Environment Variables (Shared Settings)

- `SS_COMPOSE_TIMEOUT` (optional): Default HTTP timeout for fetching compose files (default: `10s`)
- `SS_POLL_INTERVAL` (optional): Poll interval for all sentinel cycles (default: `30s`)
- `SS_DOCKER_PROXY_URL` (optional): Docker API URL (socket proxy recommended, default: `http://localhost:2375`)
- `SS_DOCKER_API_TIMEOUT` (optional): Timeout for Docker API calls (default: `30s`)
- `SS_STATE_PATH` (optional): Path to persisted state JSON for transitions (default: `/var/lib/swarm-sentinel/state.json`)
- `SS_LOG_LEVEL` (optional): Log level - trace, debug, info, warn, error, fatal, panic (default: `info`)
- `SS_DOCKER_TLS_VERIFY` (optional): Enable TLS when connecting to the Docker API host (default: `false`)
- `SS_DOCKER_TLS_CA` (optional): Path to CA certificate for Docker API TLS
- `SS_DOCKER_TLS_CERT` (optional): Path to client certificate for Docker API TLS
- `SS_DOCKER_TLS_KEY` (optional): Path to client key for Docker API TLS
- `SS_SLACK_WEBHOOK_URL` (optional): Slack webhook URL for alerts
- `SS_COMPOSE_MAPPING_FILE` (optional): Path to YAML mapping file for multi-stack mode

**Single-Stack Only:**
- `SS_COMPOSE_URL` (required in single-stack mode): URL to the rendered `docker-compose.yml`
- `SS_STACK_NAME` (optional): Swarm stack name used to scope services (empty means all services)

`DOCKER_TLS_VERIFY` and `DOCKER_CERT_PATH` are honored as fallbacks for TLS settings.

---

## Docker Swarm Deployment

### Single-Stack Mode Example

```yaml
version: '3.9'

services:
  sentinel:
    image: swarm-sentinel:latest
    environment:
      SS_COMPOSE_URL: https://artifact.example.com/prod/compose.yml
      SS_STACK_NAME: prod
      SS_DOCKER_PROXY_URL: http://docker-socket-proxy:2375
      SS_POLL_INTERVAL: 30s
      SS_LOG_LEVEL: info
    deploy:
      placement:
        constraints: [node.role == manager]
      replicas: 1
```

### Multi-Stack Mode Example

**1. Create the mapping config:**

```bash
docker config create compose-mapping ./stacks.yaml
```

**2. Deploy sentinel with mapping mounted:**

```yaml
version: '3.9'

services:
  docker-socket-proxy:
    image: tecnativa/docker-socket-proxy:latest
    ports:
      - "2375:2375"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    environment:
      SERVICES: 1
      TASKS: 1
      INFO: 1
    deploy:
      placement:
        constraints: [node.role == manager]

  sentinel:
    image: swarm-sentinel:latest
    environment:
      SS_DOCKER_PROXY_URL: http://docker-socket-proxy:2375
      SS_POLL_INTERVAL: 30s
      SS_LOG_LEVEL: info
      # Mapping file auto-detected at /run/configs/compose-mapping.yaml
    configs:
      - compose-mapping
    depends_on:
      - docker-socket-proxy
    deploy:
      placement:
        constraints: [node.role == manager]
      replicas: 1

configs:
  compose-mapping:
    file: ./stacks.yaml
```

**3. Update stacks (rotate config):**

```bash
# Edit stacks.yaml
vim stacks.yaml

# Create new config
docker config create compose-mapping ./stacks.yaml

# Force service to redeploy (picks up new config)
docker service update --force sentinel
```

### Why Swarm Configs for Multi-Stack Mode

- **Centralized:** Config stored in Swarm, not in env vars
- **Version-controlled:** Easy to track config changes
- **Non-destructive:** Update without redeploying sentinel service itself
- **Secure:** Configs are encrypted at rest in Swarm
- **Auditable:** Swarm logs config creation/update events
