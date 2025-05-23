// message.go
// Contains data structures for messages exchanged between clients and server, including WebSocket and API message formats.
package main

// Message represents a valid message for the current round.
type Message struct {
	Username string `json:"username"`
	Content  string `json:"content"`
}

// ClientMessage represents a message sent from a client.
type ClientMessage struct {
	Version  string `json:"version"`
	Type     string `json:"type"`
	Username string `json:"username"`
	Data     string `json:"data"`
}

// LogEntry represents a structured log entry.
type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Event     string `json:"event"`
	Username  string `json:"username,omitempty"`
	Message   string `json:"message,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// WSMessage represents a structured websocket message sent by the server.
type WSMessage struct {
	Version   string `json:"version"`
	Type      string `json:"type"`
	Username  string `json:"username,omitempty"`
	Data      string `json:"data"`
	ErrorCode string `json:"error_code,omitempty"`
}
