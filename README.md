# Flight Tracker Vietnam

Real-time flight tracking dashboard for Vietnamese airspace, built with Go microservices, gRPC, WebSocket and React + TypeScript.

> Work in progress. This README will grow into the full project write-up (architecture diagram, demo GIF, design decisions).

## Architecture (planned)

| Component | Role | Status |
|---|---|---|
| `services/ingest` | Polls the [OpenSky Network](https://opensky-network.org) API for aircraft over Vietnam, or replays a recording | ✅ v0 |
| `services/tracking` | Stores latest positions (Redis) and history (PostgreSQL), streams updates over gRPC | ⏳ |
| `services/gateway` | REST + WebSocket API for browsers, talks gRPC to `tracking` | ⏳ |
| `proto/` | Protobuf contracts shared by the services | ⏳ |
| `web/` | React + TypeScript map, flight list and charts | ⏳ |

## Getting started

Requirements: Go 1.23+, Docker (for PostgreSQL/Redis).

```bash
cp .env.example .env          # optionally add OpenSky API credentials
docker compose up -d          # PostgreSQL + Redis
go test -race ./...
```

### Ingest service

```bash
# Live data (anonymous access works but is heavily rate limited)
go run ./services/ingest

# Record ~1-2 hours of live data for offline development and demos
go run ./services/ingest -record data/recordings/vn.jsonl

# Replay a recording, 10x faster, forever
go run ./services/ingest -mode replay -replay-file data/recordings/vn.jsonl -replay-speed 10 -replay-loop

# Per-aircraft logs
go run ./services/ingest -log-level debug
```

Every flag can also be set with an `INGEST_*` environment variable (see `.env.example`); flags take precedence.
Run `go run ./services/ingest -h` for the full list.

Replay shifts timestamps so the recording appears to happen *now*, so downstream services cannot tell it apart from live data and demos never depend on the OpenSky API being available.
