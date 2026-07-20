# aegasislabs_assessment

A Go-based web application migrated from a Python/Django codebase. The application provides an AI-powered prompt interface, originally built around OpenAI's Completions API.

> ⚠️ **Migration Confidence: 0% — This project requires significant manual review before it is production-ready.** See [Manual Review Required](#manual-review-required) and [Known Limitations](#known-limitations) below.

---

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go (standard library) |
| Web Framework | Go `net/http` (no third-party framework) |
| Frontend Dependencies | Node.js / npm |
| AI Integration | OpenAI API (requires manual update — see Known Limitations) |
| Database | Not configured (requires manual setup — see Known Limitations) |

---

## Prerequisites

- [Go](https://golang.org/dl/) 1.21 or later
- [Node.js](https://nodejs.org/) 18 or later and npm
- An OpenAI API account with a valid API key
- A supported OpenAI model (e.g., `gpt-4o-mini` or `gpt-3.5-turbo`) — the original model is deprecated

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

Create a `.env` file in the project root (see [Environment Variables](#environment-variables) for the full list):

```bash
cp .env.example .env
# Edit .env and fill in required values
```

> No `.env.example` may exist in the migrated project. Create `.env` manually based on the table below.

### 4. Database setup

> ⚠️ **No database setup commands were detected during migration.** The original Django project used an in-memory list for prompt storage. A persistent database layer has not been implemented in the migrated Go code. You must add this manually before running the application in any meaningful way. See [Known Limitations](#known-limitations).

### 5. Run the application

> ⚠️ **No run command was detected during migration.** A typical Go entrypoint would be:

```bash
go run ./...
```

or, after building:

```bash
go build -o aegasislabs_assessment .
./aegasislabs_assessment
```

Confirm the correct entrypoint by inspecting `main.go` before running.

---

## Running Tests

> ⚠️ **No test command was detected during migration.** To run any Go tests that exist:

```bash
go test ./...
```

To run with verbose output:

```bash
go test -v ./...
```

Check whether test files (`*_test.go`) were generated as part of the migration before assuming test coverage exists.

---

## Environment Variables

No environment variables were automatically detected during migration. Based on the original Python/Django codebase and the OpenAI integration, the following variables are expected to be required:

| Variable | Description | Required | Example |
|---|---|---|---|
| `OPENAI_API_KEY` | Your OpenAI API secret key | Yes | `sk-...` |
| `OPENAI_MODEL` | The model to use for completions | Yes | `gpt-4o-mini` |
| `PORT` | Port the HTTP server listens on | No | `8080` |

> These variables must be verified against the actual Go source files. Add any additional variables discovered during manual review.

---

## Architecture Overview

The migrated codebase follows a flat Go standard library structure:

```
aegasislabs_assessment/
├── main.go              # HTTP server entrypoint, route registration
├── client.go            # OpenAI API client wrapper (migrated from client.py)
├── go.mod               # Go module definition
├── go.sum               # Dependency lockfile
├── package.json         # Frontend dependency manifest
├── package-lock.json    # Frontend dependency lockfile
└── .env                 # Environment variable configuration (not committed)
```

### Request Flow

```
HTTP Request
    └── net/http router (main.go)
            └── Handler functions
                    └── OpenAI client (client.go)
                            └── OpenAI REST API
```

There is no ORM, middleware framework, or template engine unless added manually. The Go standard library handles HTTP routing, JSON encoding/decoding, and HTTP client calls.

---

## Migration Notes

### What changed from the original Django codebase

| Concern | Django (Python) | Go (Standard Library) |
|---|---|---|
| Language | Python 3.x | Go 1.21+ |
| Web framework | Django (views, urls, settings) | `net/http` handlers and `ServeMux` |
| Project structure | Django apps with `models.py`, `views.py`, `urls.py` | Flat `.go` source files |
| Configuration | `settings.py`, `django-environ` | Environment variables loaded at startup |
| ORM / Database | Django ORM with migrations | **Not implemented — manual step required** |
| Prompt state | In-memory list (`self.prompts`) | **Not implemented — manual step required** |
| OpenAI integration | `openai` Python SDK | Direct HTTP calls or Go OpenAI SDK (manual verification required) |
| Dependency management | `pip` / `requirements.txt` | Go modules (`go.mod` / `go.sum`) |
| Frontend assets | Django static files | npm-managed (unchanged) |
| Testing | `pytest` / `unittest` | `go test` |

---

## Known Limitations

The following components could not be fully migrated and **will not function correctly** without manual intervention:

### 1. Deprecated OpenAI Completions endpoint (`main.py` → `main.go`)

- **Problem:** The original code called `openai.Completion.create` with the `text-davinci-002` engine. This endpoint and model have been deprecated and removed by OpenAI and will return errors if called.
- **Impact:** The core AI functionality of the application is non-functional as migrated.
- **Resolution:** Rewrite the OpenAI call to use the Chat Completions endpoint with a supported model:

  ```go
  // Replace the deprecated call with something equivalent to:
  // POST https://api.openai.com/v1/chat/completions
  // Model: "gpt-4o-mini" or "gpt-3.5-turbo"
  // Body: { "model": "gpt-4o-mini", "messages": [{"role": "user", "content": prompt}] }
  ```

  Refer to the [OpenAI Go SDK](https://github.com/openai/openai-go) or the [OpenAI REST API docs](https://platform.openai.com/docs/api-reference/chat) for the current interface.

### 2. In-memory prompt state (`main.py` → `main.go`)

- **Problem:** The original Django code stored prompts in `self.prompts`, an in-memory list attached to a class instance. This pattern does not survive process restarts, does not work across multiple workers, and was not idiomatic even in Django.
- **Impact:** No prompt history or persistence exists in the migrated application.
- **Resolution:** Implement a persistent storage layer. Options include:
  - A SQLite database using [`database/sql`](https://pkg.go.dev/database/sql) with the `mattn/go-sqlite3` driver for simplicity.
  - PostgreSQL or MySQL if a production-grade store is needed.
  - Define a `Prompt` struct and corresponding CRUD functions to replace the list operations.

---

## Manual Review Required

The following files were flagged during migration with **0% confidence**. A developer must manually inspect and verify each before considering the codebase functional.

| File | What to verify |
|---|---|
| `client.go` (migrated from `client.py`) | Confirm the OpenAI HTTP client is correctly constructed. Verify authentication headers (`Authorization: Bearer <key>`), request body serialization, and response parsing match the current OpenAI API contract. |
| `main.go` (migrated from `main.py`) | Confirm route handlers are registered correctly. Replace all calls to the deprecated Completions endpoint. Verify prompt state management has been replaced with a persistent store. Confirm error handling is complete and no panics are present on malformed input. |

### Suggested review checklist

- [ ] `client.go` — OpenAI API base URL is `https://api.openai.com/v1/chat/completions`
- [ ] `client.go` — API key is read from environment, not hardcoded
- [ ] `client.go` — HTTP response error codes are handled (401, 429, 500, etc.)
- [ ] `main.go` — Deprecated `text-davinci-002` / `Completion.create` references removed
- [ ] `main.go` — Prompt persistence replaced with a database-backed solution
- [ ] `main.go` — All HTTP handlers return appropriate status codes
- [ ] `go.mod` — Module path and dependencies are correct
- [ ] Environment variables are documented and loaded before use
- [ ] `npm install` dependencies are actually used by the Go application (verify integration point)
- [ ] End-to-end smoke test performed before any deployment

---

## Contributing

Because this migration was completed at 0% confidence, treat the current state of the repository as a **draft scaffold**, not a working application. Resolve all items in [Manual Review Required](#manual-review-required) and [Known Limitations](#known-limitations) before opening pull requests against this codebase.