# aegasislabs_assessment

A Go-based application migrated from the original Python/Django implementation. Based on the source modules, this project provides a ChatGPT-powered bot API with a client interface for interacting with OpenAI's language models.

> ⚠️ **Migration Confidence: 0%** — This migration required significant manual intervention. Do not treat the migrated Go code as production-ready without thorough review. See [Manual Review Required](#manual-review-required) and [Known Limitations](#known-limitations) sections.

---

## Tech Stack

| Layer | Technology |
|-------|------------|
| Language | Go (standard library) |
| HTTP | `net/http` (stdlib) |
| OpenAI Integration | OpenAI REST API (manual HTTP calls or Go SDK) |
| Frontend / Tooling | Node.js / npm |
| Original Stack | Python 3, Django, `openai` Python SDK |

---

## Prerequisites

- **Go** 1.21 or later — [https://go.dev/dl/](https://go.dev/dl/)
- **Node.js** 18 or later and **npm** — [https://nodejs.org/](https://nodejs.org/)
- An **OpenAI API key** with access to a current model (e.g., `gpt-4o-mini`)

Verify your Go installation:

```bash
go version
```

---

## Getting Started

### 1. Clone the repository

```bash
git clone https://github.com/abdullaharshadd/aegasislabs_assessment.git
cd aegasislabs_assessment
```

### 2. Install Node dependencies

```bash
npm install
```

### 3. Environment setup

Create a `.env` file or export variables directly in your shell. At minimum you will need your OpenAI API key (see [Environment Variables](#environment-variables)):

```bash
cp .env.example .env   # if an example file exists
# then edit .env and fill in required values
```

### 4. Initialize Go modules (if not already committed)

```bash
go mod tidy
```

### 5. Run the application

> ⚠️ No run command was detected during migration. The entry point is likely `main.go` in the project root or a `cmd/` subdirectory. Confirm before running.

```bash
go run ./...
```

or, to build and execute a binary:

```bash
go build -o aegasislabs ./...
./aegasislabs
```

---

## Running Tests

> ⚠️ No test command was detected during migration. Go tests follow the standard pattern below.

```bash
go test ./...
```

To run tests with verbose output:

```bash
go test -v ./...
```

To run tests with coverage:

```bash
go test -cover ./...
```

---

## Environment Variables

No environment variables were automatically extracted during migration. Based on the source code (OpenAI API usage), the following are expected to be required:

| Variable | Required | Description | Example |
|----------|----------|-------------|---------|
| `OPENAI_API_KEY` | ✅ Yes | Your OpenAI API secret key | `sk-...` |
| `OPENAI_MODEL` | Recommended | Model to use (see Known Limitations) | `gpt-4o-mini` |
| `PORT` | Optional | Port for the HTTP server | `8080` |

> **Note:** The original Python code used the `openai` library which reads `OPENAI_API_KEY` from the environment automatically. The Go implementation must handle this explicitly. Verify the actual variable names in the migrated source.

---

## Architecture Overview

The project was migrated from two Python source modules into equivalent Go files:

```
aegasislabs_assessment/
├── go.mod                  # Go module definition
├── go.sum                  # Dependency checksums
├── main.go                 # Migrated from main.py — bot API logic and HTTP handlers
├── client.go               # Migrated from client.py — OpenAI API client wrapper
├── package.json            # Node.js tooling dependencies
└── README.md
```

### Module Responsibilities

| Go File | Migrated From | Responsibility |
|---------|---------------|----------------|
| `main.go` | `main.py` | Defines the `ChatGPTBotAPI` struct, HTTP handler wiring, and the `get_response` method that calls OpenAI |
| `client.go` | `client.py` | Client-facing interface for sending prompts and receiving responses from the bot API |

The application follows a simple request → bot handler → OpenAI API → response pattern. There is no database layer.

---

## Migration Notes

### What Changed from the Original Django Codebase

1. **Framework removed — Django → Go stdlib**
   The original project used Django as its web framework. The Go version uses only the Go standard library (`net/http`). There are no Django models, views, URL routing configs, or `manage.py` commands.

2. **No ORM or database layer**
   Django's ORM has been removed entirely. This project had no apparent persistence layer beyond what OpenAI returns, so no Go database driver was introduced.

3. **Python `openai` SDK → HTTP calls**
   The Python `openai` package was replaced. The Go standard library does not include an OpenAI SDK; the migration either uses direct `net/http` REST calls or the community Go SDK. Verify `go.mod` for the actual dependency.

4. **`requirements.txt` → `go.mod`**
   Python package management has been replaced by Go modules.

5. **`npm install` retained**
   A `package.json` exists in the repository. Node.js tooling (e.g., for a frontend or build pipeline) was not part of the Python→Go migration scope and remains unchanged.

---

## Known Limitations

### Components That Could Not Be Fully Migrated

| File | Component | Reason | Status |
|------|-----------|--------|--------|
| `main.py` | `ChatGPTBotAPI.get_response` — OpenAI Completion API call | Uses the **deprecated** `openai.Completion.create` endpoint with engine `text-davinci-002`, which is **discontinued by OpenAI**. A direct 1:1 translation would call a dead API and will not work. | ❌ Must be manually rewritten |

### Required Manual Fix

The `get_response` method must be rewritten to use the current OpenAI Chat Completions API:

**Original Python (broken pattern):**
```python
# Legacy — DO NOT replicate this
response = openai.Completion.create(
    engine="text-davinci-002",
    prompt=prompt,
    ...
)
```

**Target Go pattern using current API:**
```go
// Use the Chat Completions endpoint with a supported model
// POST https://api.openai.com/v1/chat/completions
// Model: "gpt-4o-mini" or "gpt-4o"
//
// Request body:
// {
//   "model": "gpt-4o-mini",
//   "messages": [{"role": "user", "content": "<prompt>"}]
// }
//
// Response parsing must read from:
// response.choices[0].message.content
// (not response.choices[0].text as in the legacy Completions API)
```

---

## Manual Review Required

The following files have been flagged as low-confidence migrations and **must be reviewed by a developer** before use:

### `client.go` (migrated from `client.py`)

- [ ] Verify that the Go client correctly maps to the original Python client interface
- [ ] Confirm error handling matches expected behavior (Python exceptions vs. Go error returns)
- [ ] Check that HTTP request/response serialization is correct
- [ ] Validate any authentication header construction

### `main.go` (migrated from `main.py`)

- [ ] **Rewrite `get_response` to use `client.chat.completions.create`** — the original API endpoint is discontinued (see [Known Limitations](#known-limitations))
- [ ] Update model name from `text-davinci-002` to a supported model such as `gpt-4o-mini`
- [ ] Update response parsing: the Chat Completions response structure differs from the legacy Completions response
- [ ] Verify HTTP handler routes match the original Django URL patterns
- [ ] Confirm request/response JSON schemas are preserved for any existing clients

### General Checklist

- [ ] Run `go vet ./...` and resolve all warnings
- [ ] Run `go test ./...` and write tests for any untested paths
- [ ] Confirm `OPENAI_API_KEY` is loaded from the environment and never hardcoded
- [ ] Review all `TODO` and `FIXME` comments inserted during migration
- [ ] Test end-to-end with a real OpenAI API key against the live endpoint

---

## Original Project

Source repository: [https://github.com/abdullaharshadd/aegasislabs_assessment](https://github.com/abdullaharshadd/aegasislabs_assessment)
Original stack: Python · Django · openai Python SDK (`text-davinci-002`)