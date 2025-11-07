package handler

import (
	"github.com/heavydash/my-url-shortenergo/internal/repository"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name         string
		method       string
		path         string
		body         string
		wantStatus   int
		wantBody     string
		wantLocation string
	}{

		{name: "valid post",
			method:       "POST",
			path:         "/",
			body:         "https://example.com",
			wantStatus:   201,
			wantBody:     "http://localhost:8080/",
			wantLocation: "",
		},
		{name: "invalid post",
			method:       "POST",
			path:         "/",
			body:         "invalid",
			wantStatus:   400,
			wantBody:     "",
			wantLocation: "",
		},
		{name: "valid get",
			method:       "GET",
			path:         "/00000001",
			body:         "",
			wantStatus:   307,
			wantBody:     "",
			wantLocation: "https://example.com",
		},
		{name: "invalid get",
			method:       "GET",
			path:         "/invalid",
			body:         "",
			wantStatus:   400,
			wantBody:     "",
			wantLocation: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			repo := repository.NewMemoryRepository()
			if tt.name == "valid get" {
				repo.InitializeForTest("00000001", "https://example.com")
			}
			h := NewHandler(repo)

			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))

			w := httptest.NewRecorder()

			h.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.wantBody != "" {
				assert.Contains(t, w.Body.String(), tt.wantBody)
			}
			if tt.wantLocation != "" {
				assert.Equal(t, tt.wantLocation, w.Header().Get("Location"))
			}
		})
	}
}
