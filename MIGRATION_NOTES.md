# Migration Notes

**Overall confidence:** 0%  
**Recommendation:** REVIEW RECOMMENDED

---

## What was migrated

- `client.py` → `internal/client.go` (78% confidence) ⚠️ needs review
- `main.py` → `internal/main/main.go` (52% confidence) ⚠️ needs review

## Components that could not be automatically migrated

These components require manual implementation. The migrated code contains
`MIGRATION_NOTE` comments at the relevant locations.

### `openai_api_key (hardcoded)` in `main.py`
**Reason:** Contains a placeholder/hardcoded secret that must not be carried into the target as-is.
**Suggestion:** Replace with an environment variable loaded via django-environ or os.environ in Django settings.

### `openai.Completion.create (text-davinci-002)` in `main.py`
**Reason:** Uses the deprecated legacy OpenAI Completions endpoint and SDK style that is no longer supported by current openai library versions.
**Suggestion:** Manually rewrite using the current OpenAI SDK (client.chat.completions.create with a chat model like gpt-4o-mini) during migration.

### `self.prompts in-memory list` in `main.py`
**Reason:** In-memory state does not persist and is not concurrency-safe; it cannot be mapped 1:1 to a stateless Django deployment.
**Suggestion:** Replace with a Django Prompt model and use the ORM for create/read/update/delete instead of list indices.

## Files requiring manual review

These files were migrated but scored below the confidence threshold.
Review them carefully before merging.

### `client.py`
Confidence: 78%

### `main.py`
Confidence: 52%
Issues:
  - [info] The Target Expert concedes this divergence is NOT fixed in the code as written. They provided the correct fix (adding :[0-9]+ regex constraints to the three routes) but explicitly state this is 'the only change I'm making' without confirming it has actually been applied to buildRouter(). The routes as shown still use unconstrained {prompt_index} patterns.
