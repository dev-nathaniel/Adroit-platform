package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultTimeout is the standard HTTP request timeout.
const DefaultTimeout = 30 * time.Second

// APIResponse matches the unified JSON envelope returned by the Adroit API Gateway.
type APIResponse struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   *APIErrorDetail `json:"error,omitempty"`
	Meta    any             `json:"meta,omitempty"`
}

// APIErrorDetail represents the standard error body.
type APIErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// APIError is returned when an API request returns an HTTP status >= 400.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
	Details    any
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("[%s] %s (HTTP %d)", e.Code, e.Message, e.StatusCode)
	}
	return fmt.Sprintf("%s (HTTP %d)", e.Message, e.StatusCode)
}

// Client interacts with the Adroit API Gateway.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
	UserAgent  string
}

// New creates a new API client.
func New(baseURL, token string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		UserAgent: "Adroit-CLI/v1.0.0",
	}
}

// SetToken updates the client's bearer token.
func (c *Client) SetToken(token string) {
	c.Token = token
}

// Do executes an HTTP request with authentication and custom headers.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", c.UserAgent)
	if c.Token != "" && req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	return c.HTTPClient.Do(req)
}

// requestJSON executes an HTTP request, checks for errors, and decodes the standard JSON envelope.
func (c *Client) requestJSON(ctx context.Context, method, path string, body any, out any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to encode request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	url := c.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("network request failed (%s %s): %w", method, path, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	var envelope APIResponse
	if len(respBytes) > 0 {
		if err := json.Unmarshal(respBytes, &envelope); err != nil {
			if resp.StatusCode >= 400 {
				return &APIError{
					StatusCode: resp.StatusCode,
					Code:       "HTTP_ERROR",
					Message:    string(respBytes),
				}
			}
			return fmt.Errorf("failed to parse JSON response: %w (raw: %s)", err, string(respBytes))
		}
	}

	if resp.StatusCode >= 400 || !envelope.Success {
		code := "UNKNOWN_ERROR"
		msg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		var details any
		if envelope.Error != nil {
			if envelope.Error.Code != "" {
				code = envelope.Error.Code
			}
			if envelope.Error.Message != "" {
				msg = envelope.Error.Message
			}
			details = envelope.Error.Details
		}
		return &APIError{
			StatusCode: resp.StatusCode,
			Code:       code,
			Message:    msg,
			Details:    details,
		}
	}

	if out != nil && len(envelope.Data) > 0 {
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return fmt.Errorf("failed to decode response payload: %w", err)
		}
	}

	return nil
}

// Get performs a GET request and unmarshals data into out.
func (c *Client) Get(ctx context.Context, path string, out any) error {
	return c.requestJSON(ctx, http.MethodGet, path, nil, out)
}

// Post performs a POST request and unmarshals data into out.
func (c *Client) Post(ctx context.Context, path string, body, out any) error {
	return c.requestJSON(ctx, http.MethodPost, path, body, out)
}

// Patch performs a PATCH request and unmarshals data into out.
func (c *Client) Patch(ctx context.Context, path string, body, out any) error {
	return c.requestJSON(ctx, http.MethodPatch, path, body, out)
}

// Delete performs a DELETE request and unmarshals data into out.
func (c *Client) Delete(ctx context.Context, path string, out any) error {
	return c.requestJSON(ctx, http.MethodDelete, path, nil, out)
}

// PostRaw performs a POST request with custom Content-Type and raw reader.
func (c *Client) PostRaw(ctx context.Context, path, contentType string, body io.Reader, out any) error {
	url := c.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("network request failed (POST %s): %w", path, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	var envelope APIResponse
	if len(respBytes) > 0 {
		if err := json.Unmarshal(respBytes, &envelope); err != nil {
			if resp.StatusCode >= 400 {
				return &APIError{
					StatusCode: resp.StatusCode,
					Code:       "HTTP_ERROR",
					Message:    string(respBytes),
				}
			}
		}
	}

	if resp.StatusCode >= 400 || !envelope.Success {
		code := "UNKNOWN_ERROR"
		msg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		var details any
		if envelope.Error != nil {
			if envelope.Error.Code != "" {
				code = envelope.Error.Code
			}
			if envelope.Error.Message != "" {
				msg = envelope.Error.Message
			}
			details = envelope.Error.Details
		}
		return &APIError{
			StatusCode: resp.StatusCode,
			Code:       code,
			Message:    msg,
			Details:    details,
		}
	}

	if out != nil && len(envelope.Data) > 0 {
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return fmt.Errorf("failed to unmarshal response data: %w", err)
		}
	}

	return nil
}
