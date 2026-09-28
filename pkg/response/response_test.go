package response_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dev-nathaniel/Adroit-platform/pkg/response"
)

func TestJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	payload := map[string]string{"name": "adroit-app"}

	response.JSON(rec, http.StatusOK, payload)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json; charset=utf-8" {
		t.Errorf("expected Content-Type application/json; charset=utf-8, got %s", contentType)
	}

	var res response.APIResponse[map[string]string]
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !res.Success {
		t.Errorf("expected success to be true, got false")
	}

	if res.Data["name"] != "adroit-app" {
		t.Errorf("expected data.name 'adroit-app', got '%s'", res.Data["name"])
	}
}

func TestJSONWithMeta(t *testing.T) {
	rec := httptest.NewRecorder()
	items := []string{"svc_1", "svc_2"}
	meta := &response.Pagination{
		Page:       1,
		PerPage:    10,
		TotalItems: 2,
		TotalPages: 1,
	}

	response.JSONWithMeta(rec, http.StatusOK, items, meta)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var res response.APIResponse[[]string]
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !res.Success || len(res.Data) != 2 {
		t.Errorf("expected 2 items and success=true")
	}

	if res.Meta == nil || res.Meta.TotalItems != 2 {
		t.Errorf("expected pagination total_items 2")
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name         string
		call         func(w http.ResponseWriter)
		expectedCode int
		expectedErr  string
	}{
		{
			name:         "BadRequest",
			call:         func(w http.ResponseWriter) { response.BadRequest(w, "invalid field", nil) },
			expectedCode: http.StatusBadRequest,
			expectedErr:  response.ErrCodeInvalidRequest,
		},
		{
			name:         "Unauthorized",
			call:         func(w http.ResponseWriter) { response.Unauthorized(w, "") },
			expectedCode: http.StatusUnauthorized,
			expectedErr:  response.ErrCodeUnauthorized,
		},
		{
			name:         "Forbidden",
			call:         func(w http.ResponseWriter) { response.Forbidden(w, "") },
			expectedCode: http.StatusForbidden,
			expectedErr:  response.ErrCodeForbidden,
		},
		{
			name:         "NotFound",
			call:         func(w http.ResponseWriter) { response.NotFound(w, "resource missing") },
			expectedCode: http.StatusNotFound,
			expectedErr:  response.ErrCodeNotFound,
		},
		{
			name:         "Conflict",
			call:         func(w http.ResponseWriter) { response.Conflict(w, "already exists", nil) },
			expectedCode: http.StatusConflict,
			expectedErr:  response.ErrCodeConflict,
		},
		{
			name:         "InternalServerError",
			call:         func(w http.ResponseWriter) { response.InternalServerError(w, "") },
			expectedCode: http.StatusInternalServerError,
			expectedErr:  response.ErrCodeInternalError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tc.call(rec)

			if rec.Code != tc.expectedCode {
				t.Errorf("expected status %d, got %d", tc.expectedCode, rec.Code)
			}

			var res response.APIResponse[any]
			if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
				t.Fatalf("failed to decode error body: %v", err)
			}

			if res.Success {
				t.Errorf("expected success to be false")
			}

			if res.Error == nil || res.Error.Code != tc.expectedErr {
				t.Errorf("expected error code %s, got %+v", tc.expectedErr, res.Error)
			}
		})
	}
}
