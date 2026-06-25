# Migration Notes

**Overall confidence:** 0%  
**Recommendation:** REVIEW RECOMMENDED

---

## What was migrated

- `client.py` → `src/client.ts` (88% confidence)
- `main.py` → `src/main.ts` (86% confidence)

## Components that could not be automatically migrated

These components require manual implementation. The migrated code contains
`MIGRATION_NOTE` comments at the relevant locations.

### `openai.Completion.create with engine 'text-davinci-002'` in `main.py`
**Reason:** Uses the legacy OpenAI Completions API and a deprecated/retired model that is no longer available, so a direct 1:1 port would fail at runtime.
**Suggestion:** Manually rewrite to the current OpenAI SDK (openai>=1.0) using client.chat.completions.create with a supported chat model (e.g., gpt-4o-mini/gpt-3.5-turbo); this is an API-contract change, not a framework migration.

### `ChatGPTBotAPI.prompts (in-memory list state)` in `main.py`
**Reason:** In-memory storage is not durable and is incompatible with Django's typical multi-worker, request-stateless model; index-based addressing has no clean DB equivalent.
**Suggestion:** Manually decide on persistence: introduce a Django Prompt model + migrations and switch to PK-based lookups, or explicitly document and keep in-memory behavior if statelessness is acceptable.
