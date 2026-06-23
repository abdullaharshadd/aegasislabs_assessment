# Migration Notes

**Overall confidence:** 0%  
**Recommendation:** REVIEW RECOMMENDED

---

## What was migrated

- `client.py` → `internal/client.py.go` (20% confidence) ⚠️ needs review
- `main.py` → `internal/main.py.go` (25% confidence) ⚠️ needs review

## Components that could not be automatically migrated

These components require manual implementation. The migrated code contains
`MIGRATION_NOTE` comments at the relevant locations.

### `self.prompts in-memory list (ChatGPTBotAPI state)` in `main.py`
**Reason:** Module-level / instance-level in-memory storage does not survive across requests, processes, or workers in a production Django deployment and has no Django equivalent that preserves the same semantics.
**Suggestion:** Manual rewrite: replace with a Django model (e.g., Prompt model) backed by the database, or a cache backend if ephemeral storage is acceptable. Switch from index-based access to pk-based lookups.

### `openai.Completion.create with engine='text-davinci-002'` in `main.py`
**Reason:** This uses the deprecated OpenAI Completions endpoint and a retired model; it will not function with current openai library versions.
**Suggestion:** Manual rewrite using the modern OpenAI client (openai.OpenAI().chat.completions.create with a current model like gpt-4o-mini). This is independent of the web framework migration.

### `hardcoded openai_api_key` in `main.py`
**Reason:** Secret hardcoded in source; not a portable or secure config pattern.
**Suggestion:** Move to Django settings loaded from environment variables (django-environ / os.environ); never commit the key.

## Files requiring manual review

These files were migrated but scored below the confidence threshold.
Review them carefully before merging.

### `client.py`
Confidence: 20%

### `main.py`
Confidence: 25%
Issues:
  - [critical] The Completer (openAICompleter.Complete) is an unimplemented stub that always returns an error. For a valid in-range index, GetResponse returns a wrapped error, causing handleGetResponse to respond with HTTP 500 instead of the expected HTTP 200 with the generated text. The core OpenAI completion functionality is broken/missing.
  - [critical] Same root cause: the external completion service is never actually invoked because Complete is a stub. The valid-index branch cannot produce a real completion.
