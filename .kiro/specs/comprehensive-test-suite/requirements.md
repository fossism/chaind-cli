# Requirements Document

## Introduction

This feature adds a comprehensive, hermetic test suite to the `chaind` daemon. The entire suite is invoked via a single command: `./test.sh` from the project root. The suite covers every major subsystem: the Adapter interface and all platform adapters, the canonical Message schema, the SQLite store (WAL mode, dual-connection, StoreWriter serialization, migrations, FTS5 search), the IPC server (all HTTP endpoints, token auth middleware, room scoping, PII scrubbing), the AdapterRouter (register/unregister, dispatch, concurrency), the outbox scheduler, the HitL approval queue, the capability token system, the format package (Markdown parser and all renderers), PII scrubbing logic, CLI command integration via the Unix socket, and the supervisor/backoff loop.

All tests MUST be hermetic: they use in-memory or `t.TempDir()`-isolated SQLite databases and in-process servers. No test may depend on a live Telegram, Matrix, or WhatsApp connection. All Go tests are executed via `go test` (invoked by `test.sh`) — no other test runner or mechanism is used.

---

## Glossary

- **Test_Suite**: The collection of Go test files added by this feature.
- **Store**: `internal/store.Store` — the SQLite-backed message persistence layer.
- **StoreWriter**: `internal/store.StoreWriter` — the single-goroutine write serializer backed by a buffered channel.
- **AdapterRouter**: `internal/daemon.AdapterRouter` — the registry that maps platform names to live Adapter instances.
- **Adapter**: The interface defined in `internal/adapters/interface.go` that every platform integration must satisfy.
- **MockAdapter**: `internal/adapters.MockAdapter` — the in-process test double for the Adapter interface.
- **IPCServer**: `internal/ipc.IPCServer` — the HTTP/1.1 server running over a Unix socket.
- **Token**: A row in the `tokens` SQLite table representing a capability token with tier, room scope, PII scrub config, and revocation state.
- **Parser**: `internal/format.ParseMarkdown` — the Goldmark-based Markdown-to-AST parser.
- **Renderer**: Any type implementing `internal/format.Renderer` (TelegramRenderer, MatrixRenderer, PlainRenderer).
- **PII_Scrubber**: The logic in `internal/ipc/pii.go` that redacts email, phone, and PAN patterns from message content.
- **Scheduler**: `internal/daemon.StartScheduler` / `processOutbox` — the 30-second ticker that dispatches due outbox messages.
- **Approval_Queue**: The `approval_queue` SQLite table and the IPC endpoints that manage human-in-the-loop message approval.
- **Supervisor**: The `supervise()` function in `cmd/daemon.go` that restarts adapters with exponential backoff.
- **FTS5**: SQLite's built-in full-text search extension used via the `messages_fts` virtual table.
- **ULID**: Universally Unique Lexicographically Sortable Identifier — the ID format used for all canonical messages.

---

## Requirements

### Requirement 1: Single-Command Test Entry Point (`test.sh`)

**User Story:** As a developer, I want to run the entire test suite with a single command `./test.sh`, so that there is no ambiguity about how to execute tests and CI can rely on a stable entry point.

#### Acceptance Criteria

1. THE Test_Suite SHALL include a `test.sh` file at the project root that is the sole required entry point for running all tests.
2. WHEN `./test.sh` is executed, THE Test_Suite SHALL invoke `go test -race -count=1 -v ./...` to run all Go tests across all packages with the race detector enabled.
3. WHEN all tests pass, THE Test_Suite SHALL exit with code 0 and print a clear `PASS` summary line to stdout.
4. WHEN any test fails, THE Test_Suite SHALL exit with a non-zero exit code and print a clear `FAIL` summary line to stdout so that CI systems can detect failure.
5. THE `test.sh` file SHALL have executable permissions (`chmod +x`) set so that `./test.sh` works without any prior setup step.
6. WHEN `./test.sh` is executed, THE Test_Suite SHALL print verbose per-test output (via `-v`) so that individual test results are visible in CI logs.
7. THE Test_Suite SHALL NOT require any additional commands, environment variable exports, or setup steps beyond executing `./test.sh` from the project root.
8. WHEN `./test.sh` completes, THE Test_Suite SHALL print a final summary line indicating overall `PASS` or `FAIL` status as the last line of output.

