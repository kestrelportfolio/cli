// Package api provides the HTTP client for the Kestrel Portfolio API.
package api

import "encoding/json"

// Envelope is the standard API response wrapper.
// Every API response comes back as {"ok": true/false, "data": ..., "meta": ...}.
//
// In Go, json.RawMessage is like keeping the raw JSON string unparsed —
// we decode it later into the specific type we need (Property, Lease, etc.).
// This is similar to Ruby's JSON.parse returning a generic Hash that you
// then map into a model.
type Envelope struct {
	OK    bool                `json:"ok"`
	Data  json.RawMessage     `json:"data,omitempty"`
	Meta  *PaginationMeta     `json:"meta,omitempty"`
	Error string              `json:"error,omitempty"`
	Code  string              `json:"code,omitempty"`
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
// It implements Go's error interface (like Ruby's Exception class).
type APIError struct {
	StatusCode int
	Message    string
	Code       string
}

// Error satisfies the error interface — this is how Go does "duck typing".
// Any type with an Error() string method can be used wherever an error is expected,
// similar to how any Ruby object with a to_s method can be interpolated into a string.
func (e *APIError) Error() string {
	return e.Message
}
