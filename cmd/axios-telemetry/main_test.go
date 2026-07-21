package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTelemetryRequiresBearerToken(t *testing.T) {
	const token = "a-secure-telemetry-token-with-enough-length"
	handler := newHandler(token)

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if unauthorized.Code != http.StatusUnauthorized || unauthorized.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatalf("unauthorized response = %d, headers %v", unauthorized.Code, unauthorized.Header())
	}

	authorizedRequest := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	authorizedRequest.Header.Set("Authorization", "Bearer "+token)
	authorized := httptest.NewRecorder()
	handler.ServeHTTP(authorized, authorizedRequest)
	if authorized.Code != http.StatusOK {
		t.Fatalf("authorized response = %d, want 200", authorized.Code)
	}
}

func TestTelemetryRejectsWrongToken(t *testing.T) {
	handler := newHandler("correct-secure-telemetry-token-value")
	request := httptest.NewRequest(http.MethodGet, "/api/system/stats", nil)
	request.Header.Set("Authorization", "Bearer wrong-secure-telemetry-token-value")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("response = %d, want 401", response.Code)
	}
}
