package casper

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// ErrorKind describes a high–level category of Casper API errors.
type ErrorKind string

const (
	// ErrorKindClient means the request was invalid (400 and any other 4xx
	// without a kind of its own).
	ErrorKindClient ErrorKind = "client"
	// ErrorKindServer means the server failed to process the request (500
	// and any other 5xx without a kind of its own).
	ErrorKindServer ErrorKind = "server"
	// ErrorKindUnauthorized means the credential is missing or invalid (401).
	ErrorKindUnauthorized ErrorKind = "unauthorized"
	// ErrorKindForbidden means the credential lacks access (403).
	ErrorKindForbidden ErrorKind = "forbidden"
	// ErrorKindNotFound means a requested resource does not exist (404).
	ErrorKindNotFound ErrorKind = "not_found"
	// ErrorKindOperationNotAllowed means the operation is not allowed (405).
	ErrorKindOperationNotAllowed ErrorKind = "operation_not_allowed"
	// ErrorKindNotLeader means a mutation reached a cluster node that is not
	// the Raft leader (405). Error.LeaderAddr names the leader when one is
	// known; retry the request there.
	ErrorKindNotLeader ErrorKind = "not_leader"
	// ErrorKindConflict means the request conflicts with the collection's state
	// (409): a trigger with no index or the mechanism off, or a policy change
	// that lost a race.
	ErrorKindConflict ErrorKind = "conflict"
	// ErrorKindPayloadTooLarge means the batch is over the server's limit
	// (413); split it.
	ErrorKindPayloadTooLarge ErrorKind = "payload_too_large"
	// ErrorKindTooManyRequests means the server's write queue is full (429);
	// Error.RetryAfter says when to try again.
	ErrorKindTooManyRequests ErrorKind = "too_many_requests"
	// ErrorKindUnavailable means the node cannot serve the request yet (503);
	// Error.RetryAfter says when to try again.
	ErrorKindUnavailable ErrorKind = "unavailable"
	// ErrorKindTimeout means the write's outcome is unknown (504). Retrying
	// is safe: writes are upserts by id.
	ErrorKindTimeout ErrorKind = "timeout"
	// ErrorKindInsufficientStorage means the node is short of free disk (507).
	ErrorKindInsufficientStorage ErrorKind = "insufficient_storage"
	// ErrorKindTransport means no HTTP answer came back: the connection
	// failed, or the request timed out or was cancelled. Cause holds the
	// underlying error. A write may still have been applied.
	ErrorKindTransport ErrorKind = "transport"
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
	// LeaderAddr is the Raft leader's address (e.g. "10.0.0.7:7222") for
	// ErrorKindNotLeader, empty while no leader is known or for other kinds.
	LeaderAddr string
	// RetryAfter is the server's Retry-After for 429 and 503, 0 when absent.
	RetryAfter time.Duration
	// Cause optionally contains the underlying error (network, JSON decoding, gRPC, etc.).
	Cause error
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Status > 0 {
		msg := fmt.Sprintf("casper: %s (status=%d): %s", e.Kind, e.Status, e.Message)
		if e.LeaderAddr != "" {
			msg += fmt.Sprintf(" (leader=%s)", e.LeaderAddr)
		}
		if e.RetryAfter > 0 {
			msg += fmt.Sprintf(" (retry after %s)", e.RetryAfter)
		}
		return msg
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

// errorFromResponse builds a Casper Error from a non-2xx response and its
// already-read body. The body is plain text, except for the JSON refusal of
// a node that is not the leader.
func errorFromResponse(resp *http.Response, body []byte) *Error {
	var jsonErr struct {
		Error      string  `json:"error"`
		LeaderAddr *string `json:"leader_addr"`
	}
	msg := string(body)
	isJSON := json.Unmarshal(body, &jsonErr) == nil && jsonErr.Error != ""
	if isJSON {
		msg = jsonErr.Error
	}

	e := classifyHTTPError(resp.StatusCode, msg)
	if resp.StatusCode == http.StatusMethodNotAllowed && isJSON {
		e.Kind = ErrorKindNotLeader
		if jsonErr.LeaderAddr != nil {
			e.LeaderAddr = *jsonErr.LeaderAddr
		}
	}
	if seconds, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && seconds > 0 {
		e.RetryAfter = time.Duration(seconds) * time.Second
	}
	return e
}

// classifyHTTPError builds a Casper Error from an HTTP status code and message body.
func classifyHTTPError(status int, message string) *Error {
	var kind ErrorKind
	switch {
	case status == http.StatusUnauthorized:
		kind = ErrorKindUnauthorized
	case status == http.StatusForbidden:
		kind = ErrorKindForbidden
	case status == http.StatusNotFound:
		kind = ErrorKindNotFound
	case status == http.StatusMethodNotAllowed:
		kind = ErrorKindOperationNotAllowed
	case status == http.StatusConflict:
		kind = ErrorKindConflict
	case status == http.StatusRequestEntityTooLarge:
		kind = ErrorKindPayloadTooLarge
	case status == http.StatusTooManyRequests:
		kind = ErrorKindTooManyRequests
	case status == http.StatusServiceUnavailable:
		kind = ErrorKindUnavailable
	case status == http.StatusGatewayTimeout:
		kind = ErrorKindTimeout
	case status == http.StatusInsufficientStorage:
		kind = ErrorKindInsufficientStorage
	case status >= 400 && status < 500:
		kind = ErrorKindClient
	case status >= 500 && status <= 599:
		kind = ErrorKindServer
	default:
		kind = ErrorKindUnknown
	}
	return &Error{
		Kind:    kind,
		Status:  status,
		Message: message,
	}
}
