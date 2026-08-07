# Migration Notes

**Overall confidence:** 0%  
**Recommendation:** REVIEW RECOMMENDED

---

## What was migrated

- `client.py` → `internal/client.go` (49% confidence) ⚠️ needs review
- `main.py` → `internal/main.go` (78% confidence) ⚠️ needs review

## Components that could not be automatically migrated

These components require manual implementation. The migrated code contains
`MIGRATION_NOTE` comments at the relevant locations.

### `openai.Completion.create call in get_response` in `main.py`
**Reason:** Uses the deprecated legacy OpenAI Completion endpoint and engine 'text-davinci-002', which is retired and not directly portable to newer SDKs.
**Suggestion:** Manually rewrite using the current OpenAI SDK chat completions API (e.g. client.chat.completions.create with a supported model like gpt-4o-mini or gpt-3.5-turbo).

### `self.prompts in-memory list` in `main.py`
**Reason:** In-memory, non-persistent state does not translate to a stateless/multi-worker production backend and loses data on restart.
**Suggestion:** Replace with a persistent model/table (e.g. a database entity with an auto ID) and use ID-based lookups instead of list indices.

## Files requiring manual review

These files were migrated but scored below the confidence threshold.
Review them carefully before merging.

### `client.py`
Confidence: 49%
Issues:
  - [warning] The Target Expert correctly diagnosed the divergence and provided a corrected doJSON with a fallbackOnDecodeError flag and proper caller wiring. However, the response code is truncated (UpdatePrompt and DeletePrompt callers are cut off mid-line), so it cannot be confirmed that the fix was fully and correctly applied to all four callers in the actual migration file.

### `main.py`
Confidence: 78%
