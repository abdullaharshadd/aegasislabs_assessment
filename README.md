```markdown
# aegasislabs_assessment

A prompt management and OpenAI completion service, migrated from Python/Django to Go using the standard library.

---

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Language | Go (standard library) |
| HTTP Server | `net/http` (stdlib) |
| Frontend dependencies | Node.js / npm |
| AI Provider | OpenAI API |

---

## Prerequisites

- Go 1.21+
- Node.js 18+ and npm
- An OpenAI API key with access to a supported model (e.g. `gpt-4o-mini` or `gpt-3.5-turbo`)

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

Copy the example below into a `.env` file or export the variables in your shell. See the [Environment Variables](#environment-variables) section for details.

```bash
export OPENAI_API_KEY="sk-..."
```

### 4. Run the application

```bash
go run ./...
```

> **Note:** No database setup is required for the current build. See [Known Limitations](#known-limitations) for context.

---

## Running Tests

```bash
go test ./...
```

---

## Environment Variables

| Variable | Required | Description | Example |
|----------|----------|-------------|---------|
| `OPENAI_API_KEY` | Yes | API key for authenticating with the OpenAI API | `sk-abc123...` |

No `.env` file loader is included in the Go standard library. Export variables manually or integrate a library such as [`godotenv`](https://github.com/joho/godotenv) if you need file-based configuration.

---

## Architecture Overview

```
aegasislabs_assessment/
├── main.go          # HTTP server bootstrap, route registration, handler logic
├── client.go        # OpenAI API client wrapper (HTTP calls to OpenAI)
├── go.mod           # Go module definition
└── package.json     # Frontend/tooling dependencies (npm)
```

**Request flow:**

```
HTTP Request
    └─► net/http router (main.go)
            └─► Handler function
                    └─► OpenAI client (client.go)
                            └─► OpenAI REST API
```

- **`main.go`** owns routing, request parsing, response serialisation, and in-memory prompt storage.
- **`client.go`** encapsulates all communication with the OpenAI API, keeping transport concerns separate from handler logic.

---

## Migration Notes

### What changed from the original Django/Python codebase

| Area | Original (Django/Python) | Migrated (Go/stdlib) |
|------|--------------------------|----------------------|
| Language | Python 3 | Go |
| Web framework | Django + (implied Flask bootstrap in `main.py`) | `net/http` standard library |
| OpenAI SDK | `openai` Python package (`openai.Completion.create`) | Direct HTTP calls to the OpenAI REST API via `client.go` |
| Persistence | In-memory `self.prompts` list (index-based) | In-memory slice (index-based, same semantics) |
| Server startup | `app.run(debug=True)` / `manage.py runserver` | `http.ListenAndServe` in `main.go` |
| Dependency management | `pip` / `requirements.txt` | Go modules (`go.mod`) |

### Key design decisions

- The Flask-specific `app.run(debug=True)` bootstrap has no equivalent in Go. The server is started with `http.ListenAndServe`; no external runner is needed.
- Django's ORM and `manage.py` scaffolding are not present. If persistent storage is required in the future, add a database driver (e.g. `database/sql` + PostgreSQL) and migrate the in-memory slice to a proper store.

---

## Known Limitations

The following components could **not** be automatically migrated and require manual intervention before the application is production-ready.

### 1. Deprecated OpenAI API usage (`main.py` → `main.go`)

| | Detail |
|-|--------|
| **Component** | `openai.Completion.create` with engine `text-davinci-002` |
| **Reason** | This is a deprecated OpenAI SDK method and model. The `text-davinci-002` engine is no longer reliably available and the `Completion` endpoint is legacy. |
| **Impact** | API calls to OpenAI will fail at runtime. |
| **Fix** | Update `client.go` to call the **Chat Completions** endpoint (`POST /v1/chat/completions`) with a supported model. Example request body: `{"model": "gpt-4o-mini", "messages": [{"role": "user", "content": "<prompt>"}]}`. Parse the response from `choices[0].message.content` instead of `choices[0].text`. |

### 2. In-memory prompt persistence (`main.py` → `main.go`)

| | Detail |
|-|--------|
| **Component** | `self.prompts` list used as an index-based store |
| **Reason** | A slice with integer indices cannot be blindly translated to a persistent store without semantic changes. |
| **Impact** | All stored prompts are lost on server restart. Concurrent writes are not safe. |
| **Fix** | Either (a) add a mutex and accept statelessness, or (b) introduce a database (e.g. PostgreSQL via `database/sql`) with a `prompts` table and primary-key lookups replacing index lookups. |

### 3. Flask/Django server bootstrap (`main.py`)

| | Detail |
|-|--------|
| **Component** | `app.run(debug=True)` and Django `manage.py` entrypoint |
| **Reason** | Framework-specific bootstrap has no direct Go equivalent. |
| **Impact** | None — already resolved. `http.ListenAndServe` replaces this in `main.go`. |
| **Action** | Confirm the listening address and port in `main.go` match your deployment requirements. |

---

## Manual Review Required

The following files were migrated with **0% overall confidence** and must be reviewed by a developer before merging or deploying.

| File | What to verify |
|------|---------------|
| `client.go` | OpenAI HTTP request construction, authentication header (`Authorization: Bearer <key>`), response struct definitions, and error handling. Confirm the endpoint is `/v1/chat/completions`, not the legacy `/v1/completions`. |
| `main.go` | Route definitions match the original URL patterns. Handler logic correctly reads/writes the in-memory prompt store. Response serialisation (JSON) matches the contract expected by frontend clients. Prompt index-based lookup is bounds-checked to avoid panics. |

> **Recommendation:** Run the original Django application alongside the Go service and compare responses endpoint-by-endpoint before decommissioning the Python codebase.

---

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b fix/openai-client`)
3. Commit your changes
4. Open a pull request against `main`
```