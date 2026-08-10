```go
package internal

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// buildTestRouter creates a fresh router (and therefore a fresh promptStore)
// for each test so that tests are fully isolated.
func buildTestRouter() http.Handler {
	return buildRouter()
}

// jsonBody marshals v and returns a *bytes.Buffer suitable for use as an
// http.Request body.
func jsonBody(t *testing.T, v any) *bytes.Buffer {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return bytes.NewBuffer(b)
}

// doRequest fires a single request against the given handler and returns the
// recorded response.
func doRequest(t *testing.T, handler http.Handler, method, path string, body *bytes.Buffer) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequest(method, path, body)
	} else {
		req, err = http.NewRequest(method, path, nil)
	}
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

// decodeJSON decodes the recorder's body into a map[string]string.
func decodeJSON(t *testing.T, rr *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var m map[string]string
	err := json.NewDecoder(rr.Body).Decode(&m)
	require.NoError(t, err)
	return m
}

// seedPrompts POSTs one or more prompts into the handler so that subsequent
// tests can rely on a pre-populated store.
func seedPrompts(t *testing.T, handler http.Handler, prompts ...string) {
	t.Helper()
	for _, p := range prompts {
		rr := doRequest(t, handler, http.MethodPost, "/create", jsonBody(t, map[string]string{"prompt": p}))
		require.Equal(t, http.StatusCreated, rr.Code, "seed prompt failed for: %q", p)
	}
}

// ---------------------------------------------------------------------------
// POST /create
// ---------------------------------------------------------------------------

func TestHandleCreate(t *testing.T) {
	tests := []struct {
		name           string
		body           any
		wantStatus     int
		wantBodySubset map[string]string
	}{
		{
			name:           "valid prompt returns 201",
			body:           map[string]string{"prompt": "hello world"},
			wantStatus:     http.StatusCreated,
			wantBodySubset: map[string]string{"message": "Prompt created successfully"},
		},
		{
			name:           "empty prompt returns 400",
			body:           map[string]string{"prompt": ""},
			wantStatus:     http.StatusBadRequest,
			wantBodySubset: map[string]string{"error": "Prompt not provided"},
		},
		{
			name:           "missing prompt key returns 400",
			body:           map[string]string{},
			wantStatus:     http.StatusBadRequest,
			wantBodySubset: map[string]string{"error": "Prompt not provided"},
		},
		{
			name:           "null prompt value returns 400",
			body:           map[string]any{"prompt": nil},
			wantStatus:     http.StatusBadRequest,
			wantBodySubset: map[string]string{"error": "Prompt not provided"},
		},
		{
			name:           "invalid JSON body returns 400",
			body:           nil, // sentinel: we'll send raw bad JSON
			wantStatus:     http.StatusBadRequest,
			wantBodySubset: map[string]string{"error": "Prompt not provided"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler := buildTestRouter()

			var buf *bytes.Buffer
			if tc.body == nil {
				buf = bytes.NewBufferString("{bad json}")
			} else {
				buf = jsonBody(t, tc.body)
			}

			rr := doRequest(t, handler, http.MethodPost, "/create", buf)
			assert.Equal(t, tc.wantStatus, rr.Code)

			got := decodeJSON(t, rr)
			for k, v := range tc.wantBodySubset {
				assert.Equal(t, v, got[k])
			}
		})
	}
}

// TestHandleCreate_SideEffects verifies that a successful create actually
// appends to the store (observable via a subsequent delete returning "deleted").
func TestHandleCreate_SideEffects(t *testing.T) {
	handler := buildTestRouter()

	// Store is empty; index 0 should be invalid.
	rr := doRequest(t, handler, http.MethodDelete, "/delete/0", nil)
	assert.Equal(t, http.StatusOK, rr.Code)
	got := decodeJSON(t, rr)
	assert.Equal(t, "Invalid prompt index", got["message"])

	// Create a prompt.
	seedPrompts(t, handler, "first prompt")

	// Now index 0 should be valid.
	rr = doRequest(t, handler, http.MethodDelete, "/delete/0", nil)
	assert.Equal(t, http.StatusOK, rr.Code)
	got = decodeJSON(t, rr)
	assert.Equal(t, "Prompt deleted successfully", got["message"])
}

// TestHandleCreate_AppendOrder verifies that new prompts are always appended
// to the end of the list.
func TestHandleCreate_AppendOrder(t *testing.T) {
	handler := buildTestRouter()

	seedPrompts(t, handler, "first", "second", "third")

	// Update index 2 to verify it holds "third" (not "first").
	rr := doRequest(t, handler, http.MethodPut, "/update/2", jsonBody(t, map[string]string{"new_prompt": "replaced"}))
	assert.Equal(t, http.StatusOK, rr.Code)
	got := decodeJSON(t, rr)
	assert.Equal(t, "Prompt updated successfully", got["message"])

	// Update index 0 to verify it holds "first".
	rr = doRequest(t, handler, http.MethodPut, "/update/0", jsonBody(t, map[string]string{"new_prompt": "replaced-first"}))
	assert.Equal(t, http.StatusOK, rr.Code)
	got = decodeJSON(t, rr)
	assert.Equal(t, "Prompt updated successfully", got["message"])
}

// ---------------------------------------------------------------------------
// GET /get/{prompt_index}
// ---------------------------------------------------------------------------

// TestHandleGet_InvalidIndex covers the out-of-range and non-integer cases.
// We do NOT test valid-index behavior because it requires a real OpenAI API
// key; the lazy-validation path allows us to test the "key not configured"
// branch when the key is the placeholder.
func TestHandleGet(t *testing.T) {
	tests := []struct {
		name           string
		seedCount      int
		path           string
		wantStatus     int
		wantBodySubset map[string]string
	}{
		{
			name:           "negative index returns 200 with Invalid prompt index",
			seedCount:      1,
			path:           "/get/-1",
			wantStatus:     http.StatusOK,
			wantBodySubset: map[string]string{"response": "Invalid prompt index"},
		},
		{
			name:           "index equal to length returns 200 with Invalid prompt index",
			seedCount:      2,
			path:           "/get/2",
			wantStatus:     http.StatusOK,
			wantBodySubset: map[string]string{"response": "Invalid prompt index"},
		},
		{
			name:           "empty store index 0 returns 200 with Invalid prompt index",
			seedCount:      0,
			path:           "/get/0",
			wantStatus:     http.StatusOK,
			wantBodySubset: map[string]string{"response": "Invalid prompt index"},
		},
		{
			name:      "non-integer path segment returns 404",
			seedCount: 1,
			path:      "/get/abc",
			wantStatus: http.StatusNotFound,
		},
		{
			name:      "non-integer path segment (float) returns 404",
			seedCount: 1,
			path:      "/get/1.5",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler := buildTestRouter()
			for i := 0; i < tc.seedCount; i++ {
				seedPrompts(t, handler, "prompt")
			}

			rr := doRequest(t, handler, http.MethodGet, tc.path, nil)
			assert.Equal(t, tc.wantStatus, rr.Code)

			if len(tc.wantBodySubset) > 0 {
				got := decodeJSON(t, rr)
				for k, v := range tc.wantBodySubset {
					assert.Equal(t, v, got[k])
				}
			}
		})
	}
}

// TestHandleGet_ValidIndex_PlaceholderKey verifies that when the store has a
// valid prompt at the requested index but the API key is the placeholder, the
// handler returns HTTP 500 with an appropriate error (lazy-key-validation
// behavior).
func TestHandleGet_ValidIndex_PlaceholderKey(t *testing.T) {
	// Ensure OPENAI_API_KEY is unset so newPromptStore falls back to the
	// placeholder.
	t.Setenv("OPENAI_API_KEY", "")

	handler := buildTestRouter()
	seedPrompts(t, handler, "tell me a joke")

	rr := doRequest(t, handler, http.MethodGet, "/get/0", nil)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	got := decodeJSON(t, rr)
	assert.Equal(t, "OpenAI API key not configured", got["error"])
}

// TestHandleGet_DoesNotModifyStore verifies that GET /get never alters the
// list (observable by checking that delete still succeeds after a failed GET).
func TestHandleGet_DoesNotModifyStore(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	handler := buildTestRouter()
	seedPrompts(t, handler, "prompt one", "prompt two")

	// Fire a GET that will fail at the API-key check.
	rr := doRequest(t, handler, http.MethodGet, "/get/0", nil)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	// Both prompts should still exist.
	rr = doRequest(t, handler, http.MethodDelete, "/delete/1", nil)
	assert.Equal(t, http.StatusOK, rr.Code)
	got := decodeJSON(t, rr)
	assert.Equal(t, "Prompt deleted successfully", got["message"])

	rr = doRequest(t, handler, http.MethodDelete, "/delete/0", nil)
	assert.Equal(t, http.StatusOK, rr.Code)
	got = decodeJSON(t, rr)
	assert.Equal(t, "Prompt deleted successfully", got["message"])
}

// ---------------------------------------------------------------------------
// DELETE /delete/{prompt_index}
// ---------------------------------------------------------------------------

func TestHandleDelete(t *testing.T) {
	tests := []struct {
		name           string
		seeds          []string
		path           string
		wantStatus     int
		wantBodySubset map[string]string
		wantListLen    int // -1 means "don't check"
	}{
		{
			name:           "valid index 0 deletes prompt",
			seeds:          []string{"alpha", "beta"},
			path:           "/delete/0",
			wantStatus:     http.StatusOK,
			wantBodySubset: map[string]string{"message": "Prompt deleted successfully"},
			wantListLen:    1,
		},
		{
			name:           "valid last index deletes prompt",
			seeds:          []string{"alpha", "beta", "gamma"},
			path:           "/delete/2",
			wantStatus:     http.StatusOK,
			wantBodySubset: map[string]string{"message": "Prompt deleted successfully"},
			wantListLen:    2,
		},
		{
			name:           "negative index returns 200 Invalid prompt index",
			seeds:          []string{"alpha"},
			path:           "/delete/-1",
			wantStatus:     http.StatusOK,
			wantBodySubset: map[string]string{"message": "Invalid prompt index"},
			wantListLen:    1,
		},
		{
			name:           "out-of-range index returns 200 Invalid prompt index",
			seeds:          []string{"alpha"},
			path:           "/delete/5",
			wantStatus:     http.StatusOK,
			wantBodySubset: map[string]string{"message": "Invalid prompt index"},
			wantListLen:    1,
		},
		{
			name:           "empty store returns 200 Invalid prompt index",
			seeds:          []string{},
			path:           "/delete/0",
			wantStatus:     http.StatusOK,
			wantBodySubset: map[string]string{"message": "Invalid prompt index"},
			wantListLen:    0,
		},
		{
			name:       "non-integer path returns 404",
			seeds:      []string{"alpha"},
			path:       "/delete/foo",
			wantStatus: http.StatusNotFound,
			wantListLen: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler := buildTestRouter()
			seedPrompts(t, handler, tc.seeds...)

			rr := doRequest(t, handler, http.MethodDelete, tc.path, nil)
			assert.Equal(t, tc.wantStatus, rr.Code)

			if len(tc.wantBodySubset) > 0 {
				got := decodeJSON(t, rr)
				for k, v := range tc.wantBodySubset {
					assert.Equal(t, v, got[k])
				}
			}

			// Verify list length by probing with delete at expected boundary.
			if tc.wantListLen >= 0 {
				// An index equal to wantListLen should be out of range.
				probe := doRequest(t, handler, http.MethodDelete, "/delete/"+itoa(tc.wantListLen), nil)
				probeGot := decodeJSON(t, probe)
				assert.Equal(t, "Invalid prompt index", probeGot["message"],
					"expected list length %d but index %d was valid", tc.wantListLen, tc.wantListLen)
			}
		})
	}
}

// TestHandleDelete_ShiftsElements verifies that after deleting index 0 the
// former element at index 1 is now reachable at index 0 (observable via
// update).
func TestHandleDelete_ShiftsElements(t *testing.T) {
	handler := buildTestRouter()
	seedPrompts(t, handler, "first", "second", "third")

	// Delete index 0 ("first").
	rr := doRequest(t, handler, http.MethodDelete, "/delete/0", nil)
	assert