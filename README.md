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

## Environment Variables Reference

### Core Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `SS_POLL_INTERVAL` | `30s` | How often to evaluate stack health |
| `SS_LOG_LEVEL` | `info` | Log level: trace, debug, info, warn, error, fatal, panic |
| `SS_DRY_RUN` | `false` | When true, log alerts but don't send notifications |

### Compose Source (Single-Stack Mode)

| Variable | Default | Description |
|----------|---------|-------------|
| `SS_COMPOSE_URL` | *(required)* | URL to fetch the rendered docker-compose.yml |
| `SS_COMPOSE_TIMEOUT` | `10s` | HTTP timeout for fetching compose files |
| `SS_STACK_NAME` | *(empty)* | Swarm stack name to scope services; empty means all services in compose |

### Compose Source (Multi-Stack Mode)

| Variable | Default | Description |
|----------|---------|-------------|
| `SS_COMPOSE_MAPPING_FILE` | *(auto-detected)* | Path to YAML mapping file; auto-detects Swarm config/secret mounts |

### Docker API

| Variable | Default | Description |
|----------|---------|-------------|
| `SS_DOCKER_PROXY_URL` | `http://localhost:2375` | Docker API endpoint (socket proxy recommended) |
| `SS_DOCKER_API_TIMEOUT` | `30s` | Timeout for Docker API calls |
| `SS_DOCKER_TLS_VERIFY` | `false` | Enable TLS verification for Docker API |
| `SS_DOCKER_TLS_CA` | *(empty)* | Path to CA certificate for Docker API TLS |
| `SS_DOCKER_TLS_CERT` | *(empty)* | Path to client certificate for Docker API TLS |
| `SS_DOCKER_TLS_KEY` | *(empty)* | Path to client key for Docker API TLS |

**Compatibility:** `DOCKER_TLS_VERIFY` and `DOCKER_CERT_PATH` are honored as fallbacks.

**TLS example (connecting to a remote Docker host):**

```bash
SS_DOCKER_PROXY_URL=https://docker.example.com:2376
SS_DOCKER_TLS_VERIFY=true
SS_DOCKER_TLS_CA=/run/secrets/docker-ca.pem
SS_DOCKER_TLS_CERT=/run/secrets/docker-cert.pem
SS_DOCKER_TLS_KEY=/run/secrets/docker-key.pem
```

### Notifications

| Variable | Default | Description |
|----------|---------|-------------|
| `SS_SLACK_WEBHOOK_URL` | *(empty)* | Slack incoming webhook URL; empty disables Slack |
| `SS_WEBHOOK_URL` | *(empty)* | Generic webhook URL for custom integrations |
| `SS_WEBHOOK_TEMPLATE` | *(JSON)* | Go text/template for webhook payload |

**Secret files with `_FILE` suffix:**

Any configuration variable can be loaded from a file by appending `_FILE` to the variable name.
This is the recommended approach for secrets in Docker Swarm:

```bash
# Instead of setting the secret directly (not recommended):
SS_SLACK_WEBHOOK_URL=https://hooks.slack.com/services/T.../B.../xxx

# Mount as a Swarm secret and reference the file:
SS_SLACK_WEBHOOK_URL_FILE=/run/secrets/slack-webhook
```

The `_FILE` variant takes precedence over the direct variable if both are set.

**Webhook template example:**

The `SS_WEBHOOK_TEMPLATE` variable accepts a Go text/template. Available fields:

```go-template
{
  "stack": "{{.StackName}}",
  "service": "{{.ServiceName}}",
  "status": "{{.Status}}",
  "previous_status": "{{.PreviousStatus}}",
  "reasons": [{{range $i, $r := .Reasons}}{{if $i}},{{end}}"{{$r}}"{{end}}],
  "timestamp": "{{.Timestamp}}"
}
```

Example for PagerDuty:

```bash
SS_WEBHOOK_TEMPLATE='{"routing_key":"{{.RoutingKey}}","event_action":"trigger","payload":{"summary":"{{.StackName}}/{{.ServiceName}}: {{.Status}}","severity":"{{if eq .Status "FAILED"}}critical{{else}}warning{{end}}","source":"swarm-sentinel"}}'
```

### Alert Behavior

| Variable | Default | Description |
|----------|---------|-------------|
| `SS_ALERT_STABILIZATION_CYCLES` | `2` | Consecutive cycles in same state before alerting |

### State Persistence

| Variable | Default | Description |
|----------|---------|-------------|
| `SS_STATE_PATH` | `/home/nonroot/state.json` | Path to persisted state file (distroless image) |

**Note:** The Docker image uses `gcr.io/distroless/static:nonroot` as the base. The nonroot user's
home directory `/home/nonroot` is used for state persistence. Mount a volume to this path to
preserve state across container restarts.

### Observability

| Variable | Default | Description |
|----------|---------|-------------|
| `SS_HEALTH_PORT` | `8080` | Port for health endpoints; 0 to disable |
| `SS_METRICS_PORT` | `9090` | Port for Prometheus metrics; 0 to disable |