---

### Requirement 2: Test Infrastructure and Helpers

**User Story:** As a developer, I want shared test helpers that spin up isolated in-memory stores and in-process IPC servers, so that every test is hermetic and fast.

#### Acceptance Criteria

1. THE Test_Suite SHALL provide a `newTestStore(t)` helper that opens a pure-Go SQLite database in a `t.TempDir()` path, runs all migrations, and registers `t.Cleanup` to close it.
2. THE Test_Suite SHALL provide a `newTestIPCServer(t, store, router)` helper that starts an IPCServer on a temporary Unix socket path and registers `t.Cleanup` to shut it down.
3. THE Test_Suite SHALL provide a `newTestToken(t, store, tier, rooms, piiScrub)` helper that inserts a Token row into the test store and returns the token name string.
4. WHEN a test helper encounters an unrecoverable setup error, THE Test_Suite SHALL call `t.Fatal` immediately so the test fails with a clear message rather than a nil-pointer panic.
5. THE Test_Suite SHALL NOT require any network access, live platform credentials, or external processes to run.
6. THE Test_Suite SHALL be executed exclusively via `go test` (invoked by `test.sh`) and SHALL NOT require any alternative test runner or build tool.

---

### Requirement 3: Adapter Interface Contract

**User Story:** As a developer, I want compile-time and runtime verification that MockAdapter fully satisfies the Adapter interface, so that test doubles remain in sync with the real interface.

#### Acceptance Criteria

1. THE Test_Suite SHALL include a compile-time interface assertion `var _ adapters.Adapter = (*adapters.MockAdapter)(nil)` in the adapter test file.
2. WHEN `MockAdapter.Send` is called with a roomID and text, THE MockAdapter SHALL return a `schema.Message` whose `ID` field is a non-empty ULID string.
3. WHEN `MockAdapter.Send` is called, THE MockAdapter SHALL append the returned message to its `OutboundMessages` slice so callers can inspect sent messages.
4. WHEN `MockAdapter.Disconnect` is called, THE MockAdapter SHALL return nil without panicking.
5. WHEN `MockAdapter.React` is called with any msgID and emoji, THE MockAdapter SHALL return nil without panicking.
6. WHEN `MockAdapter.Watch` is called, THE MockAdapter SHALL return a non-nil channel and a nil error.

---

### Requirement 4: AdapterRouter — Registration and Dispatch

**User Story:** As a developer, I want tests that verify the AdapterRouter correctly registers, retrieves, and unregisters adapters, so that IPC routing is reliable.

#### Acceptance Criteria

1. WHEN an Adapter is registered with `AdapterRouter.Register`, THE AdapterRouter SHALL return that same Adapter instance from a subsequent `Get` call with the same platform name.
2. WHEN `AdapterRouter.Unregister` is called for a platform, THE AdapterRouter SHALL return an error from a subsequent `Get` call for that platform.
3. WHEN `AdapterRouter.Send` is called for a registered platform, THE AdapterRouter SHALL delegate to the adapter's `Send` method and return its result.
4. WHEN `AdapterRouter.Send` is called for an unregistered platform, THE AdapterRouter SHALL return a non-nil error.
5. WHEN `AdapterRouter.Ban` is called for a registered platform, THE AdapterRouter SHALL delegate to the adapter's `Ban` method.

---

### Requirement 5: AdapterRouter — Concurrency Safety

**User Story:** As a developer, I want property-based concurrency tests for the AdapterRouter, so that simultaneous adapter registration and dispatch from multiple goroutines never causes a data race.

#### Acceptance Criteria

1. WHEN 50 goroutines concurrently call `Register`, `Unregister`, and `Get` on the same AdapterRouter, THE AdapterRouter SHALL complete all operations without triggering the Go race detector.
2. WHEN `AdapterRouter.Send` is called concurrently from multiple goroutines for a registered adapter, THE AdapterRouter SHALL serialize access to the adapter map without deadlock.
3. FOR ALL sequences of interleaved `Register(A)`, `Unregister(A)`, `Register(B)` operations, THE AdapterRouter SHALL reflect the final state correctly in a subsequent `Get` call.

---

