# Design Document: Comprehensive Test Suite

## Overview

This document describes the design for a hermetic, race-detector-clean test suite for the `chaind` daemon. The suite is invoked via a single `./test.sh` at the project root and covers every major subsystem without requiring live platform credentials or network access.

The key design challenge is that several production components (`store.NewStore`, `ipc.IPCServer.Start`) are tightly coupled to the filesystem and a fixed socket path. The test infrastructure must decouple these by introducing a `NewStoreFromPath` constructor and a `StartOnListener` method so tests can inject `t.TempDir()` paths and pre-bound `net.Listener` instances.

---

## Architecture

```mermaid
graph TD
    subgraph test.sh
        A[go test -race -count=1 -v ./...]
    end

    subgraph internal/testutil
        B[newTestStore]
        C[newTestIPCServer]
        D[newTestToken]
    end

    subgraph Test Packages
        E[internal/adapters/..._test.go]
        F[internal/daemon/..._test.go]
        G[internal/store/..._test.go]
        H[internal/ipc/..._test.go]
        I[internal/format/..._test.go]
        J[internal/schema/..._test.go]
        K[cmd/integration_test.go]
    end

    A --> Test Packages
    Test Packages --> B
    Test Packages --> C
    Test Packages --> D
    B --> |t.TempDir SQLite| Store[(SQLite :memory: / TempDir)]
    C --> |temp Unix socket| IPCServer
    D --> |inserts token row| Store
```

All tests are in-process. No goroutines escape test boundaries — every helper registers `t.Cleanup` for teardown.

---

## Components and Interfaces

### 1. `internal/testutil` — Shared Test Helpers

New package containing three exported helpers used across all test files.

```go
// NewTestStore opens a pure-Go SQLite DB at t.TempDir()/test.db,
// runs all migrations, and registers t.Cleanup to close it.
func NewTestStore(t *testing.T) *store.Store

// NewTestIPCServer starts an IPCServer on a temp Unix socket,
// registers t.Cleanup to shut it down, and returns the socket path
// and an HTTP client pre-wired to dial it.
func NewTestIPCServer(t *testing.T, st *store.Store, router *daemon.AdapterRouter) (socketPath string, client *http.Client)

// NewTestToken inserts a token row into st and returns the token name.
func NewTestToken(t *testing.T, st *store.Store, tier int, rooms, piiScrub string) string
```

### 2. `store.NewStoreFromPath(path string)` — Testable Constructor

The existing `store.NewStore()` hard-codes `~/.local/share/chaind/messages.db`. A new exported constructor is needed:

```go
// NewStoreFromPath opens (or creates) a Store at the given path.
// Used by tests to open a t.TempDir()-isolated database.
func NewStoreFromPath(path string) (*Store, error)
```

`NewStore()` becomes a thin wrapper: `return NewStoreFromPath(defaultPath)`.

### 3. `ipc.IPCServer.StartOnListener(ctx, ln)` — Testable Server Start

The existing `Start(ctx)` creates its own `net.Listener` at a fixed path. A new method accepts a pre-bound listener:

```go
// StartOnListener serves on the provided listener.
// Used by tests to bind to a temp Unix socket.
func (s *IPCServer) StartOnListener(ctx context.Context, ln net.Listener) error
```

`Start(ctx)` creates the listener and delegates to `StartOnListener`.

### 4. `daemon.ProcessOutbox` — Exported Wrapper

`processOutbox` is currently unexported. It will be exported as `ProcessOutbox` (or a thin exported wrapper added) so scheduler tests can call it directly without waiting for the 30-second ticker:

```go
// ProcessOutbox is the exported entry point for scheduler tests.
func ProcessOutbox(db *store.Store, router *AdapterRouter) {
    processOutbox(db, router)
}
```

### 5. `adapters.MockAdapter` — Extended for Watch Testing

The existing `MockAdapter.Watch` returns a channel that is never written to. For SSE tests, a `SendToWatch(msg)` method is added:

