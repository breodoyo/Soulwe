package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"Backend/internal/config"

	"github.com/gin-gonic/gin"
)

func TestHealthEndpoints(t *testing.T) {
	cfg := &config.Config{
		Env:         "test",
		GinMode:     "test",
		FrontendURL: "http://localhost:5173",
	}

	router := setupRouter(cfg)

	t.Run("Root /health returns 200 OK", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var res map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("Failed to decode JSON: %v", err)
		}

		if res["status"] != "ok" {
			t.Errorf("Expected status 'ok', got %v", res["status"])
		}

		if res["service"] != "soulwe-api" {
			t.Errorf("Expected service 'soulwe-api', got %v", res["service"])
		}
	})

	t.Run("API v1 /api/v1/health returns 200 OK", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/health", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		var res map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("Failed to decode JSON: %v", err)
		}

		if res["version"] != "v1" {
			t.Errorf("Expected version 'v1', got %v", res["version"])
		}
	})

	t.Run("CORS preflight OPTIONS returns 204 No Content", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodOptions, "/health", nil)
		req.Header.Set("Origin", "http://localhost:5173")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Fatalf("Expected status %d for OPTIONS, got %d", http.StatusNoContent, w.Code)
		}

		originHeader := w.Header().Get("Access-Control-Allow-Origin")
		if originHeader != "http://localhost:5173" {
			t.Errorf("Expected Access-Control-Allow-Origin to be 'http://localhost:5173', got '%s'", originHeader)
		}
	})

	t.Run("Panic recovery middleware catches panic and returns 500 JSON", func(t *testing.T) {
		// Register a test route that deliberately panics
		router.GET("/test-panic", func(c *gin.Context) {
			panic("something went wrong inside a handler")
		})

		req, _ := http.NewRequest(http.MethodGet, "/test-panic", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("Expected status 500 for panic, got %d", w.Code)
		}

		var res map[string]map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("Failed to decode JSON: %v", err)
		}

		if res["error"]["code"] != "INTERNAL_SERVER_ERROR" {
			t.Errorf("Expected error code 'INTERNAL_SERVER_ERROR', got %v", res["error"]["code"])
		}
	})
}
