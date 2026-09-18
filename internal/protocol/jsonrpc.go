// Package protocol defines the UFoundry engine protocol: JSON-RPC 2.0 messages
// exchanged one per line (Unix socket) or one per frame (WebSocket).
//
// The engine streams items inside turns inside threads. See docs/protocol.md.
package protocol

import (
	"encoding/json"
	"fmt"
)

// Version is the protocol version. Clients must match the major version.
const Version = "1.0.0"

// Message is the envelope for every JSON-RPC message. Exactly one of the
// request, response or notification shapes is populated.
type Message struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *Error           `json:"error,omitempty"`
}

// IsRequest reports whether m is a request (has id and method).
func (m *Message) IsRequest() bool { return m.ID != nil && m.Method != "" }

// IsNotification reports whether m is a notification (method, no id).
func (m *Message) IsNotification() bool { return m.ID == nil && m.Method != "" }

// IsResponse reports whether m is a response (id, no method).
func (m *Message) IsResponse() bool { return m.ID != nil && m.Method == "" }

// Error is a JSON-RPC error object.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *Error) Error() string { return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message) }

// Standard and application error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternal       = -32603

	CodeNotFound        = -32001
	CodeBudgetExceeded  = -32002
	CodeProviderError   = -32003
	CodeVersionMismatch = -32004
	CodeConflict        = -32005
)

// Errorf builds an *Error with the given code.
func Errorf(code int, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// NewRequest builds a request message.
func NewRequest(id int64, method string, params any) (*Message, error) {
	raw, err := marshalParams(params)
	if err != nil {
		return nil, err
	}
	idRaw := json.RawMessage(fmt.Sprintf("%d", id))
	return &Message{JSONRPC: "2.0", ID: &idRaw, Method: method, Params: raw}, nil
}

// NewNotification builds a notification message.
func NewNotification(method string, params any) (*Message, error) {
	raw, err := marshalParams(params)
	if err != nil {
		return nil, err
	}
	return &Message{JSONRPC: "2.0", Method: method, Params: raw}, nil
}

// NewResult builds a success response for id.
func NewResult(id *json.RawMessage, result any) (*Message, error) {
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return &Message{JSONRPC: "2.0", ID: id, Result: raw}, nil
}

// NewError builds an error response for id.
func NewError(id *json.RawMessage, e *Error) *Message {
	if id == nil {
		null := json.RawMessage("null")
		id = &null
	}
	return &Message{JSONRPC: "2.0", ID: id, Error: e}
}

func marshalParams(params any) (json.RawMessage, error) {
	if params == nil {
		return nil, nil
	}
	return json.Marshal(params)
}
