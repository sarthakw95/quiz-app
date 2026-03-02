# HTTP API

Detailed request/response behaviors for the quiz service.

## `POST /quizzes` — Create a quiz

Creates a quiz ID, fetches questions from OpenTriviaDB, and stores quiz + questions.

Request:

```json
{ "question_count": 10 }
```

`question_count` behavior:

- default: `10` when omitted or non-positive in `POST /quizzes`
- capped: maximum `50` questions per create request

Example:

```bash
curl -sS -X POST localhost:8080/quizzes \
  -H 'Content-Type: application/json' \
  -d '{"question_count": 5}'
```

Response (example):

```json
{
  "quiz_id": "qz_ab12cd34ef",
  "question_count": 5,
  "created_at": "2026-03-02T00:00:00Z"
}
```

Status codes:


| Status | Meaning                                   |
| ------ | ----------------------------------------- |
| `201`  | quiz created                              |
| `400`  | invalid JSON body                         |
| `502`  | failed to fetch/create quiz from upstream |
| `405`  | method not allowed                        |


## `GET /questions` — Fetch questions for a quiz

Query params:

- `quiz_id` (optional)
- `create_if_missing` (optional bool): if true, create quiz if missing (reusing the same `quiz_id`)
- `question_count` (optional int, default 10): used when creating a missing quiz or when `quiz_id` is omitted; values above `50` are capped to `50`
- `username` (optional string): if present with `quiz_id`, response includes which questions were already attempted by this user

Example:

```bash
# Always creates a new quiz when quiz_id is omitted:
curl -sS 'localhost:8080/questions?question_count=5'

# Shared quiz id, create only if missing:
curl -sS 'localhost:8080/questions?quiz_id=shared-team-quiz&create_if_missing=true&question_count=5'
```

Response (example shape):

```json
{
  "quiz_id": "shared-team-quiz",
  "question_count": 5,
  "questions": [
    {
      "question_id": "q_abc123...",
      "question": "Question text",
      "options": [{"letter":"A","text":"..."},{"letter":"B","text":"..."}],
      "correct_index": 1,
      "attempt_status": "not_attempted"
    },
    {
      "question_id": "q_def456",
      "question": "Another question",
      "options": [{"letter":"A","text":"..."},{"letter":"B","text":"..."}],
      "correct_index": 0,
      "attempt_status": "already_attempted",
      "attempt_score": 1
    }
  ]
}
```
Note: `correct_index` is intentionally exposed for the interactive demo client; this is not recommended for adversarial clients.

Status codes:


| Status | Meaning                                                           |
| ------ | ----------------------------------------------------------------- |
| `200`  | questions returned                                                |
| `400`  | invalid query params (for example, non-positive `question_count`) |
| `404`  | `quiz_id` not found and `create_if_missing` not enabled           |
| `500`  | internal failure                                                  |
| `502`  | upstream fetch failure when creating a quiz                       |
| `405`  | method not allowed                                                |


## `POST /responses` — Submit answers (and optionally persist to leaderboard)

Body:

```json
{
  "quiz_id": "shared-team-quiz",
  "username": "alice",
  "responses": [
    {"question_id":"q_abc","answer":"A"}
  ]
}
```

Behavior:

- If `quiz_id` + `username` are provided:
  - validates answers against the quiz
  - persists first-time attempts
  - duplicates return `already_answered`
- If `quiz_id` is provided but `username` is omitted:
  - validates against quiz but does not persist for leaderboard
- If `quiz_id` is omitted:
  - validates against an in-memory question bank (best-effort demo mode)

`warnings` behavior:

- `warnings` is included when responses are evaluated but not persisted (missing `quiz_id` or `username`).
- `warnings` is omitted when submissions are fully leaderboard-linked.

Example:

```bash
curl -sS -X POST localhost:8080/responses \
  -H 'Content-Type: application/json' \
  -d '{
    "quiz_id": "shared-team-quiz",
    "responses": [
      {"question_id": "q_abc", "answer": "A"}
    ]
  }'
```

Example response for non-persistent mode:

```json
{
  "results": [
    {"question_id":"q_abc","status":"correct"}
  ],
  "warnings": [
    "responses are not linked to leaderboard unless both quiz_id and username are provided"
  ]
}
```

Per-question statuses:

- `correct`
- `incorrect`
- `already_answered`
- `invalid_question`
- `invalid_letter`

Status codes:


| Status | Meaning                                                 |
| ------ | ------------------------------------------------------- |
| `200`  | responses evaluated (and optionally persisted)          |
| `400`  | invalid JSON body or missing `responses`                |
| `404`  | quiz not found when quiz-scoped validation is requested |
| `500`  | internal failure                                        |
| `405`  | method not allowed                                      |


## `GET /quizzes/{quiz_id}/leaderboard`

Query params:

- `limit` (optional int; defaults to `10`, capped at `50`, and `<=0` is treated as capped "all" = `50`)

Ranking:

1. `total_score` descending
2. `last_submission_at` ascending (earlier wins ties)
3. `username` ascending (determinism)

Example:

```bash
curl -sS 'localhost:8080/quizzes/shared-team-quiz/leaderboard?limit=10'
```

Status codes:


| Status | Meaning                                         |
| ------ | ----------------------------------------------- |
| `200`  | leaderboard returned                            |
| `400`  | invalid `limit` (non-integer) or missing `quiz_id` path value |
| `404`  | quiz not found                                  |
| `500`  | internal failure                                |
| `405`  | method not allowed                              |


## `GET /quizzes/active`

Query params:

- `limit` (optional int, default 10)

Example:

```bash
curl -sS 'localhost:8080/quizzes/active?limit=10'
```

Status codes:


| Status | Meaning                   |
| ------ | ------------------------- |
| `200`  | active quiz list returned |
| `400`  | invalid `limit`           |
| `500`  | internal failure          |
| `405`  | method not allowed        |
