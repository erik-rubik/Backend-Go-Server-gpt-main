// internal/hub/messaging.go
package hub

import (
	"encoding/json"
	"strings"
)

// validateUsername checks if the username is valid
func validateUsername(username string) bool {
	// Check length (3-20 characters)
	if len(username) < 3 || len(username) > 20 {
		return false
	}

	// Check for only alphanumeric and underscore characters
	for _, char := range username {
		if !((char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '_') {
			return false
		}
	}

	return true
}

// validateMessageContent checks if message content is valid
func validateMessageContent(content string) bool {
	// Trim whitespace
	content = strings.TrimSpace(content)

	// Check length (1-500 characters)
	return len(content) >= 1 && len(content) <= 500
}

// HandleClientMessage processes incoming messages from clients.
func (h *Hub) HandleClientMessage(client *Client, message map[string]interface{}) {
	messageType, ok := message["type"].(string)
	if !ok {
		h.SendErrorMessage(client, "Invalid message format")
		return
	}

	switch messageType {
	case "client_message":
		if !h.RoundActive {
			h.SendErrorMessage(client, "No active round")
			return
		}

		// Check if user already submitted for this round
		h.Mu.Lock()
		if h.MessageLimiter[client.Username] {
			h.Mu.Unlock()
			h.SendErrorMessage(client, "You have already submitted a message for this round")
			return
		}
		h.MessageLimiter[client.Username] = true
		h.Mu.Unlock()
		data, ok := message["data"].(string)
		if !ok || data == "" {
			h.SendErrorMessage(client, "Invalid message data")
			return
		}

		// Validate message content
		if !validateMessageContent(data) {
			h.SendErrorMessage(client, "Invalid message content: must be 1-500 characters")
			return
		}

		h.ProcessMessage(client, data)
	default:
		h.SendErrorMessage(client, "Unknown message type")
	}
}

// ProcessMessage handles a valid client message during an active round.
func (h *Hub) ProcessMessage(client *Client, content string) {
	// Send acknowledgment
	h.SendAckMessage(client)

	// Publish to NATS if available
	h.publishMessageToNATS(client, content)

	h.Logger.Infof("Message from %s: %s", client.Username, content)
}

// SendErrorMessage sends an error message to a specific client.
func (h *Hub) SendErrorMessage(client *Client, errorMsg string) {
	message := map[string]interface{}{
		"version": "1.0",
		"type":    "error",
		"data":    errorMsg,
	}

	if data, err := json.Marshal(message); err == nil {
		select {
		case client.Send <- data:
		default:
			close(client.Send)
			h.Mu.Lock()
			delete(h.Clients, client)
			h.Mu.Unlock()
		}
	}
}

// SendAckMessage sends an acknowledgment message to a specific client.
func (h *Hub) SendAckMessage(client *Client) {
	message := map[string]interface{}{
		"version": "1.0",
		"type":    "ack",
		"data":    "Message received successfully",
	}

	if data, err := json.Marshal(message); err == nil {
		select {
		case client.Send <- data:
		default:
			close(client.Send)
			h.Mu.Lock()
			delete(h.Clients, client)
			h.Mu.Unlock()
		}
	}
}

// BroadcastMessage sends a message to all connected clients.
func (h *Hub) BroadcastMessage(message map[string]interface{}) {
	if data, err := json.Marshal(message); err == nil {
		h.Broadcast <- data
	}
}
