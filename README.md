# Rockets service

Consumes rocket messages and serves each rocket's current state over a REST API.

## Run

Requires Go 1.25+.

```bash
make run    # listens on :8088, stores state in rockets.db
make test   # go test -race ./...
```

Then send messages with the test program:

```bash
./rockets launch "http://localhost:8088/messages"
```

State survives restarts. Flags: `-db <path>` for another database file, `-reset` to start
with an empty one (`go run ./cmd/server -reset`).

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

- `internal/rocket` — domain: rockets, messages and `Rocket.Apply`, which applies a message
  only if it is the next one (no I/O); the service, which processes messages, and the `Store`
  interface it uses.
- `internal/storage` — SQLite `Store`: a `rockets` table and `pending_messages` for messages
  waiting on an earlier one, plus `WithTx` to run several operations in one transaction.
- `internal/httpapi` — HTTP handlers.

### How a message is processed

`Service.ProcessMessage` handles each message in one transaction (`Store.WithTx`):

1. Load the rocket and call `Rocket.Apply`:
   - number ≤ last applied → repeat, ignored (nothing is written);
   - number > last applied + 1 → arrived early, saved to `pending_messages`
     (a repeat of a pending message is ignored by its primary key);
   - otherwise → applied.
2. If it was applied, load the rocket's pending messages, apply those that now follow in
   order (`Rocket.ApplyPending`), and delete the pending messages up to the last applied one.
3. Save the rocket and commit.

So out-of-order and at-least-once delivery give the same state as an in-order stream, and a
crash at any point leaves either the whole message processed or none of it (the sender retries).

## Testing

`make test` runs unit tests for each layer: the ordering rules with plain values, message
processing against a real database (gaps, repeats, restarts), the SQLite store (transactions,
concurrent writes, reads during writes) and the HTTP API.

The service was also checked against the `rockets` program:

- a default run: 100,000 messages, no failed requests;
- runs with the server restarted, and killed with `kill -9`, mid-run: every rocket's final
  state matched one recomputed independently from the recorded messages;
- a run with `GET /rockets` polled continuously: no failed reads or writes.

## Assumptions and shortcuts

- SQLite allows one writer at a time, so writes share one connection; reads use a separate
  read-only pool and run in parallel. Scaling out would mean a shared database such as
  Postgres behind the same `Store` interface.
- A message that never arrives stalls its rocket; delivery is at-least-once, so it shouldn't happen.
- An explosion is final.
- `POST /messages` responds with no body: the test program doesn't read response bodies,
  so a body would prevent it from reusing connections.
