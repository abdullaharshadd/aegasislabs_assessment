```markdown
# aegasislabs_assessment

A Go application migrated from a Python/Django codebase. This app provides an AI-powered conversational interface, wrapping OpenAI's completion API to generate responses from user prompts and maintaining a session prompt history.

> **⚠️ Migration Warning:** Overall migration confidence is **0%**. This codebase requires significant manual review and remediation before it is production-ready. See [Manual Review Required](#manual-review-required) and [Known Limitations](#known-limitations) below.

---

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go (standard library) |
| HTTP Server | `net/http` (Go standard library) |
| AI Integration | OpenAI API (requires manual rewrite — see Known Limitations) |
| Frontend/Assets | Node.js / npm |
| Original Stack | Python 3, Django, OpenAI Python SDK (legacy) |

---

## Prerequisites

- [Go](https://go.dev/dl/) 1.21 or later
- [Node.js](https://nodejs.org/) 18 or later and npm
- An OpenAI API account and API key
- Git

---

## Getting Started

### 1. Clone the repository

```bash
git clone <repository-url>
cd aegasislabs_assessment
```

### 2. Install Node dependencies

```bash
npm install
```

### 3. Configure environment variables

Copy the example env file and fill in your values:

```bash
cp .env.example .env
```

Edit `.env` with your credentials. See the [Environment Variables](#environment-variables) table for all required values.

### 4. Install Go dependencies

```bash
go mod tidy
```

### 5. Run the application

```bash
go run ./...
```

> **Note:** No run command was detected during migration. Verify the correct entry point in `main.go` and update this command if needed.

---

## Running Tests

```bash
go test ./...
```

> **Note:** No test command was detected during migration. Test coverage from the original Django project has not been verified as migrated. Write or port tests manually before relying on this step.

---

## Environment Variables

No environment variables were automatically detected during migration. The following are expected based on application behavior and must be configured manually:

| Variable | Required | Description |
|---|---|---|
| `OPENAI_API_KEY` | Yes | Your OpenAI API key. Required for AI completion functionality. |
| `PORT` | No | Port the HTTP server listens on. Defaults to `8080` if not set. |

> **Action required:** Audit `client.go` (migrated from `client.py`) and `main.go` for any additional environment variables consumed by the application and add them here.

---

## Architecture Overview

The migrated Go project follows a flat standard-library structure derived from the two original Python modules:

```
aegasislabs_assessment/
├── main.go          # Migrated from main.py — HTTP handlers, routing, prompt session logic
├── client.go        # Migrated from client.py — OpenAI API client wrapper
├── go.mod           # Go module definition
├── go.sum           # Dependency checksums
├── package.json     # Node.js dependencies (frontend/tooling)
└── .env.example     # Environment variable template (create manually if absent)
```

### Request Flow

```
HTTP Request
    └── main.go (router / handler)
            └── client.go (OpenAI API call)
                    └── OpenAI API (remote)
```

- **`main.go`** handles HTTP routing, reads user prompts from incoming requests, appends them to an in-memory prompt list, and returns AI-generated responses.
- **`client.go`** wraps communication with the OpenAI API and returns completion responses to the handler.

---

## Migration Notes

The following changes occurred when migrating from Python/Django to Go standard library:

| Area | Django (original) | Go standard (migrated) |
|---|---|---|
| **Framework** | Django with URL routing, views, and middleware | `net/http` with manual route registration |
| **Project structure** | Django app/project layout with `settings.py`, `urls.py`, `views.py` | Flat Go package structure |
| **Configuration** | Django `settings.py` and `django-environ` | Environment variables via `os.Getenv` |
| **Database** | Django ORM with migrations | No database layer migrated (see Known Limitations) |
| **OpenAI SDK** | `openai` Python SDK (legacy, `openai.Completion.create`) | Requires manual integration with current Go OpenAI client (see Known Limitations) |
| **Dependency management** | `pip` / `requirements.txt` | Go modules (`go.mod`) |
| **Server** | Django dev server / Gunicorn (WSGI) | Go `net/http` HTTP server |

---

## Known Limitations

The following components **could not be automatically migrated** and require manual implementation before the application will function correctly.

### 1. OpenAI Completion API — `main.go` (`get_response`)

**Reason:** The original code used the deprecated legacy OpenAI Python SDK method `openai.Completion.create` with the engine `text-davinci-002`. This engine and API endpoint have been removed from OpenAI's current offerings and are unsupported in all current SDK versions.

**Impact:** AI response generation is non-functional until this is resolved.

**Suggested fix:**
Rewrite the OpenAI call in `client.go` and/or `main.go` to use the current chat completions API with a supported model:

```go
// Example using the official Go OpenAI client (github.com/sashabaranov/go-openai)
resp, err := client.CreateChatCompletion(
    context.Background(),
    openai.ChatCompletionRequest{
        Model: openai.GPT4oMini, // or openai.GPT3Dot5Turbo
        Messages: []openai.ChatCompletionMessage{
            {Role: openai.ChatMessageRoleUser, Content: userPrompt},
        },
    },
)
```

Install the client:
```bash
go get github.com/sashabaranov/go-openai
```

---

### 2. In-Memory Prompt History — `main.go` (`self.prompts`)

**Reason:** The original Django application stored the prompt history in an in-memory list (`self.prompts`) on a class instance. This state is lost on every process restart and cannot be shared across multiple server workers or instances.

**Impact:** Prompt history is ephemeral. In a multi-process or containerized deployment this will not behave as expected.

**Suggested fix (choose one):**
- **Persistent store:** Introduce a database (e.g., SQLite via `database/sql`, or PostgreSQL) and store prompts with a session ID as the primary key.
- **Acceptable ephemeral behavior:** If single-process, single-worker deployment is acceptable, explicitly document this constraint and ensure the in-process slice is correctly initialized and protected with a mutex for concurrent requests.

---

## Manual Review Required

The following files have a **low migration confidence score** and must be manually verified by a developer before the application is used:

| File | Migrated From | Issues to Verify |
|---|---|---|
| `main.go` | `main.py` | (1) OpenAI completion API rewrite needed. (2) In-memory prompt list replaced or documented. (3) HTTP route behavior matches original Django URL patterns. (4) Error handling is complete. |
| `client.go` | `client.py` | (1) OpenAI client initialization uses current SDK. (2) API key is read from environment. (3) Response parsing matches expected structure from new chat completions API. |

### Review checklist

- [ ] `OPENAI_API_KEY` is loaded from environment and not hardcoded
- [ ] OpenAI API call uses `chat.completions.create` (or equivalent) with a supported model
- [ ] HTTP handlers return appropriate status codes on error
- [ ] Prompt history storage strategy is decided and implemented
- [ ] All routes from the original Django `urls.py` are replicated in `main.go`
- [ ] `npm install` purpose is identified — confirm what frontend assets or tooling are required and document
- [ ] Tests are written or ported for both `main.go` and `client.go`
- [ ] Application runs and returns valid responses end-to-end before deploying
```