# Migration Notes

**Overall confidence:** 0%  
**Recommendation:** REVIEW RECOMMENDED

---

## What was migrated

- `client.py` → `internal/client.py.go` (10% confidence) ⚠️ needs review
- `main.py` → `internal/main.py.go` (79% confidence) ⚠️ needs review

## Components that could not be automatically migrated

These components require manual implementation. The migrated code contains
`MIGRATION_NOTE` comments at the relevant locations.

### `openai_api_key` in `main.py`
**Reason:** Hardcoded placeholder secret cannot be carried over as-is and must not be committed to source.
**Suggestion:** Load from an environment variable or secrets manager in the target framework.

### `openai.Completion.create (text-davinci-002)` in `main.py`
**Reason:** Uses the deprecated legacy OpenAI SDK API and a retired model; a direct 1:1 port will fail against the current OpenAI API.
**Suggestion:** Manually rewrite using the openai>=1.0 client (client.chat.completions.create) with a supported model.

### `self.prompts (in-memory list)` in `main.py`
**Reason:** Ephemeral, non-shared in-memory state does not translate to multi-process/persistent target architectures.
**Suggestion:** Replace with a database model (e.g. a Django Prompt model) or cache; use stable IDs instead of list indices.

## Files requiring manual review

These files were migrated but scored below the confidence threshold.
Review them carefully before merging.

### `client.py`
Confidence: 10%

### `main.py`
Confidence: 79%
Issues:
  - [info] In the Python source, the new_prompt validation (400) is performed BEFORE index bounds checking, but only after get_json() succeeds. In the Go migration, index parsing (which can return 404 for non-integer) happens before the JSON body parse/new_prompt validation. So for a non-integer index with a missing new_prompt, Python would return 404 (route doesn't match) — same as Go. This ordering difference is only observable for a valid-integer-but-out-of-range index combined with a missing new_prompt: Python returns 400 (new_prompt checked first), Go also returns 400 because body validation runs before UpdatePrompt bounds check. Behavior actually matches. No real divergence.
  - [info] Python uses request.get_json() then .get('prompt'); a missing body or non-JSON yields a falsy value and returns 400. Go returns 400 on JSON decode error too, which matches. Minor edge: Python get_json() with an empty body may raise/return None handled as falsy -> 400; Go decode error -> 400. Equivalent.
