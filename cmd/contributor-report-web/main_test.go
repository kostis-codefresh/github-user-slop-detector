package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandler(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantCode int
		wantBody []string
	}{
		{
			name:     "Serves the home page",
			path:     "/",
			wantCode: http.StatusOK,
			wantBody: []string{`name="username"`, `<button type="button">Analyze</button>`, `<button type="button" class="login">`},
		},
		{
			name:     "Unknown path is not found",
			path:     "/nope",
			wantCode: http.StatusNotFound,
		},
	}

	h := newHandler()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))

			assert.Equal(t, tt.wantCode, rec.Code, "status code for %s", tt.path)
			for _, want := range tt.wantBody {
				assert.Contains(t, rec.Body.String(), want, "body of %s", tt.path)
			}
		})
	}
}
