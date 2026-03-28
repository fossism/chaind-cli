# Implementation Plan: Comprehensive Test Suite

## Overview

Implement a hermetic, race-detector-clean test suite for the `chaind` daemon. All tests run via `./test.sh` at the project root. The plan proceeds in dependency order: production refactors first (so tests can compile), then shared test infrastructure, then package-by-package test files.

## Tasks

- [x] 1. Create `test.sh` entry point
  - Create `test.sh` at the project root with content: `#!/usr/bin/env bash\nset -e\ngo test -race -count=1 -v ./...`
  - Run `chmod +x test.sh`
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 1.8_

- [x] 2. Production refactor — `store.NewStoreFromPath`
  - [x] 2.1 Add `NewStoreFromPath(path string) (*Store, error)` to `internal/store/sqlite.go`
    - Extract the open/configure/migrate logic from `NewStore()` into `NewStoreFromPath`
    - Make `NewStore()` a thin wrapper: resolve default path then call `NewStoreFromPath`
    - _Requirements: 2.1, 7.1, 7.4_

- [x] 3. Production refactor — `ipc.IPCServer.StartOnListener`
  - [x] 3.1 Add `StartOnListener(ctx context.Context, ln net.Listener) error` to `internal/ipc/socket.go`
    - Move the `s.server.Serve(listener)` body from `Start` into `StartOnListener`
    - Make `Start(ctx)` create the listener and delegate to `StartOnListener`
    - _Requirements: 2.2_

- [x] 4. Production refactor — `daemon.ProcessOutbox` exported wrapper
  - [x] 4.1 Add exported `ProcessOutbox(db *store.Store, router *AdapterRouter)` to `internal/daemon/scheduler.go`
    - Implement as a one-line wrapper calling the existing unexported `processOutbox`
    - _Requirements: 20.1, 20.2, 20.3, 20.4_

- [x] 5. Production refactor — `adapters.MockAdapter.SendToWatch`
  - [x] 5.1 Extend `MockAdapter` in `internal/adapters/mock.go` with a stored watch channel and `SendToWatch(msg schema.Message)` method
    - Add a `watchCh chan schema.Message` field to `MockAdapter`
    - Update `Watch` to create and store the channel on first call
    - Add `SendToWatch(msg schema.Message)` that writes to the stored channel
    - _Requirements: 16.2_

- [x] 6. Add `pgregory.net/rapid` test dependency
  - [x] 6.1 Run `go get pgregory.net/rapid` and verify it appears in `go.mod` under `require` (test-only usage is fine; no build tag needed)
    - _Requirements: 2.6_

- [x] 7. Create `internal/testutil` package with shared helpers
  - [x] 7.1 Create `internal/testutil/helpers.go` with `NewTestStore`, `NewTestIPCServer`, `NewTestToken`, and `MakeMessage`
    - `NewTestStore(t)`: calls `store.NewStoreFromPath(filepath.Join(t.TempDir(), "test.db"))`, registers `t.Cleanup(st.Close)`, calls `t.Fatal` on error
    - `NewTestIPCServer(t, st, router)`: creates a temp Unix socket path in `t.TempDir()`, calls `net.Listen("unix", path)`, starts `ipc.NewIPCServer(st, router).StartOnListener(ctx, ln)` in a goroutine, returns socket path and an `*http.Client` dialing that socket; registers `t.Cleanup` to cancel context
    - `NewTestToken(t, st, tier, rooms, piiScrub)`: inserts a row into `tokens` table via `st.DB()`, returns the generated token name string; calls `t.Fatal` on error
    - `MakeMessage(platform, roomID, text)`: returns a `schema.Message` with fresh ULID, `SchemaVersion: "1.0"`, and `Timestamp: time.Now().UTC()`
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5_

- [x] 8. Adapter interface and MockAdapter tests
  - [x] 8.1 Create `internal/adapters/mock_test.go` with compile-time assertion and behavioral unit tests
    - Add `var _ adapters.Adapter = (*adapters.MockAdapter)(nil)` compile-time check
    - Test `Send` returns non-empty ULID `ID` and appends to `OutboundMessages`
    - Test `Disconnect` returns nil
    - Test `React` returns nil
    - Test `Watch` returns non-nil channel and nil error
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6_

  - [ ]* 8.2 Write property test for MockAdapter Send invariant
    - **Property 1: MockAdapter Send Invariant**
    - **Validates: Requirements 3.2, 3.3**
    - Use `rapid.Check` to generate arbitrary roomID and text strings; assert `ID != ""` and `len(OutboundMessages)` grows by 1 per call