```go
// SendToWatch pushes a message into the channel returned by Watch.
func (m *MockAdapter) SendToWatch(msg schema.Message)
```

Internally, `Watch` stores the channel so `SendToWatch` can write to it.

---

## Data Models

No new persistent data models are introduced. The test suite operates entirely on existing schema tables. The `testutil` helpers use the following existing types:

- `store.Token` — inserted by `NewTestToken`
- `schema.Message` — constructed inline in each test
- `store.SchemaSQL` — applied by `NewTestStore` via `NewStoreFromPath`

### Test Data Factories

Each test constructs minimal valid instances. A `makeMessage` factory in `testutil` produces a `schema.Message` with a fresh ULID, configurable platform/room/text:

```go
func MakeMessage(platform, roomID, text string) schema.Message {
    return schema.Message{
        SchemaVersion: "1.0",
        ID:            ulid.Make().String(),
        Platform:      platform,
        PlatformID:    ulid.Make().String(),
        Room:          schema.Room{ID: roomID},
        Author:        schema.Author{ID: ulid.Make().String()},
        Content:       schema.Content{Type: "text", Text: text},
        Timestamp:     time.Now().UTC(),
    }
}
```

---

## Correctness Properties

*A property is a characteristic or behavior that should hold true across all valid executions of a system — essentially, a formal statement about what the system should do. Properties serve as the bridge between human-readable specifications and machine-verifiable correctness guarantees.*

The Go ecosystem does not have a widely-adopted property-based testing library with the maturity of QuickCheck or Hypothesis, but `pgregory.net/rapid` is the closest equivalent and is well-maintained. All property tests use `rapid` with a minimum of 100 iterations (the `rapid` default is 100 draws per property).

### Property 1: MockAdapter Send Invariant

*For any* roomID and text string, calling `MockAdapter.Send(roomID, text)` must return a `schema.Message` with a non-empty `ID` field, and the `OutboundMessages` slice must grow by exactly one after each call.

**Validates: Requirements 3.2, 3.3**

---

### Property 2: AdapterRouter Register-Get Round-Trip

*For any* platform name and adapter instance, registering the adapter and then calling `Get` with the same platform name must return the exact same adapter instance and a nil error.

**Validates: Requirements 4.1**

---

### Property 3: AdapterRouter Unregister Removes Entry

*For any* platform name that has been registered, calling `Unregister` followed by `Get` must return a non-nil error.

**Validates: Requirements 4.2**

---

### Property 4: AdapterRouter Send Delegation

*For any* registered MockAdapter, calling `AdapterRouter.Send(platform, roomID, text)` must result in the message appearing in `MockAdapter.OutboundMessages` and must return a nil error.

**Validates: Requirements 4.3, 4.4, 4.5**

---

### Property 5: AdapterRouter Concurrency Safety

*For any* sequence of concurrent `Register`, `Unregister`, and `Get` operations across 50 goroutines on the same `AdapterRouter`, all operations must complete without triggering the Go race detector and without deadlock.

**Validates: Requirements 5.1, 5.2, 5.3**

---

### Property 6: Message JSON Round-Trip

*For any* `schema.Message` value (including nil pointer fields, non-nil HTML, empty attachments, and any SchemaVersion string), `json.Unmarshal(json.Marshal(msg))` must produce a value that is deeply equal to the original, with nil pointer fields remaining nil and non-nil pointer fields preserving their values.

**Validates: Requirements 6.1, 6.2, 6.3, 6.4, 6.5**

---

### Property 7: Schema Migration Idempotence

*For any* SQLite database path, running `SchemaSQL` twice on the same database must not return an error on the second execution.

**Validates: Requirements 7.3**

---

### Property 8: StoreWriter Concurrent Write Completeness

*For any* N goroutines each pushing M distinct messages (where N×M ≤ 1000), after the StoreWriter flushes, `Store.GetRecentMessages` must return exactly N×M messages with no duplicates dropped.

