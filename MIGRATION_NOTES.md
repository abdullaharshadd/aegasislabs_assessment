# Migration Notes

**Overall confidence:** 0%  
**Recommendation:** REVIEW RECOMMENDED

---

## What was migrated

- `client.py` → `internal/client.go` (78% confidence) ⚠️ needs review
- `main.py` → `internal/main.go` (78% confidence) ⚠️ needs review

## Components that could not be automatically migrated

These components require manual implementation. The migrated code contains
`MIGRATION_NOTE` comments at the relevant locations.

### `ChatGPTBotAPI.get_response (OpenAI Completion API call)` in `main.py`
**Reason:** Uses the deprecated openai.Completion.create endpoint and legacy engine 'text-davinci-002' which is discontinued; a direct 1:1 translation would call a dead API.
**Suggestion:** Manually rewrite to use the current OpenAI SDK (client.chat.completions.create) with a supported model like gpt-4o-mini, adapting the response parsing accordingly.

## Files requiring manual review

These files were migrated but scored below the confidence threshold.
Review them carefully before merging.

### `client.py`
Confidence: 78%

### `main.py`
Confidence: 78%
Issues:
  - [info] The Target Expert correctly analyzes the divergence and proposes an accurate fix (adding `:[0-9]+` regex constraint to route patterns), but the code has NOT been changed — the fix is proposed, not applied. The divergence therefore remains in the current code.
