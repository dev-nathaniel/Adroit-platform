package response

import (
	"encoding/json"
	"net/http"
)

// Standard error codes used across all platform services
const (
	ErrCodeInvalidRequest     = "INVALID_REQUEST"
	ErrCodeUnauthorized       = "UNAUTHORIZED"
	ErrCodeForbidden          = "FORBIDDEN"
	ErrCodeNotFound           = "NOT_FOUND"
	ErrCodeConflict           = "CONFLICT"
	ErrCodeRateLimited        = "RATE_LIMITED"
	ErrCodeInternalError      = "INTERNAL_SERVER_ERROR"
	ErrCodeServiceUnavailable = "SERVICE_UNAVAILABLE"
)

// APIResponse represents the universal response envelope for all platform endpoints
type APIResponse[T any] struct {
	Success bool        `json:"success"`
	Data    T           `json:"data,omitempty"`
	Error   *APIError   `json:"error,omitempty"`
	Meta    *Pagination `json:"meta,omitempty"`
}

// APIError encapsulates standard error payload details
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// Pagination contains pagination metadata for list endpoints
type Pagination struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	TotalItems int `json:"total_items"`
	TotalPages int `json:"total_pages"`
}

// JSON sends a JSON response with status code and data payload
func JSON[T any](w http.ResponseWriter, status int, data T) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(APIResponse[T]{
		Success: true,
		Data:    data,
	})
}

// JSONWithMeta sends a JSON response with status code, data payload, and pagination metadata
func JSONWithMeta[T any](w http.ResponseWriter, status int, data T, meta *Pagination) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(APIResponse[T]{
		Success: true,
		Data:    data,
		Meta:    meta,
	})
}

// Error writes an error response envelope to the client
func Error(w http.ResponseWriter, status int, code, message string, details any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(APIResponse[any]{
		Success: false,
		Error: &APIError{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}

// BadRequest writes a 400 Bad Request error
func BadRequest(w http.ResponseWriter, message string, details any) {
	Error(w, http.StatusBadRequest, ErrCodeInvalidRequest, message, details)
}

// Unauthorized writes a 401 Unauthorized error
func Unauthorized(w http.ResponseWriter, message string) {
	if message == "" {
		message = "Authentication credentials required or invalid"
	}
	Error(w, http.StatusUnauthorized, ErrCodeUnauthorized, message, nil)
}

// Forbidden writes a 403 Forbidden error
func Forbidden(w http.ResponseWriter, message string) {
	if message == "" {
		message = "You do not have permission to perform this action"
	}
	Error(w, http.StatusForbidden, ErrCodeForbidden, message, nil)
}

// NotFound writes a 404 Not Found error
func NotFound(w http.ResponseWriter, message string) {
	if message == "" {
		message = "The requested resource was not found"
	}
	Error(w, http.StatusNotFound, ErrCodeNotFound, message, nil)
}

// Conflict writes a 409 Conflict error
func Conflict(w http.ResponseWriter, message string, details any) {
	Error(w, http.StatusConflict, ErrCodeConflict, message, details)
}

// InternalServerError writes a 500 Internal Server Error
func InternalServerError(w http.ResponseWriter, message string) {
	if message == "" {
		message = "An unexpected error occurred while processing the request"
	}
	Error(w, http.StatusInternalServerError, ErrCodeInternalError, message, nil)
}
