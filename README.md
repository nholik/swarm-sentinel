# swarm-sentinel

**swarm-sentinel** is a lightweight health and drift sentinel for Docker Swarm.

It periodically compares:
- **Desired state** (from a rendered docker-compose file stored remotely)
- **Actual runtime state** (from the Docker Swarm API)

and emits alerts when the cluster diverges from what *should* be running.

## Core Contract (v1)

swarm-sentinel periodically polls a remotely stored docker-compose file that represents the desired state.

That compose file is the contract.
