# Migration Notes

**Overall confidence:** 0%  
**Recommendation:** REVIEW RECOMMENDED

---

## What was migrated

- `client.py` → `internal/client.go` (78% confidence) ⚠️ needs review
- `main.py` → `internal/main.go` (23% confidence) ⚠️ needs review

## Components that could not be automatically migrated

These components require manual implementation. The migrated code contains
`MIGRATION_NOTE` comments at the relevant locations.

### `openai.Completion.create with engine 'text-davinci-002'` in `main.py`
**Reason:** The legacy Completions endpoint and the text-davinci-002 model have been deprecated/retired by OpenAI and will not function.
**Suggestion:** Manually rewrite to use the current OpenAI Chat Completions API (client.chat.completions.create with a supported model like gpt-4o-mini) or the Responses API.

### `self.prompts in-memory list` in `main.py`
**Reason:** In-memory storage does not survive restarts and is not shared across processes/workers; positional indices are not a durable identifier model.
**Suggestion:** Introduce persistent storage (database model/table) with stable primary keys and update endpoints to use IDs instead of list indices.

## Files requiring manual review

These files were migrated but scored below the confidence threshold.
Review them carefully before merging.

### `client.py`
Confidence: 78%

### `main.py`
Confidence: 23%
Issues:
  - [critical] The Target Expert again only *proposes* the patch rather than confirming it is applied and verified. The code snippets are correct in principle, but the response is explicitly framed as 'am now applying the actual change' followed by proposed diffs, with an unresolved caveat that the exact Client.GetResponse signature must be confirmed against internal/client.go. Until the signature is verified and the change is actually landed (including the main.go wiring), the endpoint remains behaviorally broken (echoes prompt instead of calling OpenAI).
  - [info] The response is truncated mid-sentence and never actually removes/updates the stale MIGRATION_NOTE inside GetResponse or the superseded note claiming buildRouter must keep its no-arg signature. This is cleanup-only (non-behavioral) but remains outstanding.
