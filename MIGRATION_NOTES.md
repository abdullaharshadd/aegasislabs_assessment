# Migration Notes

**Overall confidence:** 0%  
**Recommendation:** REVIEW RECOMMENDED

---

## What was migrated

- `client.py` → `internal/client.go` (49% confidence) ⚠️ needs review
- `main.py` → `internal/main.go` (89% confidence)

## Components that could not be automatically migrated

These components require manual implementation. The migrated code contains
`MIGRATION_NOTE` comments at the relevant locations.

### `openai.Completion.create with engine='text-davinci-002'` in `main.py`
**Reason:** Uses the deprecated OpenAI Completions endpoint and a retired model that is no longer available in the current OpenAI SDK/API.
**Suggestion:** Manually rewrite to the current OpenAI SDK (openai>=1.0) using chat.completions.create with a supported model (e.g., gpt-4o-mini / gpt-3.5-turbo), or keep external LLM call in a dedicated service layer.

### `ChatGPTBotAPI in-memory prompts list` in `main.py`
**Reason:** In-memory global state does not translate to a stateless/multi-worker web deployment and is not persistent; automatic migration would preserve broken semantics.
**Suggestion:** Manually replace with a persistent store — a Django model + ORM (Prompt model) or a cache/DB layer — during migration.

## Files requiring manual review

These files were migrated but scored below the confidence threshold.
Review them carefully before merging.

### `client.py`
Confidence: 49%
Issues:
  - [warning] The Target Expert concedes the divergence is real and provides a correct fix (strict flag threaded through do), but the fix is again presented as a proposed code block rather than confirmed as applied to the actual migrated file. The response is also truncated mid-table, so there is no verification that the four call sites (create/update/get/delete) were updated to route through doWithBody/doBodyless. Until applied and verified, POST/PUT still return the lenient fallback instead of propagating the decode error.
