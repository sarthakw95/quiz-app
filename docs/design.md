# quiz-app Design Notes

## Scope

The service accepts quiz requests, fetches data from OpenTriviaDB, transforms it into the local model, persists quiz state in SQLite, and serves attempts and leaderboard results.

This document focuses on why the project is structured the way it is and what tradeoffs were made.

## Architecture Overview

### Package boundaries

1. `internal/httpapi`: HTTP routing, request parsing, response shaping.
2. `internal/quiz`: domain model, service orchestration, repository interfaces, cache logic.
3. `internal/quiz/sqlite`: SQLite-backed repository implementation.
4. `internal/opentdb`: external API client adapter.
5. `internal/userclient`: interactive client and service HTTP calls.
6. `cmd/*`: thin binaries (`quiz-service`, `quiz-user-service`, `quiz-cli`).

## Key Decisions and Tradeoffs

### SQLite for persistence

1. Chosen for low setup cost and local durability.
2. Keeps developer workflow simple (`go run` + local DB file).
3. Configured with `SetMaxOpenConns(1)` and `PRAGMA busy_timeout=5000` to reduce lock contention complexity in local usage.
4. Tradeoff: no built-in horizontal scale or migration framework in this version.
5. `quiz_questions.position` was added for a planned `next_question` progression API. That API is currently deferred, so deterministic ordering is not strictly needed for current behavior, but the column remains to support future host-controlled one-by-one release (for example, bar-trivia style play).

### Simple in-memory cache

1. Quiz and leaderboard reads check cache before DB.
2. Writes are write through with cache, thus not benefiting write performance, trading it with simplicity.
3. Cache is non-persistent and intentionally lock-free for simplicity.
4. Cache is rebuilt from DB on demand after restart, rather than warming / prefetch.
5. No TTL, size cap, or replacement policy is implemented in the current cache.
6. Operationally, the safe reset path is service restart; cache is dropped and rebuilt from SQLite on next reads.
7. Productionization should add bounded eviction (for example LRU/LFU), TTL-based invalidation, and cache metrics to prevent unbounded growth.
8. Important correctness risk: lock-free map access can panic under concurrent read/write traffic in Go.

### Duplicate-attempt enforcement

1. Attempts are unique on `(quiz_id, question_id, username_norm)`.
2. Duplicate submissions are returned as `already_answered`.
3. Existing stored result is reused so client can reconcile local state.

### Create-via-GET tradeoff

1. `GET /questions` supports convenience creation when `quiz_id` is omitted or `create_if_missing=true`.
2. This simplifies the interactive demo flow and reduces client round trips.
3. Tradeoff: creating resources via `GET` is not strict REST best practice because `GET` is expected to be read-only/idempotent.
4. `POST /quizzes` is retained as the explicit, REST-aligned create path and can be used instead.

### Client-side score handling (current mode)

1. User client handles score UX using question correctness metadata.
2. This keeps play-loop code straightforward for the current scope.
3. Tradeoff: this is a trust-the-client model, not secure for adversarial use.
4. Future change: move full scoring authority to server only.

## Data Flow

### Fetch questions

1. Request enters HTTP handler.
2. Service resolves quiz by ID or creates one (depending on query parameters and existence).
3. On create, service calls OpenTriviaDB and stores quiz/questions in SQLite.
4. Response is returned and cache may be warmed.

### Submit responses

1. Request enters `POST /responses`.
2. Service validates question/answer and writes attempts when quiz/user are provided.
3. Duplicate attempts are surfaced without rewriting prior scores.
4. Leaderboard uses cached ranking when present, DB aggregation otherwise.

## Reliability Notes

1. Missing quiz behavior is explicit (`404` unless create-if-missing is requested).
2. Invalid question IDs and invalid answer letters are handled per-item.
3. Leaderboard ordering is deterministic (score desc, submission time asc, username asc).
4. Upstream OpenTriviaDB failure is surfaced to client as fetch/create error.
5. Duplicate attempts are idempotent by key `(quiz_id, question_id, username_norm)`; prior attempt score is returned on re-submit.

## Current Assumptions

1. Single-process deployment (no distributed cache coherence or cross-node coordination).
2. Small concurrent user volume is expected; this is not tuned or load-tested for high-QPS traffic.
3. Username is an unauthenticated logical identifier, not a verified identity.
4. User client is trusted in current mode (it requests `include_correct=true`, receives `correct_index`, and computes local score UX).
5. `POST /responses` without `quiz_id` falls back to in-memory bank validation and is intentionally non-persistent.

## Failure Modes and Current Behavior

1. OpenTriviaDB unavailable/slow:
  - Quiz creation/fetch fails for that request.
  - Server applies bounded retries with backoff for retryable transport failures and retryable HTTP status codes.
2. SQLite lock or transient DB pressure:
  - Busy timeout provides short wait window; request can still fail if contention persists.
  - Single open connection reduces lock complexity but limits write concurrency.
3. Process restart:
  - In-memory cache is lost.
  - Durable state remains in SQLite and cache warms again through subsequent reads.
4. Concurrent submissions for same `(quiz, question, user)`:
  - One insert wins, others are treated as `already_answered` with stored score.
5. Mid-quiz network/service failure in `quiz-user-service`:
  - Per-question persistence is fire-and-forget best effort.
  - Some answers may be shown locally but fail to persist if the async write fails.
6. High-concurrency cache races:
  - Lock-free map access can trigger runtime panic (`concurrent map read and map write`) under heavy concurrent access.
  - Even when panic does not occur, stale ordering/snapshots are possible.
  - SQLite remains source of truth for uncached reads.
7. Adversarial client behavior:
  - `correct_index` is hidden by default, but any caller can still request `include_correct=true`; this can be abused to submit only correct answers and inflate leaderboard score.
  - User identity is unauthenticated (`username` is caller-provided), so clients can impersonate another username.
  - Current behavior is "trust-the-client" by design for demo scope; production hardening requires server-only scoring visibility and authenticated identities.
8. Long-running cache growth:
  - Cache entries are retained until process restart.
  - Workloads with many unique quizzes/users can increase memory usage over time.
  - Current mitigation is restart-based reset; production needs TTL + bounded replacement.

## Scalability Envelope (Current)

1. Reliable target is a local/demo workload with a small number of concurrent users.
2. Practical limits are driven by:
  - single SQLite connection
  - full leaderboard reads before limit slicing
  - lock-free in-process cache state and concurrency risk
3. For larger traffic, expected next steps are:
  - add synchronization or move to concurrency-safe cache model
  - add TTL + bounded eviction/replacement policy for cache memory control
  - add pagination for leaderboard reads
  - tune retry/backoff strategy and connection management
  - move to a production database/runtime topology

## Testing Approach

1. Tests are unit-test heavy across service, handlers, user client, and SQLite repository packages.
2. No dedicated end-to-end suite yet.
3. Manual smoke checks were run for create/fetch/submit/leaderboard flows.

Run tests:

```bash
go test -count=1 ./...
```

## Future Work

1. Move scoring to server-only mode.
2. Add auth and request identity.
3. Tune retries/backoff policy for external API calls (attempt count, jitter, and observability).
4. Add schema migration tooling.
5. Add cache TTL and replacement policy (for example LRU/LFU) with memory and hit-rate metrics.
6. Add synchronization to eliminate lock-free cache panic risk under concurrent traffic.
7. Add integration tests and load tests.
8. Add Docker/Compose for deployment parity.

## Related Docs

1. [README](../README.md)
2. [Diagrams](./diagrams.md)
3. [AI usage](./ai-usage.md)