**Validates: Requirements 8.1, 8.5**

---

### Property 9: Store Repository Round-Trips

*For any* message inserted directly into the database, `Store.GetMessage(id)` must return a message with the same ID, platform, room ID, and text. Similarly, *for any* sync state key-value pair, `SetSyncState` followed by `GetSyncState` must return the same value. *For any* cursor timestamp, `SaveCursor` followed by `GetCursor` must return the same timestamp.

**Validates: Requirements 9.2, 9.6, 9.7**

---

### Property 10: GetRecentMessages Ordering and Limit

*For any* set of N messages inserted with distinct ULIDs and any limit L, `GetRecentMessages(ctx, L)` must return at most L messages, and the returned messages must be in descending ULID order.

**Validates: Requirements 9.1**

---

### Property 11: FTS5 Search Containment

*For any* set of messages and any search term that appears in a subset of those messages, `SearchEngine.Search` must return only messages whose text contains the search term, and must not return messages that do not contain it.

**Validates: Requirements 10.1, 10.3**

---

### Property 12: IPC Token Auth — Valid Token Always Passes

*For any* non-revoked token in the database, a request to any IPC endpoint with that token in the `Authorization: Bearer` header must receive a non-401 response.

**Validates: Requirements 11.4, 11.5**

---

### Property 13: IPC Room Scoping

*For any* tier-1 token with a specific room allowlist, requests for rooms in the allowlist must succeed (2xx), and requests for rooms not in the allowlist must return 403. *For any* tier-0 token, requests for any room (including wildcard) must succeed.

**Validates: Requirements 12.1, 12.2, 12.3, 12.4, 12.5, 12.6**

---

### Property 14: PII Scrubbing Completeness

*For any* message containing an email address and a token with `pii_scrub="email"`, the response from `GET /api/v1/messages/recent` must not contain the original email address. The same holds for phone numbers with `pii_scrub="phone"`.

**Validates: Requirements 15.1, 15.2**

---

### Property 15: PII Scrubbing Idempotence

*For any* message and token with a non-empty `pii_scrub` configuration, calling `ScrubMessage` twice on the same message must produce the same result as calling it once.

**Validates: Requirements 15.4, 24.5**

---

### Property 16: Format Round-Trip — Plain Text Preservation

*For any* string containing only plain ASCII text (no Markdown markup), `PlainRenderer{}.Render(ParseMarkdown(input))` must return a string that contains the original text as a substring.

**Validates: Requirements 19.1, 19.2**

---

### Property 17: Outbox Scheduler Dispatch and Cleanup

*For any* set of outbox items where some have `scheduled_at` in the past and some in the future, calling `ProcessOutbox` must dispatch exactly the past-due items (calling `router.Send` for each), delete all dispatched items from the `outbox` table, and leave future items untouched — even when `router.Send` returns an error.

**Validates: Requirements 20.1, 20.2, 20.3, 20.4**

---

### Property 18: Supervisor Backoff Doubling

*For any* number of consecutive adapter failures K (where K ≥ 1), the backoff duration after K failures must equal `min(5s × 2^(K-1), 5min)`.

**Validates: Requirements 22.3**

---

## Error Handling

| Scenario | Behavior |
|---|---|
| `NewStoreFromPath` fails to open SQLite | Returns wrapped error; test helper calls `t.Fatal` |
| `NewTestIPCServer` fails to bind socket | Calls `t.Fatal` with the error |
| `ProcessOutbox` DB query fails | Logs error, returns without panicking |
| `router.Send` returns error in outbox | Logs error, still deletes outbox row |
| `ScrubMessage` called with nil token | Returns immediately, no mutation |
| `GetToken` returns unknown token | IPC middleware returns HTTP 401 |
| `json.Unmarshal` fails in IPC handler | Handler returns HTTP 400 |

---

