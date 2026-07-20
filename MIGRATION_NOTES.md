# Migration Notes

**Overall confidence:** 0%  
**Recommendation:** REVIEW RECOMMENDED

---

## What was migrated

- `client.py` → `internal/client.go` (78% confidence) ⚠️ needs review
- `main.py` → `internal/main.go` (50% confidence) ⚠️ needs review

## Components that could not be automatically migrated

These components require manual implementation. The migrated code contains
`MIGRATION_NOTE` comments at the relevant locations.

### `openai.Completion.create with engine text-davinci-002` in `main.py`
**Reason:** The legacy Completions endpoint and text-davinci-002 model are deprecated/removed by OpenAI and will not function as-is.
**Suggestion:** Manually rewrite using the current OpenAI SDK (client.chat.completions.create) with a supported model such as gpt-4o-mini or gpt-3.5-turbo.

### `self.prompts in-memory list` in `main.py`
**Reason:** In-memory list state does not translate to a stateless multi-worker web deployment and is not idiomatic in Django.
**Suggestion:** Replace with a persistent Django model (Prompt) backed by the database, and use querysets for CRUD.

## Files requiring manual review

These files were migrated but scored below the confidence threshold.
Review them carefully before merging.

### `client.py`
Confidence: 78%
Issues:
  - [info] The main() demonstration/entry-point function is not migrated. There is no equivalent function that exercises the CRUD operations in sequence and prints results, nor a package main entrypoint.

### `main.py`
Confidence: 50%
Issues:
  - [critical] The OpenAI completion call is not actually implemented — Complete() returns 'openai completion not implemented' error for valid indices with a configured key. In normal usage a valid prompt index will never return generated text; it will return HTTP 500.
  - [info] Validation order differs from the source. Original checks new_prompt (400) BEFORE index validity, but the migration parses/validates the index first (returning 400 on non-integer). For the normal integer-path case this is equivalent, but a non-integer index combined with missing new_prompt would behave differently. This is a boundary-order edge case.
