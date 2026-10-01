# Rockets service

Consumes rocket messages and serves each rocket's current state over a REST API.

## Run

Requires Go 1.25+.

```bash
make run    # listens on :8088
make test   # go test -race ./...
```

Then send messages with the test program:

```bash
./rockets launch "http://localhost:8088/messages"
```

## API

| Endpoint                | Description                                                                   |
| ----------------------- | ----------------------------------------------------------------------------- |
| `POST /messages`        | Ingest a message. `202` on success, `400` if invalid.                         |
| `GET /rockets`          | All rockets. `sort` = `channel` (default), `type`, `mission`, `speed`; `order` = `asc` (default), `desc`. |
| `GET /rockets/{channel}`| One rocket, or `404`.                                                         |

```bash
curl "http://localhost:8088/rockets"
curl "http://localhost:8088/rockets?sort=speed&order=desc"
curl "http://localhost:8088/rockets/<channel>"
```

```json
{"channel":"193270a9-…","status":"launched","type":"Falcon-9","mission":"ARTEMIS","speed":500,"lastMessageNumber":1,"lastMessageTime":"2022-02-02T19:39:05.86337+01:00"}
```

`status` is `awaiting_launch`, `launched` or `exploded` (with `explosionReason`).

## Design

- `internal/rocket` — domain: messages, rocket state, service, `Repository` interface.
- `internal/storage` — in-memory `Repository`.
- `internal/httpapi` — HTTP handlers.

Messages are applied strictly in `messageNumber` order per channel: early ones wait until
the gap before them is filled, and repeated ones are ignored. So out-of-order and
at-least-once delivery give the same state as an in-order stream.

## Assumptions and shortcuts

- State is in memory and lost on restart. Storage sits behind `Repository`, so a persistent
  store (SQLite is next) only needs a new implementation.
- A message that never arrives stalls its rocket; delivery is at-least-once, so it shouldn't happen.
- An explosion is final.
- `POST /messages` responds with no body: the test program doesn't read response bodies,
  so a body would prevent it from reusing connections.
