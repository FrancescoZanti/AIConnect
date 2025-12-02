package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fzanti/aiconnect/internal/config"
	"github.com/sirupsen/logrus"
)

func TestIsPublicPath(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		publicPaths []string
		expected    bool
	}{
		{
			name:        "exact match",
			path:        "/health",
			publicPaths: []string{"/health"},
			expected:    true,
		},
		{
			name:        "exact match - no match",
			path:        "/health",
			publicPaths: []string{"/status"},
			expected:    false,
		},
		{
			name:        "wildcard match",
			path:        "/ollama/api/generate",
			publicPaths: []string{"/ollama/*"},
			expected:    true,
		},
		{
			name:        "wildcard match - root",
			path:        "/ollama/",
			publicPaths: []string{"/ollama/*"},
			expected:    true,
		},
		{
			name:        "wildcard match - no match",
			path:        "/openai/api/generate",
			publicPaths: []string{"/ollama/*"},
			expected:    false,
		},
		{
			name:        "prefix with trailing slash",
			path:        "/vllm/models",
			publicPaths: []string{"/vllm/"},
			expected:    true,
		},
		{
			name:        "prefix with trailing slash - no match",
			path:        "/openai/models",
			publicPaths: []string{"/vllm/"},
			expected:    false,
		},
		{
			name:        "multiple public paths - first match",
			path:        "/health",
			publicPaths: []string{"/health", "/ollama/*", "/status"},
			expected:    true,
		},
		{
			name:        "multiple public paths - second match",
			path:        "/ollama/api",
			publicPaths: []string{"/health", "/ollama/*", "/status"},
			expected:    true,
		},
		{
			name:        "empty public paths",
			path:        "/health",
			publicPaths: []string{},
			expected:    false,
		},
		{
			name:        "nil public paths",
			path:        "/health",
			publicPaths: nil,
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPublicPath(tt.path, tt.publicPaths)
			if result != tt.expected {
				t.Errorf("isPublicPath(%q, %v) = %v, want %v", tt.path, tt.publicPaths, result, tt.expected)
			}
		})
	}
}

func TestLDAPAuthMiddleware_ADDisabled(t *testing.T) {
	cfg := &config.Config{}
	cfg.AD.Enabled = false

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	// Create a test handler that returns 200 OK
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with auth middleware
	handler := LDAPAuthMiddleware(cfg, log)(testHandler)

	// Create a request without authentication
	req := httptest.NewRequest("GET", "/ollama/api/generate", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Should pass through without authentication
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}
}

func TestLDAPAuthMiddleware_PublicPath(t *testing.T) {
	cfg := &config.Config{}
	cfg.AD.Enabled = true
	cfg.AD.PublicPaths = []string{"/ollama/*", "/health"}

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	// Create a test handler that returns 200 OK
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with auth middleware
	handler := LDAPAuthMiddleware(cfg, log)(testHandler)

	tests := []struct {
		name         string
		path         string
		expectedCode int
	}{
		{
			name:         "public path with wildcard",
			path:         "/ollama/api/generate",
			expectedCode: http.StatusOK,
		},
		{
			name:         "public path exact match",
			path:         "/health",
			expectedCode: http.StatusOK,
		},
		{
			name:         "protected path without auth",
			path:         "/openai/api/generate",
			expectedCode: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedCode {
				t.Errorf("Expected status %d, got %d for path %s", tt.expectedCode, rr.Code, tt.path)
			}
		})
	}
}

func TestLDAPAuthMiddleware_ADEnabled_NoAuth(t *testing.T) {
	cfg := &config.Config{}
	cfg.AD.Enabled = true
	cfg.AD.PublicPaths = []string{} // No public paths

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	// Create a test handler that returns 200 OK
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with auth middleware
	handler := LDAPAuthMiddleware(cfg, log)(testHandler)

	// Create a request without authentication
	req := httptest.NewRequest("GET", "/ollama/api/generate", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Should return Unauthorized because AD is enabled and no credentials provided
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", rr.Code)
	}
}

func TestLDAPAuthMiddleware_InvalidAuthHeader(t *testing.T) {
	cfg := &config.Config{}
	cfg.AD.Enabled = true
	cfg.AD.PublicPaths = []string{}

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	// Create a test handler that returns 200 OK
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with auth middleware
	handler := LDAPAuthMiddleware(cfg, log)(testHandler)

	tests := []struct {
		name         string
		authHeader   string
		expectedCode int
	}{
		{
			name:         "empty auth header",
			authHeader:   "",
			expectedCode: http.StatusUnauthorized,
		},
		{
			name:         "bearer auth instead of basic",
			authHeader:   "Bearer sometoken",
			expectedCode: http.StatusUnauthorized,
		},
		{
			name:         "invalid basic auth format",
			authHeader:   "Basic !!!notbase64!!!",
			expectedCode: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/ollama/api/generate", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedCode {
				t.Errorf("Expected status %d, got %d", tt.expectedCode, rr.Code)
			}
		})
	}
}