## Testing Strategy

### Dual Testing Approach

Both unit tests and property-based tests are used. Unit tests cover specific examples, edge cases, and error conditions. Property tests verify universal invariants across many generated inputs.

### Property-Based Testing Library

All property tests use **`pgregory.net/rapid`** (pure Go, no CGO, well-maintained). Each property test runs a minimum of 100 iterations (rapid's default). The library is added to `go.mod` as a test dependency.

Tag format for each property test:
```
// Feature: comprehensive-test-suite, Property N: <property_text>
```

### Test File Layout

```
internal/testutil/
    helpers.go          ← NewTestStore, NewTestIPCServer, NewTestToken, MakeMessage

internal/adapters/
    mock_test.go        ← Req 3: interface assertion, MockAdapter behavior

internal/daemon/
    router_test.go      ← Req 4, 5: AdapterRouter dispatch and concurrency
    scheduler_test.go   ← Req 20: outbox scheduler (uses ProcessOutbox)
    supervisor_test.go  ← Req 22: backoff loop

internal/store/
    store_test.go       ← Req 7: migrations, schema, WAL
    writer_test.go      ← Req 8: StoreWriter concurrency (replaces existing non-hermetic test)
    repository_test.go  ← Req 9: GetRecentMessages, GetMessage, GetToken, SyncState, Cursor

internal/search/
    search_test.go      ← Req 10: FTS5 search, since filter, triggers

internal/ipc/
    auth_test.go        ← Req 11: token auth middleware
    scope_test.go       ← Req 12: room scoping
    endpoints_test.go   ← Req 13: all HTTP endpoints
    queue_test.go       ← Req 14: approval queue lifecycle
    pii_test.go         ← Req 15, 24: PII scrubbing (unit + IPC)
    sse_test.go         ← Req 16: SSE watch endpoint

internal/schema/
    roundtrip_test.go   ← Req 6: JSON round-trip property (replaces/extends existing tests)

internal/format/
    parser_test.go      ← Req 17: ParseMarkdown node types
    renderer_test.go    ← Req 18: all three renderers
    roundtrip_test.go   ← Req 19: parse-render round-trip property

cmd/
    integration_test.go ← Req 23: CLI end-to-end via Unix socket

test.sh                 ← Req 1: single entry point
```

### Unit Test Focus

Unit tests cover:
- Specific parser/renderer examples (Req 17, 18)
- IPC endpoint HTTP status codes and response bodies (Req 13, 14)
- Token auth edge cases: missing header, revoked token, Bearer prefix stripping (Req 11)
- PII scrubber pattern matching for email, phone, PAN (Req 24)
- Store error conditions: unknown ID, unknown token name (Req 9.3, 9.5)
- FTS5 trigger correctness: update and delete (Req 10.4, 10.5)
- Supervisor lifecycle: unregister on failure, re-register on retry, context cancellation (Req 22)

### Property Test Configuration

Each property test is annotated with its property number and runs at least 100 iterations:

```go
// Feature: comprehensive-test-suite, Property 6: Message JSON Round-Trip
func TestMessageJSONRoundTrip(t *testing.T) {
    rapid.Check(t, func(rt *rapid.T) {
        // generate random schema.Message
        // marshal → unmarshal → assert equal
    })
}
```

### Hermetic Constraints

- Every test that touches SQLite uses `NewTestStore(t)` which opens a `t.TempDir()`-isolated database.
- Every test that touches the IPC server uses `NewTestIPCServer(t, st, router)` which binds to a temp Unix socket.
- No test sets `HOME`, `CHAIND_TOKEN`, or any other global environment variable without using `t.Setenv` (which auto-restores).
- The existing `internal/store/writer_test.go` and `internal/store/repository_test.go` call `store.NewStore()` which touches `~/.local/share/chaind/`. These will be rewritten to use `NewTestStore(t)`.
- The race detector (`-race`) is always enabled via `test.sh`.
