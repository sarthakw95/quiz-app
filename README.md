# quiz-app

A Go backend quiz service that:

1. Calls OpenTriviaDB for question data.
2. Transforms questions into internal quiz models.
3. Persists reusable quizzes and leaderboard.
4. Supports answer submissions.

### Start service

```bash
go run ./cmd/quiz-service -addr :8080 -db quiz.db
```

### Run CLI

```bash
go run ./cmd/quiz-cli
```

### Run user client service

```bash
go run ./cmd/quiz-user-service --username alice --server http://127.0.0.1:8080
```

Commands:

1. `quizzes [limit]`
2. `leaderboard <quiz_id> [limit]`
3. `play <quiz_id>`
4. `help`
5. `exit`

If `play <quiz_id>` uses a missing quiz id, the CLI prompts `yes/no`, creates a quiz with that same `quiz_id` when confirmed, and prints the quiz id before starting.
