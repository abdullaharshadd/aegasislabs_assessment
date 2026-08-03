# Migration Notes

**Overall confidence:** 0%  
**Recommendation:** REVIEW RECOMMENDED

---

## What was migrated

- `client.py` → `internal/client.go` (79% confidence) ⚠️ needs review
- `main.py` → `internal/main.go` (42% confidence) ⚠️ needs review

## Components that could not be automatically migrated

These components require manual implementation. The migrated code contains
`MIGRATION_NOTE` comments at the relevant locations.

### `openai.Completion.create with engine text-davinci-002` in `main.py`
**Reason:** Uses a deprecated OpenAI API endpoint and model that no longer exist in current OpenAI SDKs, so a direct call-for-call port will fail at runtime.
**Suggestion:** Manually rewrite using the current OpenAI Python SDK (client.chat.completions.create) with a supported model and adapt the response parsing (message.content instead of choices[0].text).

### `In-memory self.prompts list state` in `main.py`
**Reason:** Ephemeral, non-persistent, index-based storage does not map cleanly to a stateless, multi-process Django deployment.
**Suggestion:** Replace with a persistent Django model and use primary keys instead of list indices; requires manual data-model design.

## Files requiring manual review

These files were migrated but scored below the confidence threshold.
Review them carefully before merging.

### `client.py`
Confidence: 79%

### `main.py`
Confidence: 42%
Issues:
  - [warning] The Target Expert concedes this is a real divergence and that the fix was NOT applied in the submitted code. Non-integer path segments (e.g. /get/abc) return 200 'Invalid prompt index' instead of Flask's 404. The proposed regex fix ([0-9]+ route constraint) is described but explicitly not landed.