- [x] 9. AdapterRouter registration and dispatch tests
  - [x] 9.1 Create `internal/daemon/router_test.go` with unit tests for register/get/unregister/send/ban
    - Test `Register` then `Get` returns same instance
    - Test `Unregister` then `Get` returns non-nil error
    - Test `Send` for registered platform delegates to adapter and returns nil error
    - Test `Send` for unregistered platform returns non-nil error
    - Test `Ban` for registered platform delegates to adapter
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5_

  - [ ]* 9.2 Write property test for AdapterRouter Register-Get round-trip
    - **Property 2: AdapterRouter Register-Get Round-Trip**
    - **Validates: Requirements 4.1**

  - [ ]* 9.3 Write property test for AdapterRouter Unregister removes entry
    - **Property 3: AdapterRouter Unregister Removes Entry**
    - **Validates: Requirements 4.2**

  - [ ]* 9.4 Write property test for AdapterRouter Send delegation
    - **Property 4: AdapterRouter Send Delegation**
    - **Validates: Requirements 4.3, 4.4, 4.5**

  - [ ]* 9.5 Write property test for AdapterRouter concurrency safety
    - **Property 5: AdapterRouter Concurrency Safety**
    - **Validates: Requirements 5.1, 5.2, 5.3**
    - Spawn 50 goroutines interleaving `Register`, `Unregister`, `Get`; run with `-race`

- [x] 10. Checkpoint — ensure all tests pass so far
  - Ensure all tests pass, ask the user if questions arise.

- [x] 11. Canonical Message schema JSON round-trip tests
  - [x] 11.1 Create `internal/schema/roundtrip_test.go` with unit tests for nil pointer fields, non-nil HTML, empty attachments, and SchemaVersion preservation
    - Test nil `RootID`/`ParentID` survive round-trip as nil
    - Test non-nil `Content.HTML` pointer preserves string value
    - Test empty `Content.Attachments` does not become non-nil after round-trip
    - Test `SchemaVersion` is preserved as `"1.0"`
    - _Requirements: 6.2, 6.3, 6.4, 6.5_

  - [ ]* 11.2 Write property test for Message JSON round-trip
    - **Property 6: Message JSON Round-Trip**
    - **Validates: Requirements 6.1, 6.2, 6.3, 6.4, 6.5**
    - Use `rapid` to generate arbitrary `schema.Message` values; assert deep equality after marshal/unmarshal

- [x] 12. SQLite store migration and schema tests
  - [x] 12.1 Create `internal/store/store_test.go` with migration and WAL tests
    - Use `NewTestStore(t)` for all tests
    - Assert all tables from `SchemaSQL` exist after `NewStoreFromPath`
    - Assert `messages_fts` virtual table and its three triggers exist
    - Assert running `SchemaSQL` a second time on the same DB returns no error (idempotence)
    - Assert WAL mode is active (`PRAGMA journal_mode` returns `wal`)
    - _Requirements: 7.1, 7.2, 7.3, 7.4_

  - [ ]* 12.2 Write property test for schema migration idempotence
    - **Property 7: Schema Migration Idempotence**
    - **Validates: Requirements 7.3**
    - Use `rapid` to generate arbitrary temp paths; assert double-migration never errors

- [x] 13. StoreWriter serialization and concurrency tests
  - [x] 13.1 Rewrite `internal/store/writer_test.go` to use `NewTestStore(t)` and test concurrent write completeness, context-cancel flush, channel-full drop, and batch-size trigger
    - Test N goroutines × M messages all persisted (N×M ≤ 1000)
    - Test context cancel flushes remaining channel items
    - Test channel-full causes drop without blocking caller
    - Test 50-message batch triggers immediate flush before ticker
    - _Requirements: 8.1, 8.2, 8.3, 8.4, 8.5_

  - [ ]* 13.2 Write property test for StoreWriter concurrent write completeness
    - **Property 8: StoreWriter Concurrent Write Completeness**
    - **Validates: Requirements 8.1, 8.5**

