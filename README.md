# quiz-app (OpenTriviaDB Quiz Service)

A small production-style Go backend for creating and playing reusable quizzes.

**What it does**

- Calls **OpenTriviaDB** to fetch trivia questions (external network call).
- Transforms third-party payloads into internal domain models (HTML unescape, option shuffling, stable option letters).
- Persists quizzes and user attempts in **SQLite**.
- Exposes HTTP APIs for quiz creation, question retrieval, answer submission, leaderboard, and active quiz discovery.
- Includes two CLIs:
  - `quiz-cli`: simple standalone quiz (no server, fetches directly from OpenTriviaDB).
  - `quiz-user-service`: interactive client for playing quizzes against the server (with leaderboard persistence).

## Contents

- [Quickstart](#quickstart)
- [Repository Layout](#repository-layout)
- [Commands](#commands)
- [Configuration](#configuration)
- [API Summary](#api-summary)
- [Storage (SQLite)](#storage-sqlite)
- [Key Behaviors and Trade-offs](#key-behaviors-and-trade-offs)
- [Testing](#testing)
- [More Docs](#more-docs)

## Quickstart

### Prerequisites

- Go **1.22+**
- `github.com/mattn/go-sqlite3` (Go module dependency; requires CGO-enabled build tooling such as Xcode Command Line Tools on macOS or `gcc` on Linux)
- Internet access (quiz creation pulls from OpenTriviaDB)

### 1) Start the quiz service

```bash
go run ./cmd/quiz-service -addr :8080 -db quiz.db
```

### 2) Create/fetch a quiz and questions

```bash
# Create a new quiz with 5 questions
curl -sS -X POST localhost:8080/quizzes \
  -H 'Content-Type: application/json' \
  -d '{"question_count": 5}'

# Fetch questions for a known quiz ID (replace shared-team-quiz as needed)
curl -sS 'localhost:8080/questions?quiz_id=shared-team-quiz&create_if_missing=true&question_count=5'
```

### 3) Play from the interactive user client

```bash
go run ./cmd/quiz-user-service --username alice --server http://127.0.0.1:8080
```

Then:

- `quizzes [limit]`
- `leaderboard <quiz_id> [limit]`
- `play <quiz_id>`
- `help`
- `exit`

## Repository Layout

```text
cmd/
  quiz-service/        # HTTP backend entrypoint
  quiz-user-service/   # interactive client entrypoint
  quiz-cli/            # standalone quiz runner

internal/
  httpapi/             # handlers, routes, request/response wiring
  quiz/                # domain types, service, interfaces
  quiz/sqlite/         # SQLite store implementation
  opentdb/             # external API client
  userclient/          # HTTP client + user menu flow
  cli/                 # standalone CLI flow

docs/
```

## Commands

### `cmd/quiz-service`

HTTP server with SQLite persistence.

```bash
go run ./cmd/quiz-service -addr :8080 -db quiz.db
```

Optional debug logging:

```bash
go run ./cmd/quiz-service -debug
```

### `cmd/quiz-user-service`

Interactive client that plays quizzes on the server and persists attempts (best-effort, per-question).

```bash
go run ./cmd/quiz-user-service --username alice --server http://127.0.0.1:8080
```

### `cmd/quiz-cli`

Single-player terminal quiz that fetches directly from OpenTriviaDB (no server, no persistence).

```bash
go run ./cmd/quiz-cli
```

## Configuration

`quiz-service` supports flags and env vars:

- `-addr` (default `:8080`) or `ADDR`
- `-db` (default `quiz.db`) or `QUIZ_DB_PATH`
- `-debug` (default `false`) — logs inbound requests (truncated) and outbound OpenTriviaDB calls

Examples:

```bash
ADDR=:9090 QUIZ_DB_PATH=/tmp/quiz.db go run ./cmd/quiz-service
go run ./cmd/quiz-service -addr :9090 -db /tmp/quiz.db
```

## API Summary


| Method | Path                             | Purpose                                             |
| ------ | -------------------------------- | --------------------------------------------------- |
| `GET`  | `/questions`                     | fetch quiz questions (new quiz if `quiz_id` absent) |
| `POST` | `/responses`                     | submit/evaluate responses                           |
| `POST` | `/quizzes`                       | create a quiz                                       |
| `GET`  | `/quizzes/active`                | list recently created quizzes                       |
| `GET`  | `/quizzes/{quiz_id}/leaderboard` | fetch leaderboard                                   |


Full request/response details: [docs/api.md](docs/api.md)

## Storage (SQLite)

SQLite schema is created on service startup (`CREATE TABLE IF NOT EXISTS`).

Tables:

- `quizzes(quiz_id PK, created_at_unix, question_count, locked)`
- `questions(question_id PK, prompt, options_json, correct_index, option_count, source, created_at_unix)`
- `quiz_questions(quiz_id, question_id, position, PK(quiz_id, position), UNIQUE(quiz_id, question_id))`
- `attempts(quiz_id, question_id, username_norm, answer_letter, score, submitted_at_unix, PK(quiz_id, question_id, username_norm))`

**Attempt uniqueness key:** `(quiz_id, question_id, username_norm)`  
This enforces: *a user can answer a question once per quiz*, while still allowing the same question across different quizzes.

`quiz_questions.position` was originally kept to support a planned `next_question` flow. That flow is currently not exposed, so strict deterministic order is not required today, but the column remains useful for future hosted/bar-trivia style one-by-one question release.

## Key Behaviors and Trade-offs

- **Unauthenticated usernames**: `username` is a plain string for now; normalization is `strings.ToLower(strings.TrimSpace(username))`.
- **Idempotency on duplicates**: duplicate submits return `already_answered` and the previously stored score (if previously submitted successfully).
- **Best-effort client persistence**: `quiz-user-service` persists per question asynchronously to reduce loss on mid-quiz exit.
- **Correct answer exposure**: `correct_index` is returned to support the demo client scoring UX; production should not do this.
- **Request caps**: quiz creation and leaderboard fetches are bounded to a maximum of 50 entries per request.
- **OpenTriviaDB retries**: retryable upstream failures use bounded retry + backoff.
- **SQLite choice**: chosen for minimal-friction local persistence. Throughput is intentionally limited by `SetMaxOpenConns(1)`.
- **Cache lifecycle today**: in-memory cache has no TTL or size-based eviction/replacement policy; safe reset is service restart (DB remains source of truth). Production hardening should add TTL + bounded capacity + replacement policy (for example, LRU/LFU).

## Testing

Run all tests:

```bash
go test -count=1 ./...
```

Test focus areas include:

- OpenTriviaDB client decoding and error handling
- Question transformation (HTML unescape, shuffle, correct index)
- Quiz service caching behaviors
- SQLite invariants (duplicate prevention, leaderboard ordering)
- HTTP handlers and routing
- User client flows (play, persistence behavior)

## More Docs

1. [docs/api.md](docs/api.md)
2. [docs/design.md](docs/design.md)
3. [docs/diagrams.md](docs/diagrams.md)
4. [docs/ai-usage.md](docs/ai-usage.md)
5. [docs/THIRD_PARTY_NOTICES.md](docs/THIRD_PARTY_NOTICES.md)



Copyright (c) 2026 Sarthak Wahal.

Repository is published for evaluation and demonstration purposes.
