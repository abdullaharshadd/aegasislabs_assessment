```markdown
# Aegasis Labs Assessment

A Go application migrated from a Python/Django codebase. Based on the original source, this app provides a prompt management and AI completion service, exposing HTTP endpoints to create, retrieve, and delete prompts, and to generate responses via the OpenAI API.

> **⚠️ Migration Confidence: 0%** — The automated migration was unable to produce verified output. Every file listed under [Manual Review Required](#manual-review-required) must be inspected and rewritten by a developer before this project is production-ready.

---

## Tech Stack

| Layer | Technology |
|-------|------------|
| Language | Go (standard library) |
| HTTP Server | `net/http` (standard library) |
| AI Provider | OpenAI API |
| Frontend dependencies | Node.js / npm |

---

## Prerequisites

- [Go](https://go.dev/dl/) 1.21 or later
- [Node.js](https://nodejs.org/) 18 or later and npm
- An OpenAI API key with access to a supported model (e.g. `gpt-4o-mini`)

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

Copy the example env file (if present) or create a `.env` file manually. See the [Environment Variables](#environment-variables) table below for required keys.

```bash
cp .env.example .env   # if .env.example exists
# then edit .env and fill in values
```

### 4. Run the application

> **Note:** No run command was detected during migration. The entry point below is a likely convention for a Go project; verify the correct command in `main.go` before running.

```bash
go run ./...
```

Or build and run:

```bash
go build -o aegasislabs_assessment .
./aegasislabs_assessment
```

---

## Running Tests

> **Note:** No test command was detected during migration. Run Go tests with:

```bash
go test ./...
```

---

## Environment Variables

> **Note:** No environment variables were automatically detected. The variables below are inferred from the original Python source and must be confirmed against the migrated Go code.

| Variable | Required | Description |
|----------|----------|-------------|
| `OPENAI_API_KEY` | Yes | Your OpenAI secret key. Must have access to the Chat Completions API. |

Set variables in your shell or in a `.env` file loaded by your process manager. The Go standard library does not load `.env` files automatically; use a library such as [`godotenv`](https://github.com/joho/godotenv) or export variables in your shell:

```bash
export OPENAI_API_KEY=sk-...
```

---

## Architecture Overview

The migrated project follows a flat Go package structure using the standard library. Expected layout after migration:

```
aegasislabs_assessment/
├── main.go          # Entry point; HTTP server setup and route registration
├── client.go        # OpenAI API client wrapper (migrated from client.py)
├── go.mod
├── go.sum
└── ...              # Frontend assets / package.json
```

### Request Flow

```
HTTP Request
    └── net/http ServeMux (main.go)
            ├── POST   /prompts      → create prompt
            ├── GET    /prompts      → list prompts
            ├── DELETE /prompts/{id} → delete prompt
            └── POST   /completions  → call OpenAI via client.go
```

---

## Migration Notes

### What changed from the original Django codebase

| Area | Django (Python) | Go (standard library) |
|------|-----------------|----------------------|
| Framework | Django + DRF | `net/http` standard library |
| Routing | `urls.py` URL patterns | `http.ServeMux` |
| Serialization | DRF Serializers | `encoding/json` |
| ORM / persistence | Django ORM + database | **None — see Known Limitations** |
| OpenAI client | `openai` Python package | Manual HTTP calls or Go OpenAI SDK |
| Configuration | `settings.py` / env | Environment variables |
| WSGI server | Gunicorn / Django dev server | Go built-in `http.ListenAndServe` |
| Testing | `pytest` / Django test client | `go test` + `net/http/httptest` |

---

## Known Limitations

The following components **cannot function as migrated** and require manual intervention before the application will work correctly.

### 1. Deprecated OpenAI Completions endpoint (`main.go` / `main.py`)

- **Problem:** The original code calls `openai.Completion.create` with the engine `text-davinci-002`. This model and the legacy Completions endpoint have been **retired by OpenAI** and will return errors.
- **Impact:** All AI completion functionality is broken.
- **Required action:** Rewrite the OpenAI call to use the Chat Completions API:

  ```go
  // Replace the legacy call with something like:
  // POST https://api.openai.com/v1/chat/completions
  // Body: {"model": "gpt-4o-mini", "messages": [{"role": "user", "content": prompt}]}
  ```

  Refer to the [OpenAI Chat Completions documentation](https://platform.openai.com/docs/api-reference/chat).

### 2. In-memory prompt storage (`main.go` / `main.py`)

- **Problem:** The original code stores prompts in an in-memory list (`self.prompts`). The migrated Go code likely reproduces this pattern. In-memory storage is lost on every restart, is not shared across multiple processes or workers, and uses positional list indices as identifiers (fragile and unsafe for concurrent access).
- **Impact:** Data is not durable; concurrent requests can corrupt the list.
- **Required action:**
  - Introduce a persistent storage layer (e.g. PostgreSQL, SQLite, or another database).
  - Define a `Prompt` struct/model with a stable primary key (UUID or auto-increment integer).
  - Update all endpoints to reference prompts by ID rather than list index.
  - Add a DB setup step to this README once a database is chosen.

---

## Manual Review Required

The overall migration confidence is **0%**. Every file must be reviewed by a developer. The following files have been specifically flagged:

| File | Status | Reason |
|------|--------|--------|
| `client.go` (from `client.py`) | 🔴 Needs full review | Low confidence migration; OpenAI client logic must be rewritten to use the current Chat Completions API. Verify all HTTP request construction, authentication headers, and error handling. |
| `main.go` (from `main.py`) | 🔴 Needs full review | Low confidence migration; contains deprecated API call and in-memory storage (see Known Limitations). Route handlers, request parsing, response serialization, and concurrency safety must all be verified. |

### Review checklist

- [ ] `main.go` — confirm HTTP routes match the original Django `urls.py`
- [ ] `main.go` — replace in-memory slice with persistent storage
- [ ] `main.go` — replace legacy `Completion.create` call
- [ ] `client.go` — verify OpenAI Chat Completions request/response structs
- [ ] `client.go` — confirm `OPENAI_API_KEY` is read correctly from the environment
- [ ] All handlers — add input validation and proper HTTP error responses
- [ ] All handlers — verify correct HTTP status codes are returned
- [ ] Concurrency — ensure any shared state is protected with `sync.Mutex` or equivalent
- [ ] Tests — write or restore unit/integration tests (`go test ./...`)
- [ ] `npm install` / frontend — confirm what the frontend dependencies are for and whether a build step is needed
```