### Health Endpoints

- `GET /healthz` - Returns 200 if last cycle completed within 2× poll interval
- `GET /readyz` - Returns 200 after first successful cycle completes

### Prometheus Metrics

- `swarm_sentinel_cycle_duration_seconds` - Histogram of evaluation cycle duration
- `swarm_sentinel_services_total{stack, status}` - Gauge of services by status
- `swarm_sentinel_alerts_total{stack, severity}` - Counter of alerts emitted
- `swarm_sentinel_docker_api_errors_total` - Counter of Docker API failures
- `swarm_sentinel_last_successful_cycle_timestamp` - Unix timestamp of last success

**Scrape example:**

```yaml
scrape_configs:
  - job_name: swarm-sentinel
    static_configs:
      - targets: ["sentinel:9090"]
```

## Security Considerations

### SSRF Protection

Compose URLs are validated to block cloud provider metadata endpoints:
- AWS/GCP/Azure metadata (169.254.169.254)
- GCP metadata.google.internal
- All link-local addresses (169.254.x.x)

### State File Permissions

The state file is created with mode 0600 to protect infrastructure details.

### Secret Handling

Webhook URLs are never logged; only "set" or "unset" is shown in startup logs.

---

## Docker Image

The Docker image uses a multi-stage build with `gcr.io/distroless/static:nonroot` as the runtime base.
This provides a minimal attack surface with no shell, package manager, or other utilities.

### Build locally

```bash
docker build -t swarm-sentinel:local .

# With version info embedded in the binary:
docker build --build-arg VERSION=v1.0.0 -t swarm-sentinel:v1.0.0 .
```

### Multi-arch build (buildx)

```bash
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --build-arg VERSION=v1.0.0 \
  -t swarm-sentinel:latest \
  -t swarm-sentinel:v1.0.0 \
  --push \
  .
```

### Run locally

```bash
docker run --rm \
  -e SS_COMPOSE_URL=https://artifacts.example.com/prod/compose.yml \
  -e SS_DOCKER_PROXY_URL=http://host.docker.internal:2375 \
  -e SS_HEALTH_PORT=8080 \
  -e SS_METRICS_PORT=9090 \
  -v swarm-sentinel-state:/home/nonroot \
  -p 8080:8080 -p 9090:9090 \
  swarm-sentinel:local
```

## State Persistence

- State is stored in `SS_STATE_PATH` (default: `/home/nonroot/state.json`) inside the container.
- Mount a volume to `/home/nonroot` to preserve state across container restarts and upgrades.
- Keep the volume between upgrades to preserve alert stabilization and transition history.
- To reset state, stop the service and remove the state file or replace the volume.

**Upgrade notes:**
- State file format is backward compatible within the same major version.
- If upgrading from an older version that used `/var/lib/swarm-sentinel`, migrate the state file
  or start fresh (sentinel will re-evaluate all services on first run).

## Docker Swarm Deployment

### Prerequisites

1. A Docker Swarm cluster with at least one manager node
2. A socket proxy (recommended) or direct Docker socket access
   - Required proxy permissions: `SERVICES=1`, `TASKS=1`, `INFO=1`, `PING=1`
3. Compose files accessible via HTTP(S)

### Deployment Examples

- Single-stack: `deploy/single-stack.yaml`
- Multi-stack: `deploy/multi-stack.yaml`
- Mapping file: `deploy/compose-mapping.yaml`

### Single-Stack Mode Example

```yaml
# Deploy with: docker stack deploy -c single-stack.yaml sentinel
services:
  docker-socket-proxy:
    image: tecnativa/docker-socket-proxy:latest
    environment:
      SERVICES: 1
      TASKS: 1
      INFO: 1
      PING: 1
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    networks:
      - sentinel-internal
    deploy:
      placement:
        constraints: [node.role == manager]
      restart_policy:
        condition: on-failure
        delay: 5s
      resources:
        limits:
          memory: 64M

  sentinel:
    image: swarm-sentinel:latest
    environment:
      SS_COMPOSE_URL: https://artifacts.example.com/prod/compose.yml
      SS_STACK_NAME: prod
      SS_DOCKER_PROXY_URL: http://docker-socket-proxy:2375
      SS_POLL_INTERVAL: 30s
      SS_SLACK_WEBHOOK_URL_FILE: /run/secrets/slack-webhook
      SS_LOG_LEVEL: info
      SS_HEALTH_PORT: 8080
      SS_METRICS_PORT: 9090
    secrets:
      - slack-webhook
    volumes:
      - sentinel-state:/home/nonroot
    networks:
      - sentinel-internal
    deploy:
      placement:
        constraints: [node.role == manager]
      replicas: 1
      restart_policy:
        condition: on-failure
        delay: 5s
      resources:
        limits:
          memory: 128M
      healthcheck:
        test: ["/usr/local/bin/swarm-sentinel", "-healthcheck"]
        interval: 30s
        timeout: 5s
        retries: 3

networks:
  sentinel-internal:
    driver: overlay

volumes:
  sentinel-state:

secrets:
  slack-webhook:
    external: true
```