### Requirement 6: Canonical Message Schema — JSON Round-Trip

**User Story:** As a developer, I want property tests that verify the canonical Message schema survives JSON serialization and deserialization intact, so that the IPC API always returns correct data.

#### Acceptance Criteria

1. FOR ALL valid `schema.Message` values, THE Test_Suite SHALL verify that `json.Unmarshal(json.Marshal(msg))` produces a message equal to the original (round-trip property).
2. WHEN a `schema.Message` has nil `RootID` and nil `ParentID`, THE Test_Suite SHALL verify that the round-tripped message also has nil `RootID` and nil `ParentID`.
3. WHEN a `schema.Message` has a non-nil `Content.HTML` pointer, THE Test_Suite SHALL verify that the round-tripped message preserves the HTML string value.
4. WHEN a `schema.Message` has an empty `Content.Attachments` slice, THE Test_Suite SHALL verify that the round-tripped message does not produce a non-nil attachments field.
5. THE Test_Suite SHALL verify that `schema.Message.SchemaVersion` is preserved as `"1.0"` through a JSON round-trip.

---

### Requirement 7: SQLite Store — Migrations and Schema

**User Story:** As a developer, I want tests that verify the SQLite schema is applied correctly and idempotently, so that the store is always in a known state on startup.

#### Acceptance Criteria

1. WHEN `store.NewStore()` is called on a fresh database, THE Store SHALL create all tables defined in `store.SchemaSQL` including `messages`, `rooms`, `users`, `tokens`, `approval_queue`, `outbox`, `sync_state`, `sync_cursors`, `modlog`, `access_log`, and `optout`.
2. WHEN `store.NewStore()` is called on a fresh database, THE Store SHALL create the `messages_fts` FTS5 virtual table and its three associated triggers (`messages_ai`, `messages_ad`, `messages_au`).
3. WHEN the migration SQL is executed twice on the same database, THE Store SHALL NOT return an error (idempotence via `CREATE TABLE IF NOT EXISTS`).
4. WHEN `store.NewStore()` is called, THE Store SHALL configure the write connection with `PRAGMA journal_mode=WAL` and the read pool with a maximum of 10 open connections.

---

### Requirement 8: SQLite Store — StoreWriter Serialization

**User Story:** As a developer, I want property tests that verify the StoreWriter serializes concurrent writes without data loss or SQLite locking errors, so that high-throughput adapter ingestion is safe.

#### Acceptance Criteria

1. WHEN N goroutines each push M messages concurrently via `Store.PushMessage`, THE StoreWriter SHALL persist all N×M messages to the database without dropping any (assuming N×M ≤ channel capacity).
2. WHEN the StoreWriter's context is cancelled, THE StoreWriter SHALL flush all messages remaining in its internal channel before returning.
3. WHEN `Store.PushMessage` is called while the internal channel is at capacity (1000 items), THE StoreWriter SHALL drop the message without blocking the caller goroutine.
4. WHEN the StoreWriter accumulates 50 messages before the 100ms ticker fires, THE StoreWriter SHALL flush the batch immediately without waiting for the ticker.
5. FOR ALL batches flushed by the StoreWriter, THE Store SHALL be able to retrieve the flushed messages via `Store.GetRecentMessages` after the flush completes.

---

### Requirement 9: SQLite Store — Repository Queries

**User Story:** As a developer, I want tests for all repository query methods, so that data retrieval is correct and consistent.

#### Acceptance Criteria

1. WHEN messages are inserted and `Store.GetRecentMessages(ctx, limit)` is called, THE Store SHALL return at most `limit` messages ordered by ULID descending.
2. WHEN `Store.GetMessage(ctx, id)` is called with a known message ID, THE Store SHALL return the correct message.
3. WHEN `Store.GetMessage(ctx, id)` is called with an unknown ID, THE Store SHALL return a non-nil error.
4. WHEN `Store.GetToken(ctx, name)` is called with a known token name, THE Store SHALL return the correct Token struct.
5. WHEN `Store.GetToken(ctx, name)` is called with an unknown name, THE Store SHALL return a non-nil error.
6. WHEN `Store.SetSyncState` is called followed by `Store.GetSyncState` with the same platform and key, THE Store SHALL return the value that was set (round-trip property).
7. WHEN `Store.SaveCursor` is called followed by `Store.GetCursor` with the same platform and roomID, THE Store SHALL return the timestamp that was saved (round-trip property).

