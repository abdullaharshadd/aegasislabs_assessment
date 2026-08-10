```markdown
# aegasislabs_assessment

A Go application migrated from a Python/Django codebase. Based on the source project, this application
provides an AI-powered prompt interaction interface using the OpenAI API, with basic client/server
communication logic.

> ⚠️ **Migration Confidence: 0%** — This migration required significant manual intervention. Do not
> deploy without completing all steps in the [Manual Review Required](#manual-review-required) section.

---

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go (standard library) |
| HTTP Server | `net/http` (stdlib) |
| Frontend Dependencies | Node.js / npm |
| AI Integration | OpenAI API (requires manual rewrite — see below) |
| Persistence | To be determined — see [Known Limitations](#known-limitations) |

---

## Prerequisites

- [Go](https://golang.org/dl/) 1.21 or later
- [Node.js](https://nodejs.org/) 18 or later and npm
- An OpenAI API account and valid API key
- A database if you implement persistent prompt storage (see [Known Limitations](#known-limitations))

---

## Getting Started

### 1. Clone the Repository

```bash
git clone https://github.com/abdullaharshadd/aegasislabs_assessment.git
cd aegasislabs_assessment
```

### 2. Install Frontend Dependencies

```bash
npm install
```

### 3. Configure Environment Variables

Copy the example environment file and fill in your values:

```bash
cp .env.example .env
```

Edit `.env` with your credentials. See the [Environment Variables](#environment-variables) table below.

> ⚠️ The original source code contained a **hardcoded OpenAI API key**. This has been flagged as
> unmigrable. You **must** set the key via the environment variable `OPENAI_API_KEY` before the
> application will function. Never commit an API key to source control.

### 4. Database Setup

No automated database setup command was detected during migration. If you implement persistent
prompt storage (recommended — see [Known Limitations](#known-limitations)), set up your database
manually and run any migrations before starting the server.

### 5. Run the Application

No start command was detected by the migration tooling. Typical Go entry point:

```bash
go run ./...
```

Or build and run:

```bash
go build -o aegasislabs_assessment .
./aegasislabs_assessment
```

If a `Makefile` is present, check it for a `make run` target.

---

## Running Tests

No test command was detected during migration. To run any Go tests that exist:

```bash
go test ./...
```

> ⚠️ Test coverage from the original Django project has not been verified as migrated. Write
> new tests to cover the Go implementation before relying on this for production use.

---

## Environment Variables

The following environment variables must be set. No variables were auto-detected by the migration
tooling, but the source code analysis identified these as required:

| Variable | Required | Description | Example |
|---|---|---|---|
| `OPENAI_API_KEY` | ✅ Yes | Your OpenAI API secret key. Replaces the hardcoded key found in the original `main.py`. | `sk-...` |

Create a `.env` file at the project root:

```env
OPENAI_API_KEY=sk-your-key-here
```

Make sure your Go application loads this file (e.g., via [`godotenv`](https://github.com/joho/godotenv))
or that these variables are injected by your deployment environment.

---

## Architecture Overview

The migrated project is structured around two primary modules that correspond to the original Python files:

```
aegasislabs_assessment/
├── main.go          # Migrated from main.py — application entry point, HTTP handlers,
│                    # OpenAI interaction logic (INCOMPLETE — see Known Limitations)
├── client.go        # Migrated from client.py — client-side communication logic
├── go.mod           # Go module definition
├── go.sum           # Dependency lock file
├── package.json     # Frontend dependency manifest
└── .env.example     # Template for required environment variables (create this manually)
```

### Request Flow (intended)

```
HTTP Request → net/http router (main.go)
                    │
                    ├─→ Prompt handling logic
                    │         │
                    │         └─→ OpenAI API (chat completions)
                    │
                    └─→ Client logic (client.go)
