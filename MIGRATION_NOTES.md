# Migration Notes

**Overall confidence:** 0%  
**Recommendation:** REVIEW RECOMMENDED

---

## What was migrated

- `client.py` → `internal/client.go` (38% confidence) ⚠️ needs review
- `main.py` → `internal/main.go` (78% confidence) ⚠️ needs review

## Components that could not be automatically migrated

These components require manual implementation. The migrated code contains
`MIGRATION_NOTE` comments at the relevant locations.

### `openai.Completion.create (in get_response)` in `main.py`
**Reason:** Uses the deprecated legacy OpenAI SDK completion API and engine 'text-davinci-002', which is removed/unsupported in current OpenAI SDK versions.
**Suggestion:** Manually rewrite to the modern OpenAI client (chat.completions.create with a supported model like gpt-4o-mini/gpt-3.5-turbo), or pin the legacy openai SDK version if the exact behavior must be preserved.

### `In-memory prompts list (self.prompts)` in `main.py`
**Reason:** Volatile, single-process state that does not survive restarts and is not shared across workers; cannot be faithfully auto-migrated to a scalable/persistent target without behavioral change.
**Suggestion:** Replace with a persistent store (database table/model) using stable IDs, or explicitly document that behavior remains ephemeral if single-process is acceptable.

## Files requiring manual review

These files were migrated but scored below the confidence threshold.
Review them carefully before merging.

### `client.py`
Confidence: 38%
Issues:
  - [info] The corrected doJSON/decodeResponse code is truncated mid-string ('Inval...') and the call sites for CreatePrompt/UpdatePrompt/GetResponse/DeletePrompt are not shown passing the correct raiseOnBadJSON value. The fix is described accurately but cannot be verified as applied.
  - [info] The Target Expert's response does not address the truncated/incomplete main() snippet from the original review at all.

### `main.py`
Confidence: 78%
