# Diagrams

## 1. Component Diagram

```mermaid
flowchart LR
    U[User] --> UC[quiz-user-service CLI]
    U --> QC[quiz-cli]

    UC -->|HTTP| API[internal/httpapi]
    API --> SVC[quiz.Service]

    SVC --> CACHE[(In-memory cache)]
    SVC --> REPOQ[QuizRepository]
    SVC --> REPOA[AttemptRepository]

    REPOQ --> SQLITE[(SQLite DB)]
    REPOA --> SQLITE

    SVC --> OTDB[OpenTriviaDB]
    QC --> OTDB
```

## 2. Sequence: `play <quiz_id>` Flow

```mermaid
sequenceDiagram
    participant User
    participant Client as quiz-user-service
    participant API as quiz-service HTTP API
    participant Service as quiz.Service
    participant DB as SQLite
    participant OTDB as OpenTriviaDB

    User->>Client: play team-demo-1
    Client->>API: GET /questions?quiz_id=team-demo-1&username=alice&include_correct=true
    API->>Service: GetQuizQuestions

    alt quiz exists
        Service->>DB: read quiz + questions
    else quiz missing and user confirms create
        Service->>OTDB: fetch questions
        Service->>DB: persist quiz/questions
    end

    Service-->>API: questions (+ attempt info)
    API-->>Client: response payload

    loop each unattempted question
        Client->>User: prompt answer
        User-->>Client: answer letter
        Client->>Client: local evaluate using correct_index
        Client-)API: POST /responses (fire-and-forget)
        API->>Service: SubmitResponses
        Service->>DB: INSERT OR IGNORE attempt
    end

    Client->>User: final score summary
```

## 3. Data Model (SQLite)

```mermaid
erDiagram
    QUIZZES {
      string quiz_id PK
      int created_at_unix
      int question_count
      int locked
    }

    QUESTIONS {
      string question_id PK
      string prompt
      string options_json
      int correct_index
      int option_count
      string source
      int created_at_unix
    }

    QUIZ_QUESTIONS {
      string quiz_id PK
      int position PK
      string question_id
    }

    ATTEMPTS {
      string quiz_id PK
      string question_id PK
      string username_norm PK
      string answer_letter
      float score
      int submitted_at_unix
    }

    QUIZZES ||--o{ QUIZ_QUESTIONS : contains
    QUESTIONS ||--o{ QUIZ_QUESTIONS : included_in
    QUIZZES ||--o{ ATTEMPTS : has
    QUESTIONS ||--o{ ATTEMPTS : answered_as
```

Additional schema constraint on `quiz_questions`: `UNIQUE (quiz_id, question_id)`.

`position` was introduced for a planned `next_question` flow. That flow is currently not in the API, so strict ordering is not required in the current implementation, but this column keeps the model ready for hosted one-question-at-a-time quiz release.

## 4. Leaderboard Ordering Rule

```text
Sort order:
1) total_score DESC
2) last_submission_at ASC
3) username ASC
```