```

---

## Migration Notes

This project was migrated from **Python 3 / Django** to **Go / standard library**.

### What Changed

| Area | Original (Django) | Migrated (Go) |
|---|---|---|
| Language | Python 3 | Go |
| Web framework | Django | `net/http` (stdlib) |
| OpenAI SDK | `openai` Python library, `openai.Completion.create` | Must be manually rewritten using current SDK or `net/http` calls — see below |
| Prompt storage | In-memory `self.prompts` list | No persistent storage implemented — must be added manually |
| Configuration | Django settings / `django-environ` | Environment variables via `os.Getenv` |
| Secrets management | Hardcoded key in source (unsafe) | `OPENAI_API_KEY` environment variable |
| ORM | Django ORM | None — must be added if persistence is required |

### OpenAI API

The original code used the **deprecated** `text-davinci-002` Completions endpoint
(`openai.Completion.create`). This endpoint is no longer supported by current OpenAI SDK versions.
The migrated Go code **does not have a working OpenAI integration** — it must be rewritten using
the current Chat Completions API. See the [Manual Review Required](#manual-review-required) section.

---

## Known Limitations

The following components could not be automatically migrated and require manual implementation:

### 1. Hardcoded OpenAI API Key (`main.py` → `main.go`)

- **Problem:** The original source contained a plaintext API key embedded in code.
- **Status:** Not carried over to Go (intentionally omitted to avoid a security vulnerability).
- **Action Required:** Load the key from `OPENAI_API_KEY` environment variable using `os.Getenv("OPENAI_API_KEY")`.

### 2. Deprecated OpenAI Completions Endpoint (`main.py` → `main.go`)

- **Problem:** `openai.Completion.create` with `text-davinci-002` is no longer available in current
  OpenAI API versions. There is no direct 1:1 Go equivalent that can be auto-generated.
- **Status:** Not migrated.
- **Action Required:** Implement OpenAI calls using the current Chat Completions API. Recommended
  approach — use [`sashabaranov/go-openai`](https://github.com/sashabaranov/go-openai):

```go
client := openai.NewClient(os.Getenv("OPENAI_API_KEY"))
resp, err := client.CreateChatCompletion(
    context.Background(),
    openai.ChatCompletionRequest{
        Model: openai.GPT4oMini,
        Messages: []openai.ChatCompletionMessage{
            {Role: openai.ChatMessageRoleUser, Content: prompt},
        },
    },
)
```

### 3. In-Memory Prompt List (`main.py` → `main.go`)

- **Problem:** The original `self.prompts` list is in-memory, not concurrency-safe, and does not
  persist across restarts. This cannot be mapped to a stateless deployment.
- **Status:** Not migrated to persistent storage.
- **Action Required:** Choose a storage backend (e.g., PostgreSQL with [`lib/pq`](https://github.com/lib/pq)
  or SQLite with [`mattn/go-sqlite3`](https://github.com/mattn/go-sqlite3)) and implement CRUD
  operations for prompts. At minimum, define a `Prompt` struct and a storage interface.

---

## Manual Review Required

The following files have been flagged as **low confidence** by the migration tooling. A developer
must manually inspect, test, and correct these files before the application is considered functional:

| File | Confidence | Issues to Verify |
|---|---|---|
| `main.go` (from `main.py`) | Low | 1. OpenAI API integration is incomplete and uses a deprecated pattern — must be rewritten. 2. API key must be loaded from environment, not hardcoded. 3. Prompt persistence is not implemented. 4. Verify all HTTP handler logic is correctly translated. |
| `client.go` (from `client.py`) | Low | Verify that all client communication logic has been correctly translated to Go idioms. Check error handling, connection management, and any assumed Django context that no longer exists. |

### Review Checklist

- [ ] `OPENAI_API_KEY` is loaded from environment, not hardcoded anywhere in source
- [ ] OpenAI calls use the current Chat Completions endpoint with a supported model
- [ ] Prompt storage is backed by a persistent data store, not an in-memory slice
- [ ] All HTTP handlers in `main.go` return correct status codes and error responses
- [ ] `client.go` logic has been tested end-to-end
- [ ] No Django-specific assumptions (request middleware, ORM, settings) remain in the Go code
- [ ] `go vet ./...` and `go test ./...` pass without errors
- [ ] A `.env.example` file exists and documents all required environment variables
- [ ] The application has been tested locally before any deployment

---

## Contributing

Given the low migration confidence, all changes to `main.go` and `client.go` should go through
code review before merging.

---

## License

See the original repository for license information:
[https://github.com/abdullaharshadd/aegasislabs_assessment](https://github.com/abdullaharshadd/aegasislabs_assessment)
```