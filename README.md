```markdown
# aegasislabs_assessment

A ChatGPT-powered bot API, migrated from Python/Django to Go using the Go standard library.

---

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go (1.21+) |
| Web framework | Go standard library (`net/http`) |
| Frontend dependencies | Node.js / npm |
| AI provider | OpenAI API |

---

## Prerequisites

- [Go 1.21+](https://go.dev/dl/)
- [Node.js 18+ and npm](https://nodejs.org/)
- An OpenAI API key with access to a supported chat model (e.g., `gpt-4o-mini`)

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

Copy the example environment file and fill in your values:

```bash
cp .env.example .env
```

See the [Environment Variables](#environment-variables) table below for required keys.

### 4. Run the application

```bash
go run ./...
```

Or build and run the binary:

```bash
go build -o aegasislabs_assessment ./...
./aegasislabs_assessment
```

---

## Running Tests

```bash
go test ./...
```

---

## Environment Variables

| Variable | Required | Description |
|---|---|---|
| `OPENAI_API_KEY` | Yes | Your OpenAI API key. Must have access to chat completion endpoints. |

> **Note:** No environment variables were detected automatically during migration. Review `main.go` and `client.go` for any additional configuration that may have been hard-coded in the original Python source and needs to be externalised.

---

## Architecture Overview

The migrated project follows a flat, standard-library-based Go structure:

```
aegasislabs_assessment/
├── main.go          # HTTP server entry point, route registration, request handling
├── client.go        # OpenAI API client wrapper and chat logic
├── go.mod           # Go module definition
├── go.sum           # Dependency lock file
└── package.json     # Frontend/tooling dependencies (npm)
```

- **`main.go`** replaces the Django views and URL routing. It sets up an `http.ServeMux`, defines handlers for the bot API endpoints, and manages the application lifecycle.
- **`client.go`** replaces the original `client.py`. It wraps calls to the OpenAI API and encapsulates request/response parsing.
- No ORM or database layer is present; all state management must be handled explicitly (see [Known Limitations](#known-limitations)).

---

## Migration Notes

This project was migrated from **Python 3 / Django** to **Go / standard library**.

### What changed

| Area | Original (Django/Python) | Migrated (Go/stdlib) |
|---|---|---|
| Web framework | Django (`urls.py`, `views.py`) | `net/http` with `http.ServeMux` |
| HTTP routing | Django URL dispatcher | Manual route registration |
| OpenAI integration | `openai` Python SDK, `openai.Completion.create` | Direct HTTP calls or Go OpenAI SDK |
| Application state | Module-level Python list (`self.prompts`) | Must be externalised (see limitations) |
| Settings/config | `settings.py`, Django environment | Environment variables / `.env` file |
| Dependency management | `pip` / `requirements.txt` | Go modules (`go.mod`) |
| WSGI server | Gunicorn / Django dev server | `http.ListenAndServe` |

### Confidence

Overall migration confidence is **0%**. Both migrated modules (`client.py` → `client.go`, `main.py` → `main.go`) were flagged as low confidence. **Do not deploy without manual verification of all generated Go code.**

---

## Known Limitations

The following components could not be fully or faithfully migrated and require manual intervention before the application is functional.

### 1. Deprecated OpenAI Completion API (`main.py`)

**Component:** `openai.Completion.create` with engine `text-davinci-002`

**Reason:** The legacy Completion API endpoint and the `text-davinci-002` model are deprecated and no longer available in current OpenAI SDKs or via the OpenAI API.

**Required action:** Rewrite the OpenAI integration in `main.go` / `client.go` to use the current chat completions API:

```go
// Use the current OpenAI Go SDK or direct HTTP call:
// POST https://api.openai.com/v1/chat/completions
// Model: "gpt-4o-mini" (or another supported model)
// Response field: choices[0].message.content
```

If using the official Go SDK:

```bash
go get github.com/openai/openai-go
```

Example (current SDK):

```go
response, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
    Model: openai.F(openai.ChatModelGPT4oMini),
    Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
        openai.UserMessage("your prompt here"),
    }),
})
```

---

### 2. In-memory conversation state (`main.py`)

**Component:** `self.prompts` in-memory list inside `ChatGPTBotAPI`

**Reason:** The original code accumulated conversation history in a module-level Python list. This pattern does not survive process restarts, does not work correctly under multiple workers, and cannot be safely migrated to a stateless Go HTTP server as-is.

**Required action:** Replace with a persistent or scoped storage mechanism appropriate for your deployment:

- **Database:** Add a conversation/message table (e.g., SQLite, PostgreSQL via `database/sql`).
- **Redis:** Store session-scoped conversation history keyed by session or user ID.
- **Session store:** Use signed cookies or server-side sessions if single-user/single-session use is acceptable.

---

## Manual Review Required

The following files were migrated with low confidence and **must be manually reviewed and verified** before use:

| File | Concern |
|---|---|
| `client.go` | Migrated from `client.py`. Verify OpenAI API call construction, authentication header injection, and response parsing against the current OpenAI API specification. |
| `main.go` | Migrated from `main.py`. Verify HTTP handler logic, request parsing, response serialisation, error handling, and that conversation state is not held in memory unsafely. |

### Review checklist

- [ ] `client.go`: OpenAI API endpoint is `/v1/chat/completions`, not the deprecated `/v1/completions`.
- [ ] `client.go`: Model name is a currently supported model (e.g., `gpt-4o-mini`), not `text-davinci-002`.
- [ ] `client.go`: Response is parsed from `choices[0].message.content`, not `choices[0].text`.
- [ ] `main.go`: No package-level or handler-level mutable slice used to store conversation history.
- [ ] `main.go`: All error paths return appropriate HTTP status codes.
- [ ] `main.go`: `OPENAI_API_KEY` is read from the environment, not hard-coded.
- [ ] Both files compile without warnings: `go build ./...`
- [ ] All tests pass: `go test ./...`
- [ ] Manual end-to-end test against the OpenAI API with a valid key.

---

## Contributing

1. Ensure `go vet ./...` and `go test ./...` pass before submitting a PR.
2. Address all items in the [Manual Review Required](#manual-review-required) checklist before merging to main.
```