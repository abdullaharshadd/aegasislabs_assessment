# Migration Notes

**Overall confidence:** 0%  
**Recommendation:** REVIEW RECOMMENDED

---

## What was migrated

- `client.py` → `internal/client.py.go` (20% confidence) ⚠️ needs review
- `main.py` → `internal/main.py.go` (76% confidence) ⚠️ needs review

## Components that could not be automatically migrated

These components require manual implementation. The migrated code contains
`MIGRATION_NOTE` comments at the relevant locations.

### `ChatGPTBotAPI in-memory self.prompts state` in `main.py`
**Reason:** The prompt list is stored in process memory on a single global instance, which does not survive restarts and breaks under multi-worker/multi-process Django deployments. There is no direct equivalent that preserves the same behavior reliably.
**Suggestion:** Replace with a persistent store: create a Django Prompt model and use the ORM, or use a cache/Redis if ephemeral storage is acceptable. Switch index-based access to PK-based lookups.

### `openai.Completion.create with engine 'text-davinci-002'` in `main.py`
**Reason:** This uses the deprecated legacy OpenAI SDK API and a retired model; it will not work with the current openai library and cannot be auto-translated 1:1.
**Suggestion:** Manually rewrite using the modern openai client (openai>=1.0): instantiate OpenAI() and call client.chat.completions.create with a supported model (e.g., gpt-4o-mini), reading the API key from Django settings/env.

## Files requiring manual review

These files were migrated but scored below the confidence threshold.
Review them carefully before merging.

### `client.py`
Confidence: 20%

### `main.py`
Confidence: 76%
Issues:
  - [warning] The OpenAI completion backend is a stub that always returns an error ('not implemented'). For a valid in-range index, the original returns the completion text with HTTP 200, but the migration will fall through to HTTP 500 'internal server error'. This is explicitly flagged for manual review in the source analysis (no Go equivalent of the deprecated openai SDK), so it is an acceptable/expected migration gap rather than an introduced logic bug.
  - [info] Original Flask uses <int:prompt_index> routing so non-integer paths yield 404. The Go version parses the index inside the handler and returns HTTP 400 with an error JSON for non-integer segments instead of 404. Minor behavioral difference on a malformed-path edge case.
