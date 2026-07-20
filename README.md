```markdown
# aegasislabs_assessment

A chatbot API service migrated from Python/Django to Go using the standard library. The application exposes an HTTP API for interacting with OpenAI's language models, maintaining conversation context across requests.

---

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go (standard library) |
| HTTP Server | `net/http` |
| AI Backend | OpenAI API |
| Frontend dependencies | Node.js / npm |

---

## Prerequisites

- Go 1.21 or later
- Node.js 18 or later and npm
- An OpenAI API key (see [Environment Variables](#environment-variables))

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

Create a `.env` file in the project root (do **not** commit this file):

```bash
cp .env.example .env
```

Edit `.env` and supply your values. See the [Environment Variables](#environment-variables) table below for required keys.

> **Important:** The original source contained a hardcoded `openai_api_key` placeholder. This has been removed. You must supply a real key via the environment before the application will function.

### 4. Run the application

```bash
go run ./...
```

Or build and run the binary:

```bash
go build -o aegasislabs ./...
./aegasislabs
```

---

## Running Tests

```bash
go test ./...
```

> **Note:** Overall migration confidence is 0%. Test coverage of the migrated modules should be treated as a starting point only. See [Manual Review Required](#manual-review-required) before relying on test results.

---

## Environment Variables

| Variable | Required | Description |
|---|---|---|
| `OPENAI_API_KEY` | Yes | Secret key for authenticating with the OpenAI API. Obtain from https://platform.openai.com/api-keys |

Supply these in a `.env` file at the project root or export them into your shell session before running the application. The application reads them via `os.Getenv`.

---

## Architecture Overview

```
aegasislabs_assessment/
├── main.go           # Entry point; HTTP server setup and route registration
├── client.go         # OpenAI API client wrapper; handles request/response lifecycle
├── go.mod
├── go.sum
└── package.json      # Frontend tooling dependencies
```

**Request flow:**

```
HTTP Request
    └── net/http router (main.go)
            └── Handler function
                    └── client.go — builds OpenAI chat completion request
                            └── OpenAI API (HTTPS)
                                    └── Response marshalled to JSON → HTTP Response
```

Conversation context (the prompt history) is currently held **in memory** within the process. See [Known Limitations](#known-limitations) for implications.

---

## Migration Notes

This project was mechanically migrated from Python/Django to Go using the standard library. The following changes were made:

| Area | Django (original) | Go (migrated) |
|---|---|---|
| HTTP framework | Django + Django REST Framework | `net/http` standard library |
| OpenAI SDK | `openai` Python package | Direct HTTPS calls or Go OpenAI client |
| Configuration | `django-environ` / `settings.py` | `os.Getenv` |
| Conversation state | In-memory `list` on class instance | In-memory slice in package scope |
| WSGI server | Gunicorn / `manage.py runserver` | `go run` / compiled binary |
| Database | Django ORM (SQLite/PostgreSQL) | Not present in migrated version |

**The overall migration confidence score is 0%.** This means the automated migration could not verify correctness for either module. Treat all migrated code as a draft requiring full developer review before production use.

---

## Known Limitations

The following components **could not be automatically migrated** and require manual intervention:

### 1. Hardcoded `openai_api_key` — `main.py`

**Reason:** The original source contained a hardcoded secret placeholder string. This cannot be migrated as-is because embedding secrets in source code is a security risk.

**Action required:**
- Ensure `OPENAI_API_KEY` is read exclusively from `os.Getenv("OPENAI_API_KEY")`.
- Never commit a real key to version control.
- Verify no placeholder or literal key string remains in any source file.

---

### 2. Legacy OpenAI API call (`openai.Completion.create`, `engine='text-davinci-002'`) — `main.py`

**Reason:** The original code used the deprecated `Completion` endpoint with the retired `text-davinci-002` model. This endpoint and model are no longer available in current OpenAI API versions.

**Action required:**
Manually rewrite the API call to use the Chat Completions endpoint with a supported model. Example pattern:

```go
// Use the /v1/chat/completions endpoint
// Model: "gpt-4o-mini" or "gpt-3.5-turbo"
payload := map[string]interface{}{
    "model": "gpt-4o-mini",
    "messages": []map[string]string{
        {"role": "user", "content": prompt},
    },
}
```

---

### 3. In-memory prompt history (`ChatGPTBotAPI.prompts`) — `main.py`

**Reason:** The original class stored conversation history in a Python list on a class instance. This state is:
- Lost on every process restart.
- Not shared across multiple server workers or instances.
- Not suitable for production multi-user scenarios.

**Current migrated behaviour:** The Go version retains an equivalent in-memory slice. This has the same limitations.

**Action required:**
If persistence or multi-worker support is needed, replace the in-memory slice with a database-backed store. Suggested approach:
- Add a `conversations` table with columns `(id, session_id, role, content, created_at)`.
- Use `database/sql` with a driver for PostgreSQL or SQLite.
- Reference conversations by session ID rather than list index.

---

## Manual Review Required

The following files have been flagged for mandatory developer review before this code is used in any environment:

| File | Confidence | Issues to verify |
|---|---|---|
| `client.go` | Low (0%) | Correct HTTP request construction; error handling; response parsing matches current OpenAI API schema |
| `main.go` | Low (0%) | Route handlers are complete; no hardcoded secrets remain; in-memory state limitations are acceptable for your use case; OpenAI call uses a non-deprecated model and endpoint |

**Recommended review checklist:**

- [ ] No API keys or secrets exist in any source file
- [ ] OpenAI calls target `/v1/chat/completions` with a currently supported model
- [ ] All HTTP error codes are handled and returned to the client appropriately
- [ ] Conversation history management is suitable for your concurrency and persistence requirements
- [ ] The application behaves correctly under concurrent requests (in-memory state is not race-prone)
- [ ] `npm install` artefacts are understood and scoped — confirm what the frontend tooling is responsible for
- [ ] End-to-end integration test against the real OpenAI API has been performed with a valid key
```