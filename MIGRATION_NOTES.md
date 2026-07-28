# Migration Notes

**Overall confidence:** 0%  
**Recommendation:** REVIEW RECOMMENDED

---

## What was migrated

- `client.py` → `internal/client.go` (78% confidence) ⚠️ needs review
- `main.py` → `internal/main.go` (88% confidence)

## Components that could not be automatically migrated

These components require manual implementation. The migrated code contains
`MIGRATION_NOTE` comments at the relevant locations.

### `openai.Completion.create (in get_response)` in `main.py`
**Reason:** Uses the legacy OpenAI SDK API surface and a deprecated engine name; the modern SDK has a different client-based interface, so a direct 1:1 automatic translation would break.
**Suggestion:** Manually rewrite using the current openai SDK (client.chat.completions.create) with a supported chat model, and load the API key from environment configuration.

### `self.prompts in-memory list (ChatGPTBotAPI state)` in `main.py`
**Reason:** In-memory list state cannot be automatically converted to persistent Django storage and does not work across multiple processes/workers.
**Suggestion:** Introduce a Django model (e.g., Prompt) with a service/repository layer; replace index-based access with primary-key lookups.

## Files requiring manual review

These files were migrated but scored below the confidence threshold.
Review them carefully before merging.

### `client.py`
Confidence: 78%
