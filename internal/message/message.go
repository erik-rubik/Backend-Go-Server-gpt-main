// internal/message/message.go
// Contains data structures for messages exchanged between clients and server.
package message

type Message struct {
	Username string `json:"username"`
	Content  string `json:"content"`
}

type ClientMessage struct {
	Version  string `json:"version"`
	Type     string `json:"type"`
	Username string `json:"username"`
	Data     string `json:"data"`
}

type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Event     string `json:"event"`
	Username  string `json:"username,omitempty"`
	Message   string `json:"message,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

type WSMessage struct {
	Version   string `json:"version"`
	Type      string `json:"type"`
	Username  string `json:"username,omitempty"`
	Data      string `json:"data"`
	ErrorCode string `json:"error_code,omitempty"`
}
