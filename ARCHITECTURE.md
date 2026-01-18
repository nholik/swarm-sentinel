# Architecture Overview

## Purpose

**swarm-sentinel** is a lightweight, read-only monitoring service for Docker Swarm.  
Its sole responsibility is to detect divergence between **desired state** and **actual runtime state**, and emit alerts when health transitions occur.

The system is intentionally polling-based, state-aware, and decoupled from deployment tooling.

## High-Level Architecture

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
│  │  │ docker-socket-proxy           │       │   │             │
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
