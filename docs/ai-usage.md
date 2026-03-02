# AI Usage

## Guiding Principle

I used AI as a productivity multiplier for repetitive implementation work, not as a substitute for engineering judgment. Core design choices, tradeoffs, and correctness validation remained manual responsibilities.

The intent was to improve delivery speed without outsourcing critical thinking.

## What I Built Without AI

Before introducing AI into the workflow, I implemented the initial foundation manually:

1. Initial CLI implementation.
2. Basic OpenTriviaDB fetch calls.
3. Single-user `cli` quiz flow.
4. Initial REST endpoints for fetching questions.

During this phase, I relied on:

1. Web references.
2. Re-familiarizing myself with Go after a long gap.
3. Manual reasoning through API and package structure.



This stage itself is a working cli single user quiz application, with an external API call along with exposing endpoint and non persistent storage.

## Where AI Was Used

### Storage and caching implementation

1. I designed the database schema and selected SQLite myself.
2. I used AI to help generate parts of repository/storage code from high-level structure.
3. I used Cursor code completion to speed up DB integration and caching logic wiring.

### User client implementation

1. I defined the required API calls and flow behavior.
2. I used code completion and generation support for implementation details.
3. I manually fixed and adjusted generated code.
4. For the interactive service, I prompted AI to replicate and adapt the initial CLI style/menu logic, then iterated with significant manual corrections.

### Testing and refactoring support

1. Manual API sanity testing was done via Postman.
2. AI generated the unittests.
3. I reviewed coverage quality and made targeted fixes where needed.
4. I used AI prompts to identify boundary-separation and refactor opportunities, while keeping behavior aligned with the original implementation intent.

### Comments and documentation

1. I manually added decision/tradeoff comments.
2. I used AI to identify unclear sections and suggest where explanation was needed (especially deviations from best practices).
3. Documentation content was manually drafted/spoken, then AI-assisted for structure, gap analysis, and clarity improvements.
4. For diagrams, I manually learned and authored the component diagram; remaining diagrams were AI-generated for time efficiency and presentation consistency.

## Workflow Impact

AI improved velocity most in:

1. Repetitive code generation (handlers/repositories/tests).
2. Refactor mechanics (moving code across files/packages cleanly).
3. Documentation structuring and polishing.

This allowed me to spend more time on:

1. Architecture decisions.
2. Correctness constraints and edge cases.
3. Reviewing generated output critically.

## Tradeoffs and Risks

Using AI introduced tradeoffs I had to actively manage:

1. Potential over-generation and unnecessary complexity.
2. Occasional incorrect assumptions in generated code.
3. Need for stricter review to preserve consistency and intended behavior.

Mitigations I used:

1. Manual review and iterative fixes.
2. Re-running tests frequently (`go test -count=1 ./...`).
3. Manual endpoint sanity checks in Postman and E2E flow with spinning up quiz-service and multiple quiz-user-service.
4. Explicitly preserving design intent during refactors.



Overall, it allowed showcasing of both work and learning, while being able to deliver a not trivial sized working and enjoyably and fruitfully usable product in a reasonable amount of time. 