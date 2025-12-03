package casper

import (
	"fmt"
	"net/http"
)

// ErrorKind describes a high–level category of Casper API errors.
type ErrorKind string

const (
	// ErrorKindClient means the request was invalid (4xx except 404/405/409).
	ErrorKindClient ErrorKind = "client"
	// ErrorKindServer means the server failed to process the request (5xx).
	ErrorKindServer ErrorKind = "server"
	// ErrorKindNotFound means a requested resource does not exist (404).
	ErrorKindNotFound ErrorKind = "not_found"
	// ErrorKindOperationNotAllowed means the operation is not allowed (405).
	ErrorKindOperationNotAllowed ErrorKind = "operation_not_allowed"
	// ErrorKindConflict represents conflicts such as index already exists (409).
	ErrorKindConflict ErrorKind = "conflict"
	// ErrorKindInvalidResponse means the server response was malformed or unexpected.
	ErrorKindInvalidResponse ErrorKind = "invalid_response"
	// ErrorKindUnknown is used for everything that does not fit into the categories above.
	ErrorKindUnknown ErrorKind = "unknown"
)

// Error represents a structured Casper client error.
type Error struct {
	// Kind is a coarse–grained classification of the error.
	Kind ErrorKind
	// Status is the HTTP status code if the error came from an HTTP call, 0 otherwise.
	Status int
	// Message is a human–readable error description.
	Message string
	// Cause optionally contains the underlying error (network, JSON decoding, gRPC, etc.).
	Cause error
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Status > 0 {
		return fmt.Sprintf("casper: %s (status=%d): %s", e.Kind, e.Status, e.Message)
	}
	if e.Cause != nil {
		return fmt.Sprintf("casper: %s: %s (cause=%v)", e.Kind, e.Message, e.Cause)
	}
	return fmt.Sprintf("casper: %s: %s", e.Kind, e.Message)
}

// Unwrap allows errors.Is / errors.As to inspect the underlying cause.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// classifyHTTPError builds a Casper Error from an HTTP status code and message body.
func classifyHTTPError(status int, message string) *Error {
	switch {
	case status == http.StatusNotFound:
		return &Error{
			Kind:    ErrorKindNotFound,
			Status:  status,
			Message: message,
		}
	case status == http.StatusMethodNotAllowed:
		return &Error{
			Kind:    ErrorKindOperationNotAllowed,
			Status:  status,
			Message: message,
		}
	case status == http.StatusConflict:
		return &Error{
			Kind:    ErrorKindConflict,
			Status:  status,
			Message: message,
		}
	case status >= 400 && status < 500:
		return &Error{
			Kind:    ErrorKindClient,
			Status:  status,
			Message: message,
		}
	case status >= 500 && status <= 599:
		return &Error{
			Kind:    ErrorKindServer,
			Status:  status,
			Message: message,
		}
	default:
		return &Error{
			Kind:    ErrorKindUnknown,
			Status:  status,
			Message: message,
		}
	}
}
