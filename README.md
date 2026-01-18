# swarm-sentinel

**swarm-sentinel** is a lightweight health and drift sentinel for Docker Swarm.

It periodically compares:
- **Desired state** (from a rendered docker-compose file stored remotely)
- **Actual runtime state** (from the Docker Swarm API)

and emits alerts when the cluster diverges from what *should* be running.

## Core Contract (v1)

swarm-sentinel periodically polls a remotely stored docker-compose file that represents the desired state.

That compose file is the contract.

## Configuration

Environment variables:

- `SS_COMPOSE_URL` (required): URL to the rendered `docker-compose.yml`
- `SS_COMPOSE_TIMEOUT` (optional): HTTP timeout for fetching the compose file (default: `10s`)
- `SS_POLL_INTERVAL` (optional): Poll interval for sentinel cycles (default: `30s`)
- `SS_DOCKER_PROXY_URL` (optional): Docker socket proxy URL (default: `http://localhost:2375`)
- `SS_SLACK_WEBHOOK_URL` (optional): Slack webhook URL for alerts
