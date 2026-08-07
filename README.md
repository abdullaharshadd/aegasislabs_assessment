```markdown
# aegasislabs_assessment — Go/Echo Port

A conversational AI backend originally built with Python/Django, now ported to Go using the Echo web framework. The application provides an HTTP API for submitting prompts to an OpenAI language model and retrieving generated responses.

> **⚠️ Migration Confidence: 0%** — This migration required significant manual intervention. Do not treat the generated Go code as production-ready without thorough review. See [Manual Review Required](#manual-review-required) and [Known Limitations](#known-limitations) below.

---

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go (1.21+) |
| Web Framework | Echo v4 |
| AI Provider | OpenAI API (chat completions) |
| Frontend / Build | Node.js / npm |
| Persistence | ⚠️ Not yet implemented — see Known Limitations |

---

## Prerequisites

- [Go 1.21+](https://go.dev/dl/)
- [Node.js 18+ and npm](https://nodejs.org/) (for frontend/build tooling)
- An [OpenAI API key](https://platform.openai.com/api-keys) with access to a supported chat model (e.g. `gpt-4o-mini` or `gpt-3.5-turbo`)
- A running database instance if you implement persistence (see [Known Limitations](#known-limitations))

---

## Getting Started

### 1. Clone the repository

```bash
git clone https://github.com/aegasislabs/aegasislabs_assessment.git
cd aegasislabs_assessment
```

### 2. Install frontend dependencies

```bash
npm install
```

### 3. Install Go dependencies

```bash
go mod tidy
```

### 4. Configure environment variables

Copy the example env file and fill in your values:

```bash
cp .env.example .env
```

Edit `.env` with your OpenAI credentials and any other required values. See the [Environment Variables](#environment-variables) table below.

### 5. Database setup

> **No automated database setup is currently configured.** The original Django application used in-memory prompt storage, and no persistent database layer has been migrated. Before running the application in any environment beyond local testing, you must implement a persistence layer manually. See [Known Limitations](#known-limitations).

### 6. Run the application

> **No automated run command was detected during migration.** Start the Go server directly:

```bash
go run ./cmd/server
```

Or build and run the binary:

```bash
go build -o bin/server ./cmd/server
./bin/server
```

The Echo server will start on the port defined by your environment configuration (default: `:8080`).

---

## Running Tests

> **No test command was detected during migration.** Run the Go test suite with:

```bash
go test ./...
```

For verbose output:

```bash
go test -v ./...
```

> **Note:** Test coverage for the migrated code may be incomplete. The original Django test suite was not fully portable. Write new tests to cover the OpenAI integration and any persistence logic you add.

---

## Environment Variables

| Variable | Required | Description | Example |
|---|---|---|---|
| `OPENAI_API_KEY` | Yes | Your OpenAI API secret key | `sk-...` |
| `OPENAI_MODEL` | No | Chat model to use (see Known Limitations) | `gpt-4o-mini` |
| `PORT` | No | Port the Echo server listens on | `8080` |

> No environment variables were automatically detected by the migration tooling. The table above reflects variables inferred from the source code. Verify against the actual Go source before deploying.

---

## Architecture Overview

The migrated project follows a flat package structure typical of small Go services:

```
aegasislabs_assessment/
├── cmd/
│   └── server/
│       └── main.go          # Entry point — Echo router setup and server start
├── internal/
│   └── client/
│       └── client.go        # OpenAI API client wrapper (migrated from client.py)
├── go.mod
├── go.sum
├── package.json             # Frontend/build tooling
└── .env.example
```

### Request Flow

```
HTTP Request
    └── Echo Router (main.go)
            └── Handler function
                    └── internal/client (client.go)
                            └── OpenAI Chat Completions API
                                    └── Response returned to caller
```

Routing, middleware, and request/response serialization are handled by Echo. The OpenAI client is isolated in `internal/client` and should be the primary target for manual review and rewriting.

---

## Migration Notes

### What changed from the Django codebase

| Area | Python/Django | Go/Echo |
|---|---|---|
| **Framework** | Django (WSGI) | Echo v4 (net/http) |
| **Routing** | Django URL patterns (`urls.py`) | Echo router (`e.GET`, `e.POST`, etc.) |
| **Request/Response** | Django `HttpRequest` / `JsonResponse` | Echo `Context`, `c.JSON()` |
| **AI Client** | `openai` Python SDK — legacy `Completion.create` | Must be manually rewritten using current OpenAI Go SDK chat completions |
| **State management** | In-memory `self.prompts` list on a class instance | No direct equivalent migrated — requires a persistent store |
| **ORM / Models** | Django ORM | Not migrated — no ORM in place |
| **Settings** | `settings.py` / Django config | Environment variables via `.env` |
| **Admin / Auth** | Django built-in | Not migrated |
| **Dependency management** | `pip` / `requirements.txt` | Go modules (`go.mod`) |

---

## Known Limitations

The following components could not be automatically migrated and require manual implementation before the application is functional.

### 1. OpenAI Completion Endpoint — Retired API

- **Affected file:** `main.py` → `cmd/server/main.go` (handler) and `internal/client/client.go`
- **Original code:** `openai.Completion.create` with `engine='text-davinci-002'`
- **Problem:** The `text-davinci-002` model and the legacy Completions endpoint are retired by OpenAI and cannot be used. The Go migration cannot replicate this call.
- **Required action:** Manually rewrite the OpenAI client call using the [OpenAI Go SDK](https://github.com/openai/openai-go) chat completions API:

```go
// Example replacement using the official OpenAI Go SDK
resp, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
    Model: openai.F(openai.ChatModelGPT4oMini),
    Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
        openai.UserMessage(userPrompt),
    }),
})
```

Use a currently supported model such as `gpt-4o-mini` or `gpt-3.5-turbo`.

---

### 2. In-Memory Prompt Storage — Non-Persistent State

- **Affected file:** `main.py` → any handler managing prompt history
- **Original code:** `self.prompts` — a list on a class instance storing prompt/response pairs by index
- **Problem:** In-memory state is lost on restart, is not safe across multiple workers, and does not translate to a stateless Go HTTP server.
- **Required action:** Replace with a persistent storage layer. Recommended approach:
  - Add a database (e.g. PostgreSQL with `pgx`, or SQLite for local dev)
  - Create a `prompts` table with an auto-increment ID, prompt text, response text, and timestamp
  - Replace any index-based lookups with ID-based database queries

---

## Manual Review Required

The following files were flagged as low-confidence by the migration tool. A developer must manually read, verify, and likely rewrite significant portions of each before the application can be used safely.

| File | Confidence | Issues to Review |
|---|---|---|
| `client.py` → `internal/client/client.go` | Low | OpenAI SDK usage, authentication, error handling, model selection |
| `main.py` → `cmd/server/main.go` | Low | Retired API call, in-memory state, prompt/response handler logic |

### Review checklist

- [ ] Replace the legacy `openai.Completion.create` call with `chat.completions.create` using a supported model
- [ ] Implement persistent storage for prompts and responses (replace `self.prompts`)
- [ ] Verify all HTTP routes match the original Django URL configuration
- [ ] Confirm request validation and error responses match expected API contract
- [ ] Add or port any authentication/authorization that existed in Django middleware
- [ ] Write integration tests for the OpenAI client with a mocked HTTP transport
- [ ] Validate environment variable loading and fail-fast on missing required values
- [ ] Review any CORS configuration if this API is consumed by a browser client
```