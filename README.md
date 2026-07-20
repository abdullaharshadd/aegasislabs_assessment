```markdown
# Aegasis Labs Assessment

A Go application migrated from the original Python/Django codebase. This application interfaces with the OpenAI API to process prompts and return completions.

> ⚠️ **Migration Confidence: 0%** — This migration required significant manual intervention. All migrated files must be reviewed before this application is considered production-ready. See [Manual Review Required](#manual-review-required) and [Known Limitations](#known-limitations) below.

---

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go (standard library) |
| HTTP Server | `net/http` (standard library) |
| Frontend dependencies | Node.js / npm |
| OpenAI Integration | OpenAI REST API (manual implementation required) |

---

## Prerequisites

- [Go](https://golang.org/dl/) 1.21 or later
- [Node.js](https://nodejs.org/) and npm (for frontend assets)
- An OpenAI API key with access to a supported chat model (e.g. `gpt-4o`, `gpt-3.5-turbo`)

---

## Getting Started

### 1. Clone the repository

```bash
git clone https://github.com/abdullaharshadd/aegasislabs_assessment.git
cd aegasislabs_assessment
```

### 2. Set environment variables

Do **not** hardcode your API key in source code. Create a `.env` file or export variables in your shell before running the application.

```bash
export OPENAI_API_KEY="sk-..."
```

See the full [Environment Variables](#environment-variables) table below.

### 3. Install frontend dependencies

```bash
npm install
```

### 4. Build and run the Go application

```bash
go build ./...
go run ./...
```

---

## Running Tests

```bash
go test ./...
```

> **Note:** Test coverage for the migrated modules (`client.go`, `main.go`) is not guaranteed. Write or verify tests manually before relying on CI results.

---

## Environment Variables

| Variable | Required | Description | Example |
|---|---|---|---|
| `OPENAI_API_KEY` | ✅ Yes | Your OpenAI secret API key. Must never be committed to source control. | `sk-abc123...` |

> The original source contained a hardcoded placeholder API key. This has been removed. The application will not start or function correctly without `OPENAI_API_KEY` set in the environment.

---

## Architecture Overview

The migrated code follows a flat Go package structure using the standard library.

```
aegasislabs_assessment/
├── main.go          # Application entry point, HTTP server setup, route handlers
├── client.go        # OpenAI API client wrapper (migrated from client.py)
├── go.mod           # Go module definition
├── go.sum           # Dependency lockfile
├── package.json     # Frontend dependency manifest
└── ...
```

### Request Flow

```
HTTP Request
    └─► main.go (route handler)
            └─► client.go (OpenAI API call via net/http)
                    └─► OpenAI Chat Completions API
                            └─► Response returned to caller
```

Prompt state is handled per-request. There is no persistent storage layer in the current implementation — see [Known Limitations](#known-limitations).

---

## Migration Notes

### What changed from the original Django/Python codebase

| Area | Original (Python/Django) | Migrated (Go/standard) |
|---|---|---|
| Language | Python 3 | Go |
| Framework | Django | `net/http` standard library |
| OpenAI SDK | `openai` Python SDK (legacy, `openai.Completion.create`) | Raw HTTP calls to OpenAI REST API |
| OpenAI model | `text-davinci-002` (retired) | Must be updated to a supported model (e.g. `gpt-3.5-turbo`) |
| API key management | Hardcoded placeholder string in source | Environment variable `OPENAI_API_KEY` |
| Prompt storage | In-memory Python list (`self.prompts`) | Per-request scope only (no persistence) |
| Dependency management | `pip` / `requirements.txt` | Go modules (`go.mod`) |
| Frontend deps | Not applicable | npm (`package.json`) |

---

## Known Limitations

The following components could not be automatically migrated and require manual action before the application will work correctly.

### 1. Hardcoded API Key (`main.py` → `main.go`)

- **Problem:** The original source contained a hardcoded placeholder secret (`openai_api_key`). This value was not carried over and the application will not authenticate with OpenAI without it.
- **Action required:** Ensure `OPENAI_API_KEY` is set as an environment variable. Load it in `main.go` using `os.Getenv("OPENAI_API_KEY")`. Never commit a real key to source control.

### 2. Deprecated OpenAI API usage (`main.py` → `main.go`)

- **Problem:** The original code used `openai.Completion.create` with the `text-davinci-002` model. Both the API endpoint and the model are retired and will return errors against the current OpenAI API.
- **Action required:** Rewrite the OpenAI call in `client.go` to use the Chat Completions endpoint (`POST /v1/chat/completions`) with a currently supported model such as `gpt-3.5-turbo` or `gpt-4o`. Example request body:

```json
{
  "model": "gpt-3.5-turbo",
  "messages": [
    { "role": "user", "content": "<your prompt here>" }
  ]
}
```

### 3. In-memory prompt state (`self.prompts` list)

- **Problem:** The original Django application stored prompts in an in-memory Python list indexed by position. This pattern does not survive process restarts, does not scale across multiple instances, and was not translatable to the Go architecture.
- **Action required:** If prompt history or persistence is required, introduce a storage layer (e.g. PostgreSQL, SQLite, or Redis) and define an appropriate data model. Replace any list-index-based lookups with stable unique identifiers (e.g. UUIDs).

---

## Manual Review Required

The following files were flagged as low-confidence during migration and **must be reviewed by a developer** before the application is used in any environment.

| File | Reason for review |
|---|---|
| `client.go` (migrated from `client.py`) | Low migration confidence (0%). OpenAI client logic may be incomplete, incorrect, or non-functional against the current API. Verify authentication, request construction, response parsing, and error handling. |
| `main.go` (migrated from `main.py`) | Low migration confidence (0%). Route handlers, prompt handling logic, and API key loading must all be manually verified. In-memory state has been removed and must be replaced if persistence is needed. |

---

## Security Notice

- The original repository contained a hardcoded API key placeholder. Ensure your repository history does not contain any real secrets. If a real key was ever committed, rotate it immediately via the [OpenAI dashboard](https://platform.openai.com/api-keys).
- Add `.env` to `.gitignore` if you use a local environment file.

```bash
echo ".env" >> .gitignore
```

---

## Contributing

1. Resolve all items listed under [Manual Review Required](#manual-review-required) before opening a pull request.
2. Ensure `go vet ./...` and `go test ./...` pass with no errors.
3. Do not commit secrets or API keys.
```