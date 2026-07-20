# Migration Notes

**Overall confidence:** 0%  
**Recommendation:** REVIEW RECOMMENDED

---

## What was migrated

- `client.py` → `internal/client.go` (75% confidence) ⚠️ needs review
- `main.py` → `internal/server.go` (78% confidence) ⚠️ needs review

## Components that could not be automatically migrated

These components require manual implementation. The migrated code contains
`MIGRATION_NOTE` comments at the relevant locations.

### `openai_api_key (hardcoded value)` in `main.py`
**Reason:** Hardcoded secret placeholder cannot and should not be migrated as-is.
**Suggestion:** Move to environment variables using django-environ or os.environ; provide via .env / settings and reference in code.

### `openai.Completion.create with engine='text-davinci-002'` in `main.py`
**Reason:** Uses the legacy OpenAI SDK API and a deprecated/retired model, which will not work with current openai library versions.
**Suggestion:** Manually rewrite using the current OpenAI Python SDK (OpenAI().chat.completions.create) with a supported model such as gpt-4o-mini or gpt-3.5-turbo.

### `In-memory prompts list (ChatGPTBotAPI.prompts)` in `main.py`
**Reason:** Global in-process state has no direct database equivalent and does not survive restarts or scale across workers; it is not a 1:1 mechanical migration.
**Suggestion:** Manually rewrite as a Django model (Prompt) persisted to the database, replacing list indices with primary keys.

## Files requiring manual review

These files were migrated but scored below the confidence threshold.
Review them carefully before merging.

### `client.py`
Confidence: 75%
Issues:
  - [warning] On invalid JSON, the Python version returns ONLY the error object {"error": "Invalid response from the server"} and never raises. The Go version returns the same sentinel payload but ALSO returns a non-nil error alongside it. A caller checking `if err != nil` would treat this as a failure rather than getting the graceful fallback map, which differs from the original's swallow-the-error semantics.
  - [info] In the original, create_prompt and update_prompt did NOT catch JSONDecodeError — they would propagate it. The Go doJSON uniformly applies the error-fallback map to all methods, so create/update now also return the sentinel map on decode failure. Behavior is slightly more lenient than the original for these two, but still returns an error alongside, so propagation semantics are largely preserved.
  - [info] The main() demo driver that executes the fixed sequence (create, create, get, update, get, delete, get) and prints 8 lines to stdout is not present in this migrated file. This was an explicit component/spec.

### `main.py`
Confidence: 78%
Issues:
  - [info] Validation order differs from the original. In the Python original, body validation (new_prompt presence) is checked BEFORE index validation. In the Go version, parseIndex is called first, so a request with a non-integer path segment returns 'Invalid prompt index' (200) even when new_prompt is missing. However, for the normal case where the index is a valid integer, index range validation still happens after body validation (store.Update is called after the new_prompt check), so this only diverges on non-integer paths — an edge case not reachable in Flask due to the <int:> converter.
