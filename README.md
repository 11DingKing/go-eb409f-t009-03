# Ejina Microgrid Dispatch Center

A self-contained Go backend that models the dispatch operations of the Ejina
Banner (额济纳旗) power supply service center for a source-grid-load-storage
microgrid composed of grid-forming energy-storage battery cabins, PV arrays and
wind turbines.

It implements the five dispatch flows a duty officer runs daily, with the
concurrency boundaries, idempotency and failure-recovery rules the domain
requires:

1. **Inspection dispatch** — a certified inspector inspects a battery cabin
   inside the fixed daily window and uploads photos for traceability.
2. **Tiered anomaly reporting** — three severity levels; a Level-1 anomaly
   freezes the cabin and its inspections and must be acknowledged by the duty
   officer within 15 minutes, otherwise it is auto-escalated.
3. **Maintenance work-order closure** — a work order can only be closed after the
   assigned inspector reviews and signs it (closed-loop control); close is
   idempotent.
4. **Black-start execution** — restoration after total loss of supply requires
   dual authorization (duty officer + maintenance lead) before triggering.
5. **Off-grid / on-grid switching** — before connecting, the auto-synchronizer
   must confirm voltage, frequency and phase difference are within thresholds;
   on failure a fault code is recorded, the upper grid is notified, the system
   falls back to off-grid independent operation with the backup tie-line, and a
   retest can re-issue the connection.

**Concurrency boundary:** when a black start is executing and a Level-1 anomaly
is reported on a participating cabin that has not yet been energized, dispatch
locks that cabin, isolates the fault *first*, then resumes startup so the
faulted cabin is skipped — preventing energizing a faulted cabin.

## Project layout

```
cmd/ejinagrid        HTTP service entrypoint (wiring, seeding, graceful shutdown)
internal/clock       injectable time abstraction (system + fake for tests)
internal/domain      entities and state machines (cabin, inspection, anomaly,
                     work order, black start, grid sync)
internal/store       thread-safe in-memory persistence with JSON snapshot
internal/service     application orchestration, coordinator, concurrency
internal/api         HTTP handlers and routing (net/http ServeMux patterns)
internal/worker      background tasks: escalation, retest prep, snapshot
config/              example environment configuration
```

## Run locally

Requires Go 1.26.

```bash
go run ./cmd/ejinagrid
# -> ejinagrid listening on :51523 (data=./data/ejinagrid.json, inspection window 08:00-10:00)
```

The store is seeded with three cabins (`cabin-1/2/3`) on first start if empty.
State is snapshotted to `EJINA_DATA` (default `./data/ejinagrid.json`) and
reloaded on restart.

### Configuration (environment)

| Variable | Default | Meaning |
|---|---|---|
| `EJINA_ADDR` | `:51523` | Listen address |
| `EJINA_DATA` | `./data/ejinagrid.json` | Snapshot file path |
| `EJINA_INSPECTION_WINDOW_START` | `8` | Daily inspection window start hour |
| `EJINA_INSPECTION_WINDOW_END` | `10` | Daily inspection window end hour (exclusive) |
| `EJINA_TICK_SECONDS` | `30` | Background worker tick interval |

See `config/ejinagrid.example.env`.

## Port

The service listens on **51523** by default.

## Main HTTP API

All bodies are JSON. Path params use `{id}`. Errors map to status codes:
`404` not found, `409` duplicate, `422` invalid state/domain rule, `400` bad
request.

### Cabins
- `GET /api/cabins` — list cabins
- `GET /api/cabins/{id}` — cabin status
- `POST /api/cabins` — seed a cabin `{id,name,site_id,capacity_kwh}`

### Inspections (flow 1)
- `POST /api/inspections` — dispatch `{cabin_id,inspector_id,inspector_name,cert_no,cert_expiry}`
- `POST /api/inspections/{id}/start`
- `POST /api/inspections/{id}/photo` — `{ref}`
- `POST /api/inspections/{id}/review` — `{inspector_id,note}`
- `POST /api/inspections/{id}/complete`
- `GET /api/inspections`, `GET /api/inspections/{id}`

### Anomalies (flow 2)
- `POST /api/anomalies` — report `{cabin_id,level,description,reporter}` (level 1..3)
- `POST /api/anomalies/{id}/ack` — `{by}` (within 15 min for Level-1)
- `POST /api/anomalies/{id}/isolate` — `{fault_code}`
- `POST /api/anomalies/{id}/resolve`
- `GET /api/anomalies`, `GET /api/anomalies/{id}`

### Work orders (flow 3)
- `POST /api/workorders` — open `{cabin_id,anomaly_id,title,description,assignee,inspector_id,inspector_name,cert_no}`
- `POST /api/workorders/{id}/start`
- `POST /api/workorders/{id}/review` — `{inspector_id,note}` (must be before close)
- `POST /api/workorders/{id}/close` — idempotent
- `GET /api/workorders`, `GET /api/workorders/{id}`

### Black start (flow 4)
- `POST /api/blackstarts` — request `{cabin_ids,requester}`
- `POST /api/blackstarts/{id}/authorize` — `{role,principal}` (role: `duty_officer`/`maintenance_lead`, both required)
- `POST /api/blackstarts/{id}/execute`
- `POST /api/blackstarts/{id}/rollback` — `{reason}`
- `GET /api/blackstarts`, `GET /api/blackstarts/{id}`

### Grid sync (flow 5)
- `POST /api/gridsyncs` — initiate `{black_start_id?,max_voltage_delta_volts?,max_frequency_delta_hz?,max_phase_delta_deg?}`
- `POST /api/gridsyncs/{id}/sync` — begin synchronizer check
- `POST /api/gridsyncs/{id}/verify` — `{grid_voltage_volts,local_voltage_volts,grid_frequency_hz,local_frequency_hz,phase_delta_deg}`
- `POST /api/gridsyncs/{id}/connect` — connect after verify passes
- `POST /api/gridsyncs/{id}/retest` — move a failed sync to retesting
- `GET /api/gridsyncs`, `GET /api/gridsyncs/{id}`

### Health
- `GET /healthz` → `{"status":"ok"}`

## Docker

The image is multi-arch (amd64/arm64) via `docker buildx`; the final `scratch`
stage contains only the static binary.

```bash
# Build for the native architecture.
docker build -t ejinagrid:latest .

# Build and push multi-arch images (amd64 + arm64).
docker buildx build --platform linux/amd64,linux/arm64 -t ejinagrid:latest .

# Run, publishing the port and persisting state under ./data.
docker run --rm -p 51523:51523 -v "$PWD/data:/data" ejinagrid:latest
```

Quick smoke test:

```bash
curl -s http://localhost:51523/healthz
curl -s http://localhost:51523/api/cabins | head
```

## Test

```bash
go fmt ./...
go mod tidy
go mod verify
go build ./...
go test -timeout=120s -count=1 ./...
```

Tests cover normal paths, error paths, state transitions, concurrency and
cancellation, and failure recovery, with no dependency on external services.
