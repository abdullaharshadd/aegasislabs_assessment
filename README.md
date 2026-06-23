```markdown
# aegasislabs_assessment

A ChatGPT-powered bot API, migrated from Python/Django to Go using the standard library.

---

## Tech Stack

| Layer | Technology |
|-------|------------|
| Language | Go (standard library) |
| HTTP | `net/http` |
| Frontend dependencies | Node.js / npm |
| AI backend | OpenAI API (see Migration Notes) |

---

## Prerequisites

- Go 1.21 or later
- Node.js 18 or later and npm
- An OpenAI API key (see Environment Variables)

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

Copy the example below into a `.env` file or export variables directly in your shell. See the [Environment Variables](#environment-variables) section for the full list.

```bash
export OPENAI_API_KEY="sk-..."
```

> **Warning:** The original codebase contained a hardcoded API key in source. This has been flagged for removal. Never commit secrets to version control.

### 4. Run the Go server

```bash
go run .
```

> **Note:** No automated run command was detected during migration. Adjust the entry point (`main.go`) if your binary or port configuration differs.

### 5. Database setup

No database setup step was detected. However, see [Known Limitations](#known-limitations) — in-memory state used in the original code **must** be replaced with persistent storage before the application is production-ready.

---

## Running Tests

No test command was detected in the migration plan. To run any Go tests that exist:

```bash
go test ./...
```

If test files are missing, they should be written as part of the manual review process described below.

---

## Environment Variables

| Variable | Required | Description | Example |
|----------|----------|-------------|---------|
| `OPENAI_API_KEY` | Yes | Secret key for authenticating with the OpenAI API | `sk-abc123...` |

No `.env` loader is included in the standard library. Use a package such as [`github.com/joho/godotenv`](https://github.com/joho/godotenv) or manage secrets via your deployment platform.

---

## Architecture Overview

```
aegasislabs_assessment/
├── main.go          # Entry point; HTTP server setup, route registration
├── client.go        # OpenAI API client wrapper (migrated from client.py)
├── go.mod           # Go module definition
├── package.json     # Frontend/tooling dependencies
└── ...
```

### Request flow

```
HTTP Request
    │
    ▼
net/http Router (main.go)
    │
    ▼
Handler functions
    │
    ▼
OpenAI client (client.go)
    │
    ▼
OpenAI REST API
    │
    ▼
HTTP Response
```

The Go migration replaces Django views with `net/http` handler functions and replaces Django URL patterns with standard library routing. No ORM or middleware framework is included; any persistence layer must be added manually.

---

## Migration Notes

### What changed from the original Django codebase

| Area | Original (Python/Django) | Migrated (Go/standard library) |
|------|--------------------------|-------------------------------|
| Language | Python 3 | Go |
| Web framework | Django | `net/http` (standard library) |
| API client | `openai` Python package | HTTP calls via `net/http` or equivalent |
| Configuration | Django `settings.py` | Environment variables |
| URL routing | `urls.py` patterns | `http.HandleFunc` / `http.ServeMux` |
| Views | Django class/function-based views | Go handler functions |

### Overall migration confidence: 0%

The automated migration completed **2 of 2 modules** structurally, but the confidence score is **0%**. This means the migrated output requires thorough manual review before it can be considered functional. Treat the generated Go files as a starting scaffold, not a working implementation.

---

## Known Limitations

The following components could not be fully migrated automatically. They require manual rewrites before the application will work correctly.

### 1. In-memory prompt storage (`main.py` → `main.go`)

- **Component:** `self.prompts` in-memory list on `ChatGPTBotAPI`
- **Why it wasn't migrated:** In-memory instance state does not survive across requests, processes, or workers. This is equally problematic in both Django and Go and has no automatic equivalent.
- **Required fix:** Replace with persistent storage. Options:
  - A relational database (e.g., PostgreSQL) accessed via [`database/sql`](https://pkg.go.dev/database/sql) or an ORM such as [GORM](https://gorm.io)
  - A cache backend (e.g., Redis) if ephemeral storage is acceptable
  - Replace index-based access with ID/primary-key-based lookups

### 2. Deprecated OpenAI API usage (`main.py` → `main.go`)

- **Component:** `openai.Completion.create` with `engine='text-davinci-002'`
- **Why it wasn't migrated:** This endpoint and model are retired and will return errors with current OpenAI API versions regardless of language.
- **Required fix:** Rewrite the API call to use the current Chat Completions endpoint:
  ```
  POST https://api.openai.com/v1/chat/completions
  model: gpt-4o-mini (or another supported model)
  ```
  This change is independent of the framework migration.

### 3. Hardcoded API key (`main.py` → `main.go`)

- **Component:** `openai_api_key` hardcoded in source
- **Why it wasn't migrated:** Secrets cannot be safely represented in generated code.
- **Required fix:** Load the key exclusively from the environment variable `OPENAI_API_KEY`. Remove any hardcoded value from all source files and verify it is not present in git history.

---

## Manual Review Required

The following files were flagged as low-confidence or containing unmigrable components. A developer **must** manually inspect and verify each one before deployment.

| File | Issue | Priority |
|------|-------|----------|
| `main.go` (from `main.py`) | In-memory state, deprecated OpenAI endpoint, hardcoded secret | **Critical** |
| `client.go` (from `client.py`) | Low automated migration confidence; logic may be incomplete or incorrect | **High** |

### Review checklist

- [ ] Remove any hardcoded `OPENAI_API_KEY` values from all Go source files
- [ ] Replace in-memory prompt list with a persistent storage layer
- [ ] Update OpenAI API calls to use the Chat Completions endpoint with a supported model
- [ ] Verify all HTTP handler signatures and routing match the original endpoint contracts
- [ ] Confirm request/response JSON structures match what frontend code expects
- [ ] Add input validation and error handling to all handlers
- [ ] Write unit and integration tests for `client.go` and request handlers
- [ ] Run `go vet ./...` and `staticcheck ./...` to catch common issues

---

## Contributing

Because this project has a 0% migration confidence score, all pull requests touching migrated files must include:

1. A description of what was manually corrected
2. Tests covering the corrected behavior
3. Confirmation that no secrets are present in the diff

---

## License

See the original repository for license information.
```