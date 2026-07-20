# Migration Notes

**Overall confidence:** 0%  
**Recommendation:** REVIEW RECOMMENDED

---

## What was migrated

- `client.py` → `internal/client.go` (82% confidence) ⚠️ needs review
- `main.py` → `internal/` (82% confidence) ⚠️ needs review

## Components that could not be automatically migrated

These components require manual implementation. The migrated code contains
`MIGRATION_NOTE` comments at the relevant locations.

### `openai.Completion.create with engine 'text-davinci-002'` in `main.py`
**Reason:** The legacy OpenAI Completion API and the text-davinci-002 model are deprecated and no longer available in current OpenAI SDKs.
**Suggestion:** Manually rewrite to use the current OpenAI SDK (openai>=1.0) with client.chat.completions.create and a supported model (e.g., gpt-4o-mini), adjusting response parsing accordingly.

### `self.prompts in-memory list (ChatGPTBotAPI state)` in `main.py`
**Reason:** In-memory module-level state does not persist and is not safe across multiple workers/requests; it cannot be faithfully migrated to a stateless/multi-process target as-is.
**Suggestion:** Replace with persistent storage (database model, Redis, or session-scoped store) in the target stack.

## Files requiring manual review

These files were migrated but scored below the confidence threshold.
Review them carefully before merging.

### `client.py`
Confidence: 82%
Issues:
  - [info] The original create_prompt (and update_prompt) do NOT wrap JSON decoding in try/except and would propagate a JSON decode error. The migration's doJSON always returns errInvalidResponse() on JSON decode failure, so CreatePrompt/UpdatePrompt now gracefully handle non-JSON responses instead of propagating the error.

### `main.py`
Confidence: 82%
Issues:
  - [info] The original evaluates the 'new_prompt' presence check BEFORE parsing/checking the index. In the migration, parseIndex is called first; however since the route uses <int:prompt_index> in Flask and chi's {prompt_index} with strconv.Atoi, both effectively require a valid integer in the path. The ordering difference only manifests for non-integer paths, which Flask's <int:> converter would 404 anyway. For valid integer indices the body check still runs before the bounds check (UpdatePrompt), so the documented invariant is preserved. Minor ordering divergence only.