- [x] 14. Store repository query tests
  - [x] 14.1 Rewrite `internal/store/repository_test.go` to use `NewTestStore(t)` and cover all query methods
    - Test `GetRecentMessages` returns at most `limit` messages in descending ULID order
    - Test `GetMessage` with known ID returns correct message
    - Test `GetMessage` with unknown ID returns non-nil error
    - Test `GetToken` with known name returns correct Token struct
    - Test `GetToken` with unknown name returns non-nil error
    - Test `SetSyncState` / `GetSyncState` round-trip
    - Test `SaveCursor` / `GetCursor` round-trip
    - _Requirements: 9.1, 9.2, 9.3, 9.4, 9.5, 9.6, 9.7_

  - [ ]* 14.2 Write property test for Store repository round-trips
    - **Property 9: Store Repository Round-Trips**
    - **Validates: Requirements 9.2, 9.6, 9.7**

  - [ ]* 14.3 Write property test for GetRecentMessages ordering and limit
    - **Property 10: GetRecentMessages Ordering and Limit**
    - **Validates: Requirements 9.1**

- [x] 15. FTS5 full-text search tests
  - [x] 15.1 Create `internal/search/search_test.go` covering search containment, empty results, since filter, update trigger, and delete trigger
    - Use `NewTestStore(t)` and insert messages via `StoreWriter` flush
    - Test search returns only messages containing the term
    - Test search with no-match term returns empty non-nil slice
    - Test `since` filter excludes old messages
    - Test FTS5 update trigger: updated message text appears in results
    - Test FTS5 delete trigger: deleted message does not appear in results
    - _Requirements: 10.1, 10.2, 10.3, 10.4, 10.5_

  - [ ]* 15.2 Write property test for FTS5 search containment
    - **Property 11: FTS5 Search Containment**
    - **Validates: Requirements 10.1, 10.3**

- [-] 16. Checkpoint — ensure all tests pass so far
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 17. IPC server token authentication middleware tests
  - [ ] 17.1 Create `internal/ipc/auth_test.go` covering all auth middleware edge cases
    - Use `NewTestStore(t)`, `NewTestIPCServer(t, st, router)`, `NewTestToken(t, ...)`
    - Test missing `Authorization` header and no `CHAIND_TOKEN` env → HTTP 401
    - Test unknown token → HTTP 401
    - Test revoked token → HTTP 401
    - Test valid non-revoked token → HTTP 2xx
    - Test `Bearer ` prefix is stripped before lookup
    - _Requirements: 11.1, 11.2, 11.3, 11.4, 11.5_

  - [ ]* 17.2 Write property test for IPC token auth — valid token always passes
    - **Property 12: IPC Token Auth — Valid Token Always Passes**
    - **Validates: Requirements 11.4, 11.5**

- [ ] 18. IPC server room scoping tests
  - [ ] 18.1 Create `internal/ipc/scope_test.go` covering tier-0 wildcard, tier-1 allow, tier-1 deny, empty room, and wildcard room
    - Tier-0 token + any room → allowed
    - Tier-1 token with `rooms="roomA"` + `room=roomA` → allowed
    - Tier-1 token with `rooms="roomA"` + `room=roomB` → HTTP 403
    - Tier-1 token + empty room → HTTP 403
    - Tier-1 token + `room=*` → HTTP 403
    - Tier-0 token + `room=*` → allowed
    - _Requirements: 12.1, 12.2, 12.3, 12.4, 12.5, 12.6_

  - [ ]* 18.2 Write property test for IPC room scoping
    - **Property 13: IPC Room Scoping**
    - **Validates: Requirements 12.1, 12.2, 12.3, 12.4, 12.5, 12.6**

- [ ] 19. IPC server HTTP endpoint tests
  - [ ] 19.1 Create `internal/ipc/endpoints_test.go` covering all HTTP endpoints
    - `GET /api/v1/messages/recent` → HTTP 200 + JSON array
    - `POST /api/v1/messages/recent` → HTTP 405
    - `GET /api/v1/messages/search?q=term` → HTTP 200 + JSON array
    - `GET /api/v1/messages/search` (no `q`) → HTTP 400
    - `POST /api/v1/messages/send` with registered MockAdapter → HTTP 200 + sent message JSON
    - `POST /api/v1/messages/send` with `require_approval: true` → HTTP 200 + `"status":"queued for approval"` + row in `approval_queue`
    - `POST /api/v1/messages/send` with malformed JSON → HTTP 400
    - `GET /api/v1/adapters/status` → HTTP 200 + `{"daemon":"running",...}`
    - `POST /api/v1/moderate` with registered MockAdapter → HTTP 200 + `"status":"moderated"`
    - `POST /api/v1/moderate` with malformed JSON → HTTP 400
    - _Requirements: 13.1, 13.2, 13.3, 13.4, 13.5, 13.6, 13.7, 13.8, 13.9, 13.10_

