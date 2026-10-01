package httpapi_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Aleksashka-i/lunar-backend-engineer-challenge/internal/httpapi"
	"github.com/Aleksashka-i/lunar-backend-engineer-challenge/internal/rocket"
	"github.com/Aleksashka-i/lunar-backend-engineer-challenge/internal/storage"
)

const launched = `{
	"metadata": {
		"channel": "193270a9-c9cf-404a-8f83-838e71d9ae67",
		"messageNumber": 1,
		"messageTime": "2022-02-02T19:39:05.86337+01:00",
		"messageType": "RocketLaunched"
	},
	"message": {"type": "Falcon-9", "launchSpeed": 500, "mission": "ARTEMIS"}
}`

func newServer() http.Handler {
	return httpapi.New(rocket.NewService(storage.NewMemory()), slog.New(slog.DiscardHandler))
}

func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func TestPostMessageAndGetRocket(t *testing.T) {
	h := newServer()
	rec := do(h, http.MethodPost, "/messages", launched)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())

	rec = do(h, http.MethodGet, "/rockets/193270a9-c9cf-404a-8f83-838e71d9ae67", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var s rocket.State
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &s))
	assert.Equal(t, rocket.StatusLaunched, s.Status)
	assert.Equal(t, "Falcon-9", s.Type)
	assert.Equal(t, 500, s.Speed)
	assert.Equal(t, "ARTEMIS", s.Mission)
}

// TestPostMessageEmptyResponse guards connection reuse: the rockets test client
// never reads response bodies, so a body makes it open a connection per message.
func TestPostMessageEmptyResponse(t *testing.T) {
	h := newServer()
	for range 2 { // new, then duplicate
		rec := do(h, http.MethodPost, "/messages", launched)
		assert.Equal(t, http.StatusAccepted, rec.Code)
		assert.Empty(t, rec.Body.String())
	}
}

func TestListRockets(t *testing.T) {
	h := newServer()
	do(h, http.MethodPost, "/messages", launched)

	rec := do(h, http.MethodGet, "/rockets?sort=speed&order=desc", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var states []rocket.State
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &states))
	assert.Len(t, states, 1)
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name, method, path, body string
		want                     int
	}{
		{"malformed JSON", http.MethodPost, "/messages", "{", http.StatusBadRequest},
		{"unknown message type", http.MethodPost, "/messages", `{"metadata":{"channel":"c","messageNumber":1,"messageType":"Nope"}}`, http.StatusBadRequest},
		{"unknown rocket", http.MethodGet, "/rockets/unknown", "", http.StatusNotFound},
		{"unknown sort field", http.MethodGet, "/rockets?sort=color", "", http.StatusBadRequest},
		{"unknown sort order", http.MethodGet, "/rockets?order=up", "", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(newServer(), tt.method, tt.path, tt.body)
			assert.Equal(t, tt.want, rec.Code)
		})
	}
}