---

### Requirement 10: FTS5 Full-Text Search

**User Story:** As a developer, I want tests for the FTS5 search engine, so that message search returns accurate, relevance-ranked results.

#### Acceptance Criteria

1. WHEN messages containing a specific term are inserted and `SearchEngine.Search` is called with that term, THE SearchEngine SHALL return only messages whose text contains the term.
2. WHEN `SearchEngine.Search` is called with a term that matches no messages, THE SearchEngine SHALL return an empty (non-nil) slice.
3. WHEN `SearchEngine.Search` is called with a `since` duration filter, THE SearchEngine SHALL exclude messages older than the specified duration.
4. WHEN a message is inserted and then its text is updated, THE SearchEngine SHALL return the updated text in search results (FTS5 trigger correctness).
5. WHEN a message is deleted from the `messages` table, THE SearchEngine SHALL NOT return that message in subsequent search results (FTS5 delete trigger correctness).
6. FOR ALL pairs of messages where message A contains the search term more times than message B, THE SearchEngine SHALL rank message A at least as high as message B in BM25-ordered results (metamorphic relevance property).

---

### Requirement 11: IPC Server — Token Authentication Middleware

**User Story:** As a developer, I want tests for the `requireToken` middleware, so that all IPC endpoints are protected against unauthorized access.

#### Acceptance Criteria

1. WHEN a request is made to any IPC endpoint without an `Authorization` header and without a `CHAIND_TOKEN` environment variable, THE IPCServer SHALL respond with HTTP 401.
2. WHEN a request is made with a `Bearer` token that does not exist in the `tokens` table, THE IPCServer SHALL respond with HTTP 401.
3. WHEN a request is made with a token whose `revoked` field is `true`, THE IPCServer SHALL respond with HTTP 401.
4. WHEN a request is made with a valid non-revoked token, THE IPCServer SHALL forward the request to the handler and respond with HTTP 2xx.
5. WHEN a request includes `Authorization: Bearer <token>`, THE IPCServer SHALL strip the `Bearer ` prefix before looking up the token.

---

### Requirement 12: IPC Server — Room Scoping

**User Story:** As a developer, I want tests for room-level capability enforcement, so that scoped tokens cannot access rooms outside their allowlist.

#### Acceptance Criteria

1. WHEN a tier-0 token makes a request with any room parameter, THE IPCServer SHALL allow the request.
2. WHEN a tier-1 token with `rooms="roomA"` makes a GET request with `room=roomA`, THE IPCServer SHALL allow the request.
3. WHEN a tier-1 token with `rooms="roomA"` makes a GET request with `room=roomB`, THE IPCServer SHALL respond with HTTP 403.
4. WHEN a tier-1 token makes a request with an empty room parameter, THE IPCServer SHALL respond with HTTP 403.
5. WHEN a tier-1 token makes a request with `room=*`, THE IPCServer SHALL respond with HTTP 403.
6. WHEN a tier-0 token makes a request with `room=*`, THE IPCServer SHALL allow the request.

---

### Requirement 13: IPC Server — HTTP Endpoints

**User Story:** As a developer, I want tests for every IPC HTTP endpoint, so that the API contract is verified end-to-end.

#### Acceptance Criteria

1. WHEN `GET /api/v1/messages/recent` is called with a valid token, THE IPCServer SHALL respond with HTTP 200 and a JSON array of messages.
2. WHEN `GET /api/v1/messages/recent` is called with a non-GET method, THE IPCServer SHALL respond with HTTP 405.
3. WHEN `GET /api/v1/messages/search?q=term` is called with a valid token, THE IPCServer SHALL respond with HTTP 200 and a JSON array of matching messages.
4. WHEN `GET /api/v1/messages/search` is called without the `q` parameter, THE IPCServer SHALL respond with HTTP 400.
5. WHEN `POST /api/v1/messages/send` is called with a valid payload and a registered MockAdapter, THE IPCServer SHALL respond with HTTP 200 and the sent message JSON.
6. WHEN `POST /api/v1/messages/send` is called with `"require_approval": true`, THE IPCServer SHALL respond with HTTP 200 and a `"status": "queued for approval"` body, and SHALL insert a row into the `approval_queue` table.
7. WHEN `POST /api/v1/messages/send` is called with malformed JSON, THE IPCServer SHALL respond with HTTP 400.
8. WHEN `GET /api/v1/adapters/status` is called with a valid token, THE IPCServer SHALL respond with HTTP 200 and a JSON object containing `"daemon": "running"`.
9. WHEN `POST /api/v1/moderate` is called with a valid payload and a registered MockAdapter, THE IPCServer SHALL respond with HTTP 200 and a `"status": "moderated"` body.
10. WHEN `POST /api/v1/moderate` is called with malformed JSON, THE IPCServer SHALL respond with HTTP 400.