- [ ] 20. IPC server HitL approval queue tests
  - [ ] 20.1 Create `internal/ipc/queue_test.go` covering the full approval queue lifecycle
    - `GET /api/v1/queue` → HTTP 200 + JSON array of pending items
    - Enqueue via `require_approval: true`, then `POST /api/v1/queue/exec?id=<id>` → dispatches via router, deletes row
    - `POST /api/v1/queue/exec?id=<unknown>` → HTTP 404
    - `POST /api/v1/queue/deny?id=<id>` → deletes row without dispatch, HTTP 200 + `"status":"denied"`
    - `POST /api/v1/queue/deny` (no `id`) → HTTP 400
    - _Requirements: 14.1, 14.2, 14.3, 14.4, 14.5_

- [ ] 21. IPC server PII scrubbing tests
  - [ ] 21.1 Create `internal/ipc/pii_test.go` with unit tests for `ScrubMessage` and IPC-level scrubbing
    - `ScrubMessage` with `pii_scrub="email"` replaces `user@example.com` with `[REDACTED_EMAIL]`
    - `ScrubMessage` with `pii_scrub="phone"` replaces 10-digit Indian mobile with `[REDACTED_PHONE]`
    - `ScrubMessage` with `pii_scrub="pan"` replaces `ABCDE1234F` with `[REDACTED_PAN]`
    - `ScrubMessage` with empty `pii_scrub` leaves content unchanged
    - `ScrubMessage` with nil token in context leaves content unchanged without panic
    - `GET /api/v1/messages/recent` with `pii_scrub="email"` token redacts email in response body
    - `GET /api/v1/messages/recent` with `pii_scrub="phone"` token redacts phone in response body
    - `GET /api/v1/messages/recent` with empty `pii_scrub` returns original content
    - _Requirements: 15.1, 15.2, 15.3, 24.1, 24.2, 24.3, 24.4, 24.6_

  - [ ]* 21.2 Write property test for PII scrubbing idempotence
    - **Property 15: PII Scrubbing Idempotence**
    - **Validates: Requirements 15.4, 24.5**
    - Use `rapid` to generate message text and pii_scrub config; assert `ScrubMessage` twice == once

- [ ] 22. IPC server SSE watch endpoint test
  - [ ] 22.1 Create `internal/ipc/sse_test.go` covering the SSE watch endpoint
    - `GET /api/v1/messages/watch?platform=mock&room=test` with registered MockAdapter → `Content-Type: text/event-stream`
    - Push a message via `MockAdapter.SendToWatch`; assert `data: <json>\n\n` event is received
    - Cancel client context; assert no goroutine leak (use `goleak` or manual goroutine count check)
    - _Requirements: 16.1, 16.2, 16.3_

- [ ] 23. Checkpoint — ensure all tests pass so far
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 24. Format package parser tests
  - [ ] 24.1 Create `internal/format/parser_test.go` covering all supported node types
    - `"**bold**"` → `TextNode{Bold: true}`
    - `"*italic*"` → `TextNode{Italic: true}`
    - `` "`code`" `` → `CodeNode`
    - Fenced code block → `BlockNode` with correct `Language` and `Content`
    - `"[label](https://example.com)"` → `LinkNode` with correct `URL` and `Label`
    - Autolink URL → `LinkNode` with `URL == Label`
    - Empty string → non-nil empty slice, no panic
    - _Requirements: 17.1, 17.2, 17.3, 17.4, 17.5, 17.6, 17.7_

- [ ] 25. Format package renderer tests
  - [ ] 25.1 Create `internal/format/renderer_test.go` covering all three renderers for every node type
    - `TelegramRenderer`: bold → `<b>...</b>`, link → `<a href="URL">Label</a>`, mention → `<a href="tg://user?id=...">...</a>`, code → `<code>...</code>`, block → `<pre><code class="language-LANG">...</code></pre>`
    - `MatrixRenderer`: bold → `**...**`, link → `[Label](URL)`, mention → `[DisplayName](https://matrix.to/#/UserID)`
    - `PlainRenderer`: mention → `@DisplayName`, link → `Label (URL)`, bold → content without markup
    - _Requirements: 18.1, 18.2, 18.3, 18.4, 18.5, 18.6, 18.7, 18.8, 18.9, 18.10, 18.11_

