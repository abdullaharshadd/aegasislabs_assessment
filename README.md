# aegasislabs_assessment

A chatbot API service migrated from Python/Django to Go using the standard library. The application exposes an HTTP API for managing and querying a conversational AI backend powered by OpenAI.

---

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go (standard library) |
| HTTP Server | `net/http` |
| Frontend dependencies | Node.js / npm |
| AI Provider | OpenAI API |

---

## Prerequisites

- Go 1.21 or later
- Node.js 18 or later and npm
- An OpenAI API key

---

## Getting Started

### 1. Clone the repository

```bash
git clone https://github.com/abdullaharshadd/aegasislabs_assessment.git
cd aegasislabs_assessment
```

### 2. Install frontend dependencies

```bash
npm install
```

### 3. Configure environment variables

Create a `.env` file in the project root (or export variables directly into your shell). See the [Environment Variables](#environment-variables) section below for required keys.

```bash
cp .env.example .env
# Edit .env and fill in values
```

### 4. Run the application

```bash
go run ./...
```

> **Note:** No automated database setup command was detected during migration. If a database is required by your deployment, configure the connection string in `.env` and apply any schema migrations manually before starting the server.

---

## Running Tests

```bash
go test ./...
```

---

## Environment Variables

> ⚠️ The migration analysis detected no environment variables configured automatically. The variables below are required based on the application's functionality and must be added manually.

| Variable | Required | Description |
|---|---|---|
| `OPENAI_API_KEY` | Yes | Secret key used to authenticate requests to the OpenAI API. Obtain from https://platform.openai.com/api-keys. |

Add additional variables to this table as you complete the manual rewrite steps described in [Known Limitations](#known-limitations).

---

## Architecture Overview

The migrated codebase follows a flat Go package structure using only the standard library:

```
aegasislabs_assessment/
├── main.go          # Entry point; HTTP server setup and route registration
├── client.go        # OpenAI client wrapper (⚠ requires manual review — see below)
├── go.mod
├── go.sum
└── package.json     # Frontend/tooling dependencies
```

### Request Flow

```
HTTP Request
    └─► net/http router (main.go)
            └─► handler function
                    └─► OpenAI client (client.go)
                            └─► OpenAI REST API
```

State management (prompt history) is currently held in memory within the handler layer. See [Known Limitations](#known-limitations) for the implications of this.

---

## Migration Notes

This project was automatically migrated from **Python 3 / Django** to **Go / standard library**. The following summarises what changed:

| Area | Before (Django/Python) | After (Go/standard library) |
|---|---|---|
| HTTP framework | Django views + URL conf | `net/http` handlers and `ServeMux` |
| Project entry point | `manage.py runserver` | `go run ./...` |
| Dependency management | `pip` / `requirements.txt` | Go modules (`go.mod` / `go.sum`) |
| OpenAI integration | `openai` Python package | HTTP calls to OpenAI REST API (see caveat below) |
| Configuration | Django `settings.py` | Environment variables |
| ORM / models | Django ORM | No ORM; in-memory state (see Known Limitations) |

---

## Known Limitations

The automated migration achieved **0% overall confidence**. Two components could not be fully migrated and require manual intervention before the application is production-ready.

### 1. OpenAI API call — `client.py` → `client.go`

**File:** `main.py` (migrated to `client.go`)  
**Component:** `openai.Completion.create` inside `get_response`  
**Reason:** The original code used the legacy Python OpenAI SDK (`openai.Completion.create`) with a deprecated engine name (e.g., `text-davinci-003`). There is no direct 1-to-1 equivalent in Go; the current OpenAI API uses a chat-completions endpoint with a different request/response shape.

**What to do:**

Manually rewrite the OpenAI call in `client.go` to target the current chat completions endpoint:

```
POST https://api.openai.com/v1/chat/completions
```

Use a supported model such as `gpt-4o` or `gpt-3.5-turbo` and load the API key from the `OPENAI_API_KEY` environment variable:

```go
apiKey := os.Getenv("OPENAI_API_KEY")
```

A minimal Go HTTP call structure to replace the legacy completion call:

```go
type ChatRequest struct {
    Model    string        `json:"model"`
    Messages []ChatMessage `json:"messages"`
}

type ChatMessage struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}
```

---

### 2. In-memory prompt list — `ChatGPTBotAPI` state

**File:** `main.py`  
**Component:** `self.prompts` in-memory list on `ChatGPTBotAPI`  
**Reason:** The original class stored conversation history in a Python list on the instance. This pattern does not survive process restarts, cannot be shared across multiple server workers, and was not automatically convertible to any persistent storage layer.

**What to do:**

Replace the in-memory slice in Go with a persistent storage backend. Options:

- **SQLite** (single-process, zero-config): use `database/sql` with the `modernc.org/sqlite` driver.
- **PostgreSQL / MySQL**: use `database/sql` with the appropriate driver and add a connection string environment variable.

Define a `Prompt` struct representing a stored message and introduce a repository layer that handles create, read, and delete by ID. Replace any index-based access in the original logic with primary-key lookups.

---

## Manual Review Required

The following files **must be manually reviewed and corrected** by a developer before the application is considered functional. The automated migration produced output for these files but confidence is too low to rely on the result without verification.

| File | Reason | Priority |
|---|---|---|
| `client.go` | Contains the OpenAI integration rewritten from the legacy Python SDK. The API surface, request structure, and model names are likely incorrect. | **High — app will not function without this** |
| `main.go` | In-memory prompt state must be replaced with persistent storage before deploying to any multi-worker or restartable environment. | **High — data loss on every restart** |

After completing the manual rewrites above, run the full test suite and perform an end-to-end smoke test against the OpenAI API to validate correctness:

```bash
go test ./...
curl -X POST http://localhost:8080/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "Hello"}'
```

---

## Contributing

1. Fix the components listed in [Manual Review Required](#manual-review-required) first.
2. Add integration tests covering the OpenAI client and prompt persistence.
3. Document any new environment variables in the table above before opening a pull request.