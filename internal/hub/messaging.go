// internal/hub/messaging.go
package hub

import (
	"encoding/json"
	"strings"
)

// validateUsername checks if the provided username is valid according to predefined rules.
// Rules include length constraints (3-20 characters) and character set (alphanumeric and underscore).
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

// validateMessageContent checks if the provided message content is valid.
// It trims leading/trailing whitespace and then checks length constraints (1-500 characters).
func validateMessageContent(content string) bool {
	// Trim whitespace
	content = strings.TrimSpace(content)

	// Check length (1-500 characters)
	return len(content) >= 1 && len(content) <= 500
}

// HandleClientMessage processes incoming messages from a connected client.
// It first determines the message type and then routes it to the appropriate handler.
// For "client_message" type, it performs checks for active round, submission limits, and message validity before processing.
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

// ProcessMessage takes a valid client message during an active round, stores it,
// broadcasts it to all clients, publishes to NATS, and logs the message.
func (h *Hub) ProcessMessage(client *Client, content string) {
	h.Mu.Lock()
	currentRoundID := h.CurrentRoundID
	h.Mu.Unlock()

	// Store the message for winner selection
	h.addRoundMessage(currentRoundID, client.Username, content)

	// No broadcast of individual messages – only the winning message is ever shown to everyone.
	// Optionally still acknowledge the sender locally so they know it was accepted.
	h.SendAckMessage(client) // Keep per-user ack (not broadcast)

	// Publish to NATS if available
	h.publishMessageToNATS(client, content)

	h.Logger.Infof("Message from %s in round %d: %s", client.Username, currentRoundID, content)
}

// SendErrorMessage constructs and sends an error message to a specific client.
// The error message includes a version, type ("error"), and the error details.
// If sending fails, it closes the client's send channel and removes the client from the hub.
func (h *Hub) SendErrorMessage(client *Client, errorMsg string) {
	message := map[string]interface{}{
		"version": "1.0",
		"type":    "error",
		"data":    errorMsg,
	}

	if data, err := json.Marshal(message); err == nil {
		// The WritePump has a deadline and will handle a slow client.
		// Sending to client.Send will block until the WritePump is ready.
		client.Send <- data
	}
}

// SendAckMessage constructs and sends an acknowledgment message to a specific client.
// This is typically sent after a client's message has been successfully received and initially processed.
// If sending fails, it closes the client's send channel and removes the client from the hub.
func (h *Hub) SendAckMessage(client *Client) {
	message := map[string]interface{}{
		"version": "1.0",
		"type":    "ack",
		"data":    "Message received successfully",
	}

	if data, err := json.Marshal(message); err == nil {
		// The WritePump has a deadline and will handle a slow client.
		// Sending to client.Send will block until the WritePump is ready.
		client.Send <- data
	}
}

// BroadcastMessage marshals a given message map into JSON and sends it to the hub's broadcast channel.
// This channel is then read by the hub's Run loop to distribute the message to all connected clients.
func (h *Hub) BroadcastMessage(message map[string]interface{}) {
	if data, err := json.Marshal(message); err == nil {
		h.Broadcast <- data
	}
}