- [ ] 26. Format package parse-render round-trip tests
  - [ ] 26.1 Create `internal/format/roundtrip_test.go` with round-trip unit and property tests
    - Plain text input: `PlainRenderer{}.Render(ParseMarkdown(input))` contains original text
    - Non-empty Markdown: `TelegramRenderer{}.Render(ParseMarkdown(input))` is non-empty
    - Parse-render-parse consistency for simple AST
    - _Requirements: 19.1, 19.2, 19.3_

  - [ ]* 26.2 Write property test for format round-trip plain text preservation
    - **Property 16: Format Round-Trip — Plain Text Preservation**
    - **Validates: Requirements 19.1, 19.2**
    - Use `rapid` to generate plain ASCII strings; assert `PlainRenderer` output contains input as substring

- [ ] 27. Outbox scheduler tests
  - [ ] 27.1 Create `internal/daemon/scheduler_test.go` using `ProcessOutbox` directly
    - Use `NewTestStore(t)` and a `NewAdapterRouter()` with registered `MockAdapter`
    - Insert past-due outbox item; call `ProcessOutbox`; assert `MockAdapter.OutboundMessages` has the item and row is deleted
    - Insert future outbox item; call `ProcessOutbox`; assert no dispatch and row remains
    - Insert item where `router.Send` returns error; call `ProcessOutbox`; assert row is still deleted
    - _Requirements: 20.1, 20.2, 20.3, 20.4_

  - [ ]* 27.2 Write property test for outbox scheduler dispatch and cleanup
    - **Property 17: Outbox Scheduler Dispatch and Cleanup**
    - **Validates: Requirements 20.1, 20.2, 20.3, 20.4**

- [ ] 28. Capability token system tests
  - [ ] 28.1 Create `internal/store/token_test.go` (or extend `repository_test.go`) covering full token lifecycle
    - Insert token; `GetToken` returns matching `Tier`, `Rooms`, `PiiScrub`, `Revoked`
    - Revoked token on IPC endpoint → HTTP 401 (via `NewTestIPCServer`)
    - Tier-0 token allows all rooms including wildcard
    - Tier-1 token with `rooms="roomA,roomB"` allows `roomA`, denies `roomC`
    - Token with `pii_scrub="email,phone"` applies both redactions on GET endpoint
    - _Requirements: 21.1, 21.2, 21.3, 21.4, 21.5, 21.6_

- [ ] 29. Supervisor backoff loop tests
  - [ ] 29.1 Create `cmd/supervisor_test.go` (or `internal/daemon/supervisor_test.go`) testing the `supervise` function
    - Adapter `Start` returns error → `Unregister` is called for that platform
    - After failure, adapter is re-registered before next `Start` call
    - Consecutive failures double backoff up to 5 minutes: verify `min(5s × 2^(K-1), 5min)` sequence
    - Context cancel stops retrying without goroutine leak
    - Clean `Start` return (nil error) resets backoff to 5 seconds
    - _Requirements: 22.1, 22.2, 22.3, 22.4, 22.5_

  - [ ]* 29.2 Write property test for supervisor backoff doubling
    - **Property 18: Supervisor Backoff Doubling**
    - **Validates: Requirements 22.3**
    - Use `rapid` to generate K failures; assert backoff sequence matches `min(5s × 2^(K-1), 5min)`

- [ ] 30. CLI integration tests
  - [ ] 30.1 Create `cmd/integration_test.go` exercising CLI commands through an in-process IPC server
    - `chaind send` against in-process server with registered MockAdapter → `MockAdapter.OutboundMessages` contains sent message
    - `chaind read` against in-process server with pre-populated messages → output contains expected message text
    - `chaind search` with query term → output contains only matching messages
    - Any CLI command without valid `CHAIND_TOKEN` → exits with non-zero status
    - Use `t.Setenv("CHAIND_TOKEN", token)` and a custom socket path via env var or flag
    - _Requirements: 23.1, 23.2, 23.3, 23.4_

- [ ] 31. Final checkpoint — ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for a faster MVP
- Production refactors (tasks 2–5) must be completed before any test files can compile
- `pgregory.net/rapid` is the property-based testing library; add via `go get pgregory.net/rapid`
- All tests use `NewTestStore(t)` / `NewTestIPCServer(t, ...)` — never `store.NewStore()` directly
- The race detector is always active via `test.sh`; concurrency tests (tasks 9.5, 13.1) are the primary race coverage
- Property tests annotate each `rapid.Check` call with `// Feature: comprehensive-test-suite, Property N: <title>`