### Multi-Stack Mode Example (Production)

**1. Create the stack mapping config:**

```yaml
# deploy/compose-mapping.yaml
stacks:
  - name: prod
    compose_url: https://artifacts.example.com/prod/compose.yml
    timeout: 20s
    
  - name: staging
    compose_url: https://artifacts.example.com/staging/compose.yml
    
  - name: monitoring
    compose_url: https://artifacts.example.com/monitoring/compose.yml
```

```bash
docker config create compose-mapping deploy/compose-mapping.yaml
```

**2. Create the Slack webhook secret:**

```bash
echo "https://hooks.slack.com/services/T.../B.../xxx" | docker secret create slack-webhook -
```

**3. Deploy the stack:**

```yaml
# Deploy with: docker stack deploy -c multi-stack.yaml sentinel
services:
  docker-socket-proxy:
    image: tecnativa/docker-socket-proxy:latest
    environment:
      SERVICES: 1
      TASKS: 1
      INFO: 1
      PING: 1
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    networks:
      - sentinel-internal
    deploy:
      placement:
        constraints: [node.role == manager]
      restart_policy:
        condition: on-failure
        delay: 5s
      resources:
        limits:
          memory: 64M

  sentinel:
    image: swarm-sentinel:latest
    environment:
      SS_DOCKER_PROXY_URL: http://docker-socket-proxy:2375
      SS_POLL_INTERVAL: 30s
      SS_SLACK_WEBHOOK_URL_FILE: /run/secrets/slack-webhook
      SS_LOG_LEVEL: info
      SS_ALERT_STABILIZATION_CYCLES: 2
      SS_HEALTH_PORT: 8080
      SS_METRICS_PORT: 9090
    configs:
      - source: compose-mapping
        target: /run/configs/compose-mapping.yaml
    secrets:
      - slack-webhook
    volumes:
      # Higher memory: runs one runner per stack
      - sentinel-state:/home/nonroot
    networks:
      - sentinel-internal
    deploy:
      placement:
        constraints: [node.role == manager]
      replicas: 1
      restart_policy:
        condition: on-failure
        delay: 5s
      resources:
        limits:
          memory: 256M
      healthcheck:
        test: ["/usr/local/bin/swarm-sentinel", "-healthcheck"]
        interval: 30s
        timeout: 5s
        retries: 3

networks:
  sentinel-internal:
    driver: overlay

volumes:
  sentinel-state:

configs:
  compose-mapping:
    external: true

secrets:
  slack-webhook:
    external: true
```

**4. Update stacks (rotate config):**

```bash
# Edit deploy/compose-mapping.yaml
vim deploy/compose-mapping.yaml

# Create new config version
docker config create compose-mapping-v2 deploy/compose-mapping.yaml

# Update service to use new config
docker service update \
  --config-rm compose-mapping \
  --config-add source=compose-mapping-v2,target=/run/configs/compose-mapping.yaml \
  sentinel_sentinel
```

### Why Swarm Configs for Multi-Stack Mode

- **Centralized:** Config stored in Swarm, not in env vars
- **Version-controlled:** Easy to track config changes
- **Non-destructive:** Update without redeploying sentinel service itself
- **Secure:** Configs are encrypted at rest in Swarm
- **Auditable:** Swarm logs config creation/update events

## Monitoring Scope

### What swarm-sentinel monitors

- **Service existence**: Services defined in compose must exist in Swarm
- **Replica counts**: Running replicas vs desired (replicated and global modes)
- **Image versions**: Expected image tag vs deployed image
- **Configs/Secrets**: Attached configs and secrets (name-based, not content)
- **Service updates**: Awareness of rolling updates to suppress false positives

### What swarm-sentinel does NOT monitor

- **Networks/Volumes**: Infrastructure resources are out of scope
- **Config/Secret content**: Only names are compared, not actual content
- **Image digests**: Tag-based comparison; digest-pinned workflows may need enhancement
- **Node health**: Focus is on service health, not infrastructure

## Troubleshooting

### Common Issues

**"docker api unreachable"**
- Verify socket proxy is running and accessible
- Check `SS_DOCKER_PROXY_URL` is correct
- Ensure socket proxy has `SERVICES=1 TASKS=1 INFO=1 PING=1`

**"compose fetch failed"**
- Verify compose URL is accessible from the container
- Check for authentication requirements
- Review `SS_COMPOSE_TIMEOUT` if fetches are slow

**"state file corrupt"**
- Safe to delete; sentinel will start fresh
- Check disk space on state volume

### Debug Mode

Set `SS_LOG_LEVEL=debug` for verbose output including:
- Compose fingerprint changes
- Per-service health evaluation
- Notification delivery attempts