---

### Requirement 14: IPC Server — HitL Approval Queue Endpoints

**User Story:** As a developer, I want tests for the approval queue lifecycle, so that human-in-the-loop message gating works correctly end-to-end.

#### Acceptance Criteria

1. WHEN `GET /api/v1/queue` is called with a valid token, THE IPCServer SHALL respond with HTTP 200 and a JSON array of pending approval items.
2. WHEN a message is enqueued via `POST /api/v1/messages/send` with `require_approval: true` and then `POST /api/v1/queue/exec?id=<id>` is called, THE IPCServer SHALL dispatch the message via the router and delete the row from `approval_queue`.
3. WHEN `POST /api/v1/queue/exec?id=<id>` is called with an unknown ID, THE IPCServer SHALL respond with HTTP 404.
4. WHEN `POST /api/v1/queue/deny?id=<id>` is called with a known ID, THE IPCServer SHALL delete the row from `approval_queue` without dispatching the message, and SHALL respond with HTTP 200 and `"status": "denied"`.
5. WHEN `POST /api/v1/queue/deny?id=<id>` is called without the `id` parameter, THE IPCServer SHALL respond with HTTP 400.

---

### Requirement 15: IPC Server — PII Scrubbing on Read Endpoints

**User Story:** As a developer, I want tests for PII scrubbing applied by the middleware on read responses, so that sensitive data is redacted before leaving the daemon.

#### Acceptance Criteria

1. WHEN a message containing an email address is stored and `GET /api/v1/messages/recent` is called with a token whose `pii_scrub` includes `"email"`, THE IPCServer SHALL replace the email address with `[REDACTED PII]` in the response body.
2. WHEN a message containing a phone number is stored and `GET /api/v1/messages/recent` is called with a token whose `pii_scrub` includes `"phone"`, THE IPCServer SHALL replace the phone number with `[REDACTED PII]` in the response body.
3. WHEN `GET /api/v1/messages/recent` is called with a token whose `pii_scrub` is empty, THE IPCServer SHALL return the original message content unmodified.
4. WHEN PII scrubbing is applied to a response that has already been scrubbed, THE PII_Scrubber SHALL produce the same output (idempotence property).

---

### Requirement 16: IPC Server — SSE Watch Endpoint

**User Story:** As a developer, I want a test for the SSE watch endpoint, so that live message streaming to clients is verified.

#### Acceptance Criteria

1. WHEN `GET /api/v1/messages/watch?platform=mock&room=test` is called with a valid token and a registered MockAdapter, THE IPCServer SHALL respond with `Content-Type: text/event-stream`.
2. WHEN a message is pushed to the MockAdapter's Watch channel, THE IPCServer SHALL emit a `data: <json>\n\n` SSE event containing the message JSON.
3. WHEN the client disconnects, THE IPCServer SHALL close the Watch channel context without leaking goroutines.

---

### Requirement 17: Format Package — Markdown Parser

**User Story:** As a developer, I want tests for the Markdown parser, so that all supported node types are correctly parsed into the internal AST.

#### Acceptance Criteria

