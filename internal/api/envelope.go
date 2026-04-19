// Package api provides the HTTP client for the Kestrel Portfolio API.
package api

import (
	"encoding/json"
	"strings"
)

// Envelope is the standard API response wrapper.
// Every API response comes back as {"ok": true/false, "data": ..., "meta": ...}
// or, on validation failure, {"ok": false, "errors": [...]}.
//
// In Go, json.RawMessage is like keeping the raw JSON string unparsed —
// we decode it later into the specific type we need (Property, Lease, etc.).
// This is similar to Ruby's JSON.parse returning a generic Hash that you
// then map into a model.
type Envelope struct {
	OK     bool            `json:"ok"`
	Data   json.RawMessage `json:"data,omitempty"`
	Meta   *PaginationMeta `json:"meta,omitempty"`
	Error  string          `json:"error,omitempty"`
	Code   string          `json:"code,omitempty"`
	Errors []string        `json:"errors,omitempty"`
}

// PaginationMeta matches the API's pagination metadata.
// NextPage is a pointer (*int) because it can be null in JSON — Go doesn't have
// nil for plain ints, so we use a pointer. Similar to Ruby's nil vs 0 distinction.
type PaginationMeta struct {
	Page     int  `json:"page"`
	NextPage *int `json:"next_page"`
	Count    int  `json:"count"`
}

// APIError represents an error response from the API.
// It carries both the standard {error, code} form and the write-endpoint
// {errors: [...]} validation form. Callers that only want a human message
// call Error(); callers that care about which shape can check Code or Errors.
type APIError struct {
	StatusCode int
	Message    string   // from the `error` field (standard errors)
	Code       string   // from the `code` field (standard errors)
	Errors     []string // from the `errors` array (validation errors)
}

// Error satisfies the error interface — this is how Go does "duck typing".
// Any type with an Error() string method can be used wherever an error is expected,
// similar to how any Ruby object with a to_s method can be interpolated into a string.
func (e *APIError) Error() string {
	if len(e.Errors) > 0 {
		return strings.Join(e.Errors, "; ")
	}
	return e.Message
}

// IsValidation reports whether this is a validation error (422 with errors array).
// Commands can use this to give targeted UX on write failures.
func (e *APIError) IsValidation() bool {
	return len(e.Errors) > 0
}
