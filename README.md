# Task API

A RESTful task API in Go. Tasks live in memory, can optionally be synced to a
snapshot file, and come with a small browser console for manual testing.

## Requirements

- Go 1.26 or newer. Go 1.21 to 1.25 download the 1.26 toolchain automatically.
- Or Docker, which needs no local Go.

## Run

```bash
go run ./cmd/server
```

The API and the console listen on <http://localhost:8080>.

```bash
docker build -t task-api .
docker run --rm -p 8080:8080 task-api
```

To keep tasks across restarts, mount a volume and point `DATA_FILE` at it:

```bash
docker run --rm -p 8080:8080 -v task-data:/data -e DATA_FILE=/data/tasks.json task-api
```

## Test

```bash
go test -race ./...
```

## API

| Method | Path | Success | Description |
|---|---|---|---|
| `GET` | `/tasks` | `200` | List tasks ordered by id |
| `POST` | `/tasks` | `201` | Create a task; the `Location` header points to it |
| `PUT` | `/tasks/{id}` | `200` | Replace a task's `name` and `status` |
| `DELETE` | `/tasks/{id}` | `204` | Delete a task |

A task looks like this:

```json
{"id": 1, "name": "buy milk", "status": 0}
```

- `name` is required, must contain a non-whitespace character, and may be at
  most 255 characters. It is stored exactly as sent.
- `status` is required and must be `0` (incomplete) or `1` (completed).
- `id` is assigned by the server and never reused.

Errors use one shape, `{"error": "..."}`, with these status codes:

| Status | Meaning |
|---|---|
| `400` | Invalid JSON, missing or invalid field, unknown field, or invalid id |
| `404` | Unknown path or task |
| `405` | Method not allowed; see the `Allow` header |
| `413` | Body larger than 1 MiB |
| `500` | Internal error; details are only logged |

```bash
curl -i -X POST localhost:8080/tasks -d '{"name":"buy milk","status":0}'
curl -i localhost:8080/tasks
curl -i -X PUT localhost:8080/tasks/1 -d '{"name":"buy milk","status":1}'
curl -i -X DELETE localhost:8080/tasks/1
```

## Web pages

| Page | For |
|---|---|
| <http://localhost:8080/> | Managing tasks: add, complete, rename and delete |
| <http://localhost:8080/test> | Testing the API: sends requests exactly as written, including invalid input, and shows the raw response |

The API has no authentication, so do not expose the service publicly.

## Configuration

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | Listening port |
| `LOG_LEVEL` | `INFO` | `DEBUG`, `INFO`, `WARN` or `ERROR`; filters debug logs only |
| `LOG_OUTPUT` | `stdout` | Only `stdout` is supported |
| `DATA_FILE` | unset | Snapshot file; unset means memory only |
| `DATA_FLUSH_INTERVAL` | `5s` | How often unsaved changes are written; `0` writes only on shutdown |

Invalid values stop the server at startup with a non-zero exit code.

## Persistence

With `DATA_FILE` set, the server loads the snapshot before serving, writes
changes periodically, and writes again on graceful shutdown (`SIGINT` or
`SIGTERM`). Writes go to a temporary file that replaces the snapshot, so a
failed write never damages the previous one. A corrupt or unreadable snapshot
stops startup instead of being overwritten. After a crash, the server restores
the last successful write.

## Logs

Every event is one JSON line on stdout. `log_type` separates per-request
`access` logs from `debug` logs:

```bash
docker logs <container> 2>&1 | jq -c 'select(.log_type == "access")'
```

## Known limitations

- **Task ids are not checked for exhaustion.** Ids are 64-bit integers. Only a
  hand-edited snapshot with `next_id` at the maximum can reach the limit; the
  next create then overflows, and the next startup refuses the snapshot without
  overwriting it. See [the domain spec](docs/spec/core/01-domain.md).

## Layout

| Path | Contents |
|---|---|
| `cmd/server` | Configuration, wiring, startup and graceful shutdown |
| `internal/task` | Domain model, validation and service |
| `internal/memory` | In-memory repository |
| `internal/httpapi` | HTTP routes ([chi](https://github.com/go-chi/chi)), request parsing and error mapping |
| `internal/snapshot` | Snapshot file and sync |
| `internal/logging` | JSON logger |
| `internal/webui` | Embedded browser console |
| `docs/spec` | Specification (RFC 2119) |