1. WHEN `ParseMarkdown` is called with `"**bold**"`, THE Parser SHALL return a slice containing a `TextNode` with `Bold: true`.
2. WHEN `ParseMarkdown` is called with `"*italic*"`, THE Parser SHALL return a slice containing a `TextNode` with `Italic: true`.
3. WHEN `ParseMarkdown` is called with `` "`code`" ``, THE Parser SHALL return a slice containing a `CodeNode`.
4. WHEN `ParseMarkdown` is called with a fenced code block, THE Parser SHALL return a slice containing a `BlockNode` with the correct `Language` and `Content` fields.
5. WHEN `ParseMarkdown` is called with `"[label](https://example.com)"`, THE Parser SHALL return a slice containing a `LinkNode` with the correct `URL` and `Label`.
6. WHEN `ParseMarkdown` is called with an autolink URL, THE Parser SHALL return a slice containing a `LinkNode` whose `URL` and `Label` are both the URL string.
7. WHEN `ParseMarkdown` is called with an empty string, THE Parser SHALL return a non-nil empty slice without panicking.

---

### Requirement 18: Format Package — Renderer Correctness

**User Story:** As a developer, I want tests for all three renderers, so that each platform receives correctly formatted output for every AST node type.

#### Acceptance Criteria

1. WHEN `TelegramRenderer.Render` is called with a bold `TextNode`, THE TelegramRenderer SHALL wrap the content in `<b>...</b>` tags.
2. WHEN `TelegramRenderer.Render` is called with a `LinkNode`, THE TelegramRenderer SHALL produce `<a href="URL">Label</a>`.
3. WHEN `TelegramRenderer.Render` is called with a `MentionNode`, THE TelegramRenderer SHALL produce `<a href="tg://user?id=UserID">DisplayName</a>`.
4. WHEN `TelegramRenderer.Render` is called with a `CodeNode`, THE TelegramRenderer SHALL wrap the content in `<code>...</code>` tags.
5. WHEN `TelegramRenderer.Render` is called with a `BlockNode`, THE TelegramRenderer SHALL produce a `<pre><code class="language-LANG">...</code></pre>` block.
6. WHEN `MatrixRenderer.Render` is called with a bold `TextNode`, THE MatrixRenderer SHALL wrap the content in `**...**`.
7. WHEN `MatrixRenderer.Render` is called with a `LinkNode`, THE MatrixRenderer SHALL produce `[Label](URL)`.
8. WHEN `MatrixRenderer.Render` is called with a `MentionNode`, THE MatrixRenderer SHALL produce `[DisplayName](https://matrix.to/#/UserID)`.
9. WHEN `PlainRenderer.Render` is called with a `MentionNode`, THE PlainRenderer SHALL produce `@DisplayName`.
10. WHEN `PlainRenderer.Render` is called with a `LinkNode`, THE PlainRenderer SHALL produce `Label (URL)`.
11. WHEN `PlainRenderer.Render` is called with a bold `TextNode`, THE PlainRenderer SHALL output the content without any markup.

---

### Requirement 19: Format Package — Parse-Render Round-Trip

**User Story:** As a developer, I want a round-trip property test for the format package, so that parsing then rendering preserves the semantic text content.

#### Acceptance Criteria

1. FOR ALL Markdown strings containing only plain text (no markup), THE Test_Suite SHALL verify that `PlainRenderer.Render(ParseMarkdown(input))` returns a string containing the original text.
2. FOR ALL Markdown strings, THE Test_Suite SHALL verify that `ParseMarkdown` followed by `TelegramRenderer.Render` produces a non-empty string when the input is non-empty.
3. WHEN `ParseMarkdown` is called on the output of `TelegramRenderer.Render` applied to a simple AST, THE Test_Suite SHALL verify the resulting AST contains the same text content (parse-render-parse consistency).

---

### Requirement 20: Outbox Scheduler

**User Story:** As a developer, I want tests for the outbox scheduler, so that scheduled messages are dispatched at the correct time and removed after delivery.

#### Acceptance Criteria

1. WHEN an outbox item with `scheduled_at` set to a past timestamp is present in the database and `processOutbox` is called, THE Scheduler SHALL call `router.Send` with the item's platform, room_id, and content.
2. WHEN `processOutbox` successfully dispatches an outbox item, THE Scheduler SHALL delete that item from the `outbox` table.
3. WHEN an outbox item with `scheduled_at` set to a future timestamp is present and `processOutbox` is called, THE Scheduler SHALL NOT dispatch that item.
4. WHEN `router.Send` returns an error for an outbox item, THE Scheduler SHALL still delete the item from the `outbox` table to prevent infinite retry loops.

---

### Requirement 21: Capability Token System

