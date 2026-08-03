```markdown
# aegasislabs_assessment

A Go application migrated from a Python/Django codebase. Based on the source modules (`client.py`, `main.py`), this app provides an interface for interacting with the OpenAI API, managing conversation prompts, and returning completions — originally built as a Django backend, now ported to Go using the standard library.

> ⚠️ **Migration confidence: 0%** — This migration required significant manual intervention. Do not deploy without completing the manual review steps documented below.

---

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go (1.21+) |
| Web framework | Go standard library (`net/http`) |
| Frontend dependencies | Node.js / npm |
| AI provider | OpenAI API |
| Original stack | Python 3 / Django |

---

## Prerequisites

- [Go](https://golang.org/dl/) 1.21 or later
- [Node.js](https://nodejs.org/) and npm (for frontend assets)
- An OpenAI API key with access to a current supported model (e.g., `gpt-4o`, `gpt-3.5-turbo`)

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

Copy the example env file and fill in your values:

```bash
cp .env.example .env
```

Then edit `.env` with your OpenAI credentials and any other required values. See the [Environment Variables](#environment-variables) table below.

> **Note:** No environment variables were automatically detected during migration. Review `main.py` and `client.py` in the original source to confirm all secrets and config values have been captured.

### 4. Database setup

No automated database setup command was detected during migration.

> **Manual step required:** The original code used an in-memory `self.prompts` list. If a persistent store has been introduced during migration, set up your database manually. See [Known Limitations](#known-limitations) for context.

### 5. Run the application

> **Manual step required:** No run command was detected. Start the Go server with:

```bash
go run .
```

Or build and run the binary:

```bash
go build -o aegasislabs_assessment .
./aegasislabs_assessment
```

---

## Running Tests

> **Manual step required:** No test command was detected during migration.

Run Go tests with:

```bash
go test ./...
```

Run with verbose output:

```bash
go test -v ./...
```

> Tests may not exist yet. The original Django project's test suite was not automatically ported. Write tests before considering this production-ready.

---

## Environment Variables

No environment variables were automatically detected. Based on the source modules, the following are expected to be required:

| Variable | Required | Description |
|---|---|---|
| `OPENAI_API_KEY` | ✅ Yes | Your OpenAI API secret key |
| `OPENAI_MODEL` | Recommended | Model to use (e.g., `gpt-4o`). The original code used the deprecated `text-davinci-002` — this **must** be updated |

> Review `client.py` and `main.py` from the original source to confirm this list is complete. Add any missing variables to `.env` before running.

---

## Architecture Overview

```
aegasislabs_assessment/
├── main.go          # Entry point; HTTP server setup and route registration
│                    # Migrated from main.py
├── client.go        # OpenAI API client wrapper
│                    # Migrated from client.py
├── go.mod           # Go module definition
├── go.sum           # Dependency lock file
├── package.json     # Frontend dependency manifest
└── .env.example     # Environment variable template (create manually if absent)
```

### Request flow

```
HTTP Request
    └── net/http router (main.go)
            └── handler functions
                    └── OpenAI client (client.go)
                            └── OpenAI API (chat completions endpoint)
```

The application exposes HTTP endpoints (defined in `main.go`) that accept prompt input, forward requests to the OpenAI API via the client module (`client.go`), and return completions to the caller.

---

## Migration Notes

### What changed from the original Django codebase

| Area | Original (Django/Python) | Migrated (Go/standard library) |
|---|---|---|
| Language | Python 3 | Go 1.21+ |
| Web framework | Django | `net/http` standard library |
| HTTP routing | Django URL dispatcher + views | `net/http` `ServeMux` handlers |
| OpenAI SDK | `openai` Python package | Go HTTP client or Go OpenAI SDK |
| OpenAI endpoint | `openai.Completion.create` (deprecated) | Must be rewritten — see Known Limitations |
| Response parsing | `choices[0].text` | Must use `message.content` from chat completions response |
| State management | In-memory `self.prompts` list | Requires manual redesign — see Known Limitations |
| Configuration | Django `settings.py` | Environment variables via `.env` |
| WSGI server | Gunicorn / Django dev server | Go built-in HTTP server |
| Dependency management | `pip` / `requirements.txt` | Go modules (`go.mod`) |

---

## Known Limitations

The following components could **not** be automatically migrated and require manual implementation before the application will function correctly.

### 1. Deprecated OpenAI API endpoint (`main.py`)

- **Component:** `openai.Completion.create` with `engine=text-davinci-002`
- **Problem:** This endpoint and model no longer exist in current OpenAI SDKs. A direct port will fail at runtime.
- **Required action:** Rewrite the OpenAI call using the current chat completions API:
  - Use `client.chat.completions.create` (Python) or the equivalent Go SDK method
  - Switch to a supported model such as `gpt-4o` or `gpt-3.5-turbo`
  - Update response parsing from `choices[0].text` to `choices[0].message.content`
- **Affected file:** `client.go` (migrated from `client.py`) and `main.go`

### 2. In-memory prompt state (`main.py`)

- **Component:** `self.prompts` list used for storing conversation history
- **Problem:** An ephemeral, index-based in-memory list does not work correctly in a stateless or multi-process deployment. Data is lost on restart. Index-based access is fragile.
- **Required action:** Replace with a persistent storage layer:
  - Define a data model (database table or key-value store) to hold prompt history
  - Use primary keys or UUIDs instead of list indices for record references
  - Update all read/write/delete operations to use the persistent store
- **Affected file:** `main.go`

---

## Manual Review Required

The following files and components **must be reviewed and verified by a developer** before this application is considered functional.

| File | Component | Issue |
|---|---|---|
| `client.go` | OpenAI API call implementation | Migrated from a deprecated Python API call. Verify the Go implementation uses a current endpoint and supported model. |
| `main.go` | OpenAI completion handler | Calls the deprecated `text-davinci-002` engine. Rewrite required. |
| `main.go` | Prompt state management | In-memory list replaced — confirm a persistent storage solution has been implemented. |
| `main.go` | Response parsing | Original parsed `choices[0].text`. Verify migrated code parses `choices[0].message.content` from chat completion response. |
| `main.go` | Route definitions | Confirm all original Django URL routes have been correctly replicated as Go HTTP handlers. |
| `client.go` | Authentication | Confirm the OpenAI API key is read from the environment and not hardcoded. |
| All | Error handling | Django provides structured error responses by default. Verify the Go handlers return appropriate HTTP status codes and error messages. |

> **Overall migration confidence is 0%.** Every functional area of this codebase should be tested end-to-end before any production use.

---

## Contributing

1. Complete all items in the [Manual Review Required](#manual-review-required) section
2. Add test coverage for all handlers and the OpenAI client
3. Verify the application runs end-to-end with a real OpenAI API key
4. Document any additional environment variables discovered during testing
```