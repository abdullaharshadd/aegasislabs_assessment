# Migration Notes

**Overall confidence:** 0%  
**Recommendation:** REVIEW RECOMMENDED

---

## What was migrated

- `client.py` → `internal/client.py.go` (20% confidence) ⚠️ needs review
- `main.py` → `internal/main.py.go` (80% confidence) ⚠️ needs review

## Components that could not be automatically migrated

These components require manual implementation. The migrated code contains
`MIGRATION_NOTE` comments at the relevant locations.

### `openai.Completion.create with engine 'text-davinci-002'` in `main.py`
**Reason:** This is a deprecated OpenAI SDK API and model that may not exist in current openai library versions; not a direct 1:1 code translation but an API-surface change.
**Suggestion:** Manually rewrite to the current OpenAI SDK (openai>=1.0 client.chat.completions.create) with a supported model (e.g. gpt-4o-mini / gpt-3.5-turbo). Keep the wrapper service pattern but update the call signature and response parsing (response.choices[0].message.content).

### `In-memory self.prompts list as persistence` in `main.py`
**Reason:** Not automatically migrable to a Django-idiomatic persistent design; a list index-based store cannot be blindly translated to a Django Model without semantic changes.
**Suggestion:** Manually rewrite as a Django Model (e.g. Prompt with text field + auto PK) or keep as an in-memory/cache store if statelessness is intentional; update endpoints to use PK lookups instead of list indices.

### `app.run(debug=True) / Flask app bootstrap` in `main.py`
**Reason:** Flask-specific server bootstrap has no code equivalent in Django, which uses manage.py + WSGI/ASGI.
**Suggestion:** Drop this block; rely on Django's standard project scaffolding (settings.py, urls.py, wsgi.py/asgi.py) generated separately.

## Files requiring manual review

These files were migrated but scored below the confidence threshold.
Review them carefully before merging.

### `client.py`
Confidence: 20%

### `main.py`
Confidence: 80%
Issues:
  - [warning] The CompletionClient is only a stub that always returns an error. For a valid index, the original returns HTTP 200 with the completion text; the migration will return HTTP 500 because stubCompletionClient.Complete always errors. This is an intentional/documented migration gap (no Go OpenAI SDK wired in), so the valid-index success path is not functional until a real client is added.
  - [info] For a non-numeric path segment, parseIndex returns HTTP 400. The original Flask route uses <int:prompt_index> which would 404 on a non-integer. This is a minor routing-edge-case difference unlikely to affect normal usage.
