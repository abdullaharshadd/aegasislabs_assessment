```markdown
# aegasislabs_assessment

A ChatGPT-powered bot API, migrated from Python/Django to Go using the standard library.

---

## Tech Stack

| Layer     | Technology          |
|-----------|---------------------|
| Language  | Go (standard library) |
| Runtime   | Go 1.21+            |
| Frontend deps | Node.js / npm   |
| AI backend | OpenAI API (modern SDK) |

---

## Prerequisites

- Go 1.21 or later
- Node.js and npm (for frontend/client assets)
- An OpenAI API key with access to a supported model (e.g., `gpt-4o-mini`)

---

## Getting Started

### 1. Clone the repository

```bash
git clone https://github.com/abdullaharshadd/aegasislabs_assessment.git
cd aegasislabs_assessment
```

### 2. Install dependencies

Install frontend dependencies:

```bash
npm install
```

Install Go module dependencies:

```bash
go mod tidy
```

### 3. Environment setup

Copy the example environment file (if present) or create a `.env` file manually:

```bash
cp .env.example .env
```

Edit `.env` and fill in the required values. See the [Environment Variables](#environment-variables) section below.

### 4. Database setup

No automated database setup command was detected during migration.

> ⚠️ **Manual step required:** The original Django application used in-memory state to store prompts. If a persistent store has been added during migration (e.g., PostgreSQL, SQLite, or Redis), configure the connection string in your environment and run any applicable migrations manually. See [Known Limitations](#known-limitations) for context.

### 5. Run the application

```bash
go run ./...
```

Or build and run a binary:

```bash
go build -o aegasislabs_assessment .
./aegasislabs_assessment
```

---

## Running Tests

```bash
go test ./...
```

For verbose output:

```bash
go test -v ./...
```

> ⚠️ Test coverage may be incomplete. The migration produced 0% overall confidence, meaning all test cases should be manually verified before relying on them in CI.

---

## Environment Variables

No environment variables were automatically detected from the original codebase. The following are **required** based on the application's functionality and must be set manually:

| Variable          | Description                                      | Required | Example                        |
|-------------------|--------------------------------------------------|----------|--------------------------------|
| `OPENAI_API_KEY`  | API key for authenticating with the OpenAI API   | Yes      | `sk-...`                       |
| `OPENAI_MODEL`    | Model name to use for completions                | No       | `gpt-4o-mini`                  |
| `PORT`            | Port the HTTP server listens on                  | No       | `8080`                         |

Set these in a `.env` file or export them directly in your shell before running the application:

```bash
export OPENAI_API_KEY="sk-..."
export OPENAI_MODEL="gpt-4o-mini"
export PORT="8080"
```

---

## Architecture Overview

The migrated project follows a flat Go package structure using only the standard library:

```
aegasislabs_assessment/
├── main.go          # HTTP server entry point, route registration, handler logic
├── client.go        # OpenAI API client wrapper (replaces client.py)
├── go.mod           # Go module definition
├── go.sum           # Dependency checksums
└── package.json     # Frontend/Node dependencies
```

**Request flow:**

1. `main.go` starts an `net/http` server and registers routes corresponding to the original Django URL patterns.
2. Handlers in `main.go` process incoming requests (prompt submission, retrieval, etc.).
3. `client.go` wraps calls to the OpenAI API using the modern Go OpenAI SDK or raw HTTP, replacing the original Python `openai.Completion.create` calls.
4. Prompt state management — previously held in an in-memory Python list — must now be handled via an external store (see [Known Limitations](#known-limitations)).

---

## Migration Notes

### What changed from the original Django codebase

| Area | Django (original) | Go/standard (migrated) |
|---|---|---|
| Web framework | Django views + URL routing | `net/http` router in `main.go` |
| AI API calls | `openai.Completion.create`, engine `text-davinci-002` (deprecated) | Modern OpenAI client, `chat.completions.create`, supported model |
| Prompt storage | In-memory `self.prompts` list on `ChatGPTBotAPI` class | Requires external persistent store (not automatically migrated) |
| Dependency management | `pip` / `requirements.txt` | Go modules (`go.mod`) |
| Frontend | npm (unchanged) | npm (unchanged) |
| Configuration | Django `settings.py` | Environment variables |

---

## Known Limitations

The following components could **not** be fully or reliably migrated and require manual implementation before the application is production-ready.

### 1. In-memory prompt state (`main.py` → `main.go`)

**Component:** `ChatGPTBotAPI.self.prompts`

**Problem:** The original Django view stored the prompt list in process memory on a single global class instance. This approach:
- Does not survive process restarts.
- Breaks under any multi-worker or multi-process deployment.
- Has no direct safe equivalent in the Go migration without adding infrastructure.

**Required action:** Replace with a persistent store. Options:
- Add a database (PostgreSQL, SQLite) and create a `prompts` table; use `database/sql` with PK-based lookups instead of index-based access.
- Use Redis for ephemeral but cross-process storage if losing prompts on restart is acceptable.
- Update all handlers that previously used list indexes to use database primary key lookups.

---

### 2. Deprecated OpenAI API usage (`main.py` → `main.go`)

**Component:** `openai.Completion.create` with engine `text-davinci-002`

**Problem:** The original code uses the legacy OpenAI Python SDK completions endpoint with the `text-davinci-002` model, which is retired. This cannot be translated 1:1 and will not function.

**Required action:** Manually rewrite the OpenAI call in `client.go` using the current API:
```go
// Use the modern completions endpoint
client.chat.completions.create(...)
```
Use a supported model such as `gpt-4o-mini` or `gpt-3.5-turbo`. Read the API key from the `OPENAI_API_KEY` environment variable.

---

## Manual Review Required

The following files were migrated with **low confidence** and must be manually reviewed and tested by a developer before use:

| File | Reason for review |
|---|---|
| `client.go` (from `client.py`) | Low migration confidence (0%). Verify that API client initialization, request construction, and response parsing match the original behavior. Confirm the deprecated OpenAI endpoint has been replaced. |
| `main.go` (from `main.py`) | Low migration confidence (0%). Verify all route handlers are present and correct. In-memory prompt state is unmigrated — persistence logic must be added manually. Confirm error handling and HTTP status codes match original expectations. |

> **Overall migration confidence is 0%.** Treat all migrated code as a first draft requiring full manual verification. Do not deploy to production without a thorough code review and integration testing against the OpenAI API with a valid key and supported model.
```