**User Story:** As a developer, I want tests for the full capability token lifecycle, so that token issuance, validation, scoping, and revocation are all verified.

#### Acceptance Criteria

1. WHEN a token is inserted into the `tokens` table and `Store.GetToken` is called with its name, THE Store SHALL return a Token struct with matching `Tier`, `Rooms`, `PiiScrub`, and `Revoked` fields.
2. WHEN a token with `revoked = true` is used in a request to any IPC endpoint, THE IPCServer SHALL respond with HTTP 401.
3. WHEN a tier-0 token is used, THE IPCServer SHALL allow access to all rooms including wildcard requests.
4. WHEN a tier-1 token with `rooms = "roomA,roomB"` is used to access `roomA`, THE IPCServer SHALL allow the request.
5. WHEN a tier-1 token with `rooms = "roomA,roomB"` is used to access `roomC`, THE IPCServer SHALL respond with HTTP 403.
6. WHEN a token with `pii_scrub = "email,phone"` is used on a GET endpoint, THE IPCServer SHALL apply both email and phone redaction to the response.

---

### Requirement 22: Supervisor Backoff Loop

**User Story:** As a developer, I want tests for the supervisor's exponential backoff behavior, so that adapter restarts are reliable and bounded.

#### Acceptance Criteria

1. WHEN a supervised adapter's `Start` method returns an error, THE Supervisor SHALL call `AdapterRouter.Unregister` for that adapter's platform.
2. WHEN a supervised adapter fails and is retried, THE Supervisor SHALL re-register the adapter before calling `Start` again.
3. WHEN a supervised adapter fails repeatedly, THE Supervisor SHALL double the backoff duration on each failure up to a maximum of 5 minutes.
4. WHEN the supervisor context is cancelled, THE Supervisor SHALL stop retrying and return without leaking goroutines.
5. WHEN a supervised adapter's `Start` returns nil (clean exit without error), THE Supervisor SHALL reset the backoff to 5 seconds before the next attempt.

---

### Requirement 23: CLI Integration Tests

**User Story:** As a developer, I want end-to-end integration tests that exercise CLI commands through the Unix socket, so that the full request path from CLI to adapter is verified.

#### Acceptance Criteria

1. WHEN the `chaind send` CLI command is invoked against an in-process IPC server with a registered MockAdapter, THE Test_Suite SHALL verify that the MockAdapter's `OutboundMessages` slice contains the sent message.
2. WHEN the `chaind read` CLI command is invoked against an in-process IPC server with pre-populated messages, THE Test_Suite SHALL verify that the command output contains the expected message text.
3. WHEN the `chaind search` CLI command is invoked with a query term, THE Test_Suite SHALL verify that the command output contains only messages matching the term.
4. WHEN any CLI command is invoked without a valid `CHAIND_TOKEN`, THE Test_Suite SHALL verify that the command exits with a non-zero status code.

---

### Requirement 24: PII Scrubber Unit Tests

**User Story:** As a developer, I want unit tests for the PII scrubber functions directly, so that each redaction pattern is verified in isolation.

#### Acceptance Criteria

1. WHEN `ScrubMessage` is called on a message containing `user@example.com` with a token whose `pii_scrub` is `"email"`, THE PII_Scrubber SHALL replace the email with `[REDACTED_EMAIL]`.
2. WHEN `ScrubMessage` is called on a message containing a 10-digit Indian mobile number with a token whose `pii_scrub` is `"phone"`, THE PII_Scrubber SHALL replace the number with `[REDACTED_PHONE]`.
3. WHEN `ScrubMessage` is called on a message containing a PAN number (e.g., `ABCDE1234F`) with a token whose `pii_scrub` is `"pan"`, THE PII_Scrubber SHALL replace the PAN with `[REDACTED_PAN]`.
4. WHEN `ScrubMessage` is called on a message with a token whose `pii_scrub` is empty, THE PII_Scrubber SHALL leave the message content unchanged.
5. WHEN `ScrubMessage` is called twice on the same message with the same token, THE PII_Scrubber SHALL produce the same result as calling it once (idempotence property).
6. WHEN `ScrubMessage` is called with a nil token in context, THE PII_Scrubber SHALL leave the message content unchanged without panicking.
