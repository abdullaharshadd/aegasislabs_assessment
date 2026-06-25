```markdown
# aegasislabs_assessment

A chatbot API service that accepts user prompts and returns AI-generated responses. Originally built with Python/Django, this project has been migrated to Node.js/Express.

---

## Tech Stack

- **Runtime:** Node.js
- **Framework:** Express
- **AI Provider:** OpenAI API
- **Language:** JavaScript (CommonJS/ESM)

---

## Prerequisites

- Node.js >= 18.x
- npm >= 9.x
- An OpenAI API key with access to a supported chat model (e.g., `gpt-3.5-turbo` or `gpt-4o-mini`)

---

## Getting Started

### 1. Clone the repository

```bash
git clone https://github.com/abdullaharshadd/aegasislabs_assessment.git
cd aegasislabs_assessment
```

### 2. Install dependencies

```bash
npm install
```

### 3. Configure environment variables

Create a `.env` file in the project root. See the [Environment Variables](#environment-variables) section for required values.

```bash
cp .env.example .env
# then edit .env with your values
```

### 4. Database setup

No database setup is required. The current implementation uses **in-memory state** for prompt storage. See [Known Limitations](#known-limitations) for implications.

### 5. Run the development server

No start script was detected in the migration output. Check `package.json` for the correct entry point, then run:

```bash
node index.js
# or, if a start script is defined:
npm start
```

---

## Running Tests

No test script was detected during migration. Check `package.json` for any test configuration:

```bash
npm test
```

If no tests have been ported yet, this command will likely return an error or a placeholder message. Writing tests for the migrated code is recommended before production use.

---

## Environment Variables

No environment variables were automatically detected during migration. However, based on the original Django project's use of the OpenAI API, the following variable is **required** at runtime and must be set manually:

| Variable         | Required | Description                                              | Example                        |
|------------------|----------|----------------------------------------------------------|--------------------------------|
| `OPENAI_API_KEY` | Yes      | Your OpenAI API secret key used to authenticate requests | `sk-...`                       |
| `PORT`           | No       | Port the Express server listens on (default varies)      | `3000`                         |

> **Note:** The migration tooling reported no environment variables detected (`[]`). Audit all source files — particularly any file that initializes the OpenAI client — to ensure no additional secrets are required.

---

## Architecture Overview

The project was migrated from a Django app structure to a flat Express project. The original 2 Python modules map to the following:

```
aegasislabs_assessment/
├── index.js          # Express app entry point; defines routes
├── package.json      # Node.js project manifest and dependency list
├── .env              # Local environment variables (not committed)
└── ...               # Additional modules migrated from Django views/urls
```

**Request flow:**

1. Client sends a POST request with a prompt payload to an Express route.
2. The route handler appends the prompt to an in-memory list (`prompts` array).
3. The handler calls the OpenAI API to generate a completion.
4. The response is returned to the client as JSON.

> **Important:** Because prompt state is held in memory, it is lost on every server restart and is not shared across multiple Node.js processes.

---

## Migration Notes

The following changes were made when migrating from Django to Express:

| Area | Django (original) | Express (migrated) |
|---|---|---|
| Framework | Django + Django REST Framework | Express.js |
| Language | Python 3.x | Node.js / JavaScript |
| Routing | `urls.py` + view classes/functions | Express `Router` |
| Request/response | Django `HttpRequest` / `JsonResponse` | `req` / `res` Express objects |
| App config | `settings.py` | `.env` + inline config |
| Dependency management | `requirements.txt` / `pip` | `package.json` / `npm` |
| OpenAI SDK | `openai` Python package | `openai` npm package |
| State management | In-memory list in view class | In-memory array in route module |

**Overall migration confidence: 0%** — The automated migration completed structural porting of 2/2 modules, but the core AI integration component could not be faithfully ported (see below). **Do not deploy without completing the manual steps described in the sections below.**

---

## Known Limitations

These components could not be automatically migrated and will cause **runtime failures** if not manually addressed before use.

---

### 1. Deprecated OpenAI Completions API (`main.py`)

**Component:** `openai.Completion.create` using engine `text-davinci-002`

**Why it cannot be directly ported:**
The original code calls the legacy OpenAI Completions API with the `text-davinci-002` model. This model has been **retired by OpenAI** and is no longer available. A 1:1 port of this call to the `openai` npm package would fail at runtime with an API error.

**Required manual fix:**
Rewrite the OpenAI call to use the current SDK interface. Replace the legacy call with:

```javascript
// Using openai npm package >= 4.x
import OpenAI from 'openai';

const client = new OpenAI({ apiKey: process.env.OPENAI_API_KEY });

const response = await client.chat.completions.create({
  model: 'gpt-3.5-turbo', // or 'gpt-4o-mini'
  messages: [
    { role: 'user', content: userPrompt }
  ],
});

const reply = response.choices[0].message.content;
```

This is an **API contract change**, not just a framework migration. The response object shape differs from the legacy API — update any code that parses the response accordingly.

---

### 2. In-memory prompt storage (`main.py` — `ChatGPTBotAPI.prompts`)

**Component:** `ChatGPTBotAPI.prompts` — a plain Python list used to store prompts across requests.

**Why it is problematic:**
- State is lost on every server restart.
- Does not work correctly with multiple Node.js worker processes (e.g., under a process manager like PM2 in cluster mode).
- Index-based addressing (e.g., `prompts[0]`) has no safe equivalent in a stateless multi-worker model.

**Required manual decision:**
Choose one of the following approaches and implement it manually:

- **Option A (Persistent storage):** Introduce a database (e.g., PostgreSQL, MongoDB, SQLite) and a `Prompt` model. Replace index-based lookups with ID/PK-based queries.
- **Option B (Explicit statelessness):** Document that the service is intentionally stateless, remove any cross-request prompt references, and ensure no endpoint relies on previously stored prompts surviving a restart.

---

## Manual Review Required

The following files and components **must be manually reviewed and corrected** by a developer before this codebase is production-ready. The automated migration could not resolve these with sufficient confidence.

| File | Component | Action Required |
|---|---|---|
| `main.py` (migrated output) | `openai.Completion.create` / `text-davinci-002` | Replace with `client.chat.completions.create` using a supported model. Validate response parsing logic. |
| `main.py` (migrated output) | `ChatGPTBotAPI.prompts` in-memory list | Decide on persistence strategy. Implement DB model or explicitly document stateless behavior. |
| All migrated files | Environment variable wiring | Confirm `OPENAI_API_KEY` and any other secrets are correctly loaded from `.env` using `dotenv` or equivalent. |
| `package.json` | `scripts.start` and `scripts.test` | Verify a `start` script exists and points to the correct entry file. Add a `test` script if tests are written. |

> **Confidence warning:** The overall migration confidence score is **0%**. Treat the entire migrated codebase as a draft scaffold that requires line-by-line review, not a deployable artifact.
```