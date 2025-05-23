// hub.go
// Implements the Hub struct to manage all connected clients, message broadcasting, round state, and NATS/JetStream integration.
package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/erilali/logger"
	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
)

// Hub manages clients and rounds.
type Hub struct {
	clients     map[*Client]bool
	register    chan *Client
	unregister  chan *Client
	broadcast   chan []byte
	roundActive bool
	mu          sync.Mutex

	natsConn       *nats.Conn
	js             nats.JetStreamContext
	startTime      time.Time
	currentRoundID int64           // current round ID (timestamp)
	messageLimiter map[string]bool // maps username to round submission status
	logger         *logger.Logger  // structured logger
}

func NewHub(nc *nats.Conn, js nats.JetStreamContext) *Hub {
	return &Hub{
		clients:        make(map[*Client]bool),
		register:       make(chan *Client),
		unregister:     make(chan *Client),
		broadcast:      make(chan []byte),
		roundActive:    false,
		natsConn:       nc,
		js:             js,
		startTime:      time.Now(),
		currentRoundID: 0,
		messageLimiter: make(map[string]bool),
		logger:         logger.NewLogger("hub"),
	}
}

func (h *Hub) Run() {
	go h.runRoundManager()
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			h.logEvent("client_connected", client.username, "")
			go h.writePump(client)
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			h.logEvent("client_disconnected", client.username, "")
		case message := <-h.broadcast:
			h.mu.Lock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.Unlock()
		}
	}
}

func (h *Hub) logEvent(event, username, detail string) {
	h.logger.LogEvent("info", event, username, detail)
}

func (h *Hub) broadcastMessage(msgType, username, data string) {
	wsMsg := WSMessage{
		Version:  "1.0",
		Type:     msgType,
		Username: username,
		Data:     data,
	}
	message, err := json.Marshal(wsMsg)
	if err != nil {
		h.logger.Errorf("Error marshaling websocket message: %v", err)
		return
	}
	h.broadcast <- message
}

func (h *Hub) handleClientMessage(client *Client, clientMsg ClientMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Ensure the message type is correct.
	if clientMsg.Type != "client_message" {
		errorMessage := WSMessage{
			Version:   "1.0",
			Type:      "error",
			Username:  client.username,
			Data:      "Invalid message type.",
			ErrorCode: "INVALID_TYPE",
		}
		jsonError, _ := json.Marshal(errorMessage)
		_ = client.conn.WriteMessage(websocket.TextMessage, jsonError)
		return
	}

	// Check if we are in an active round.
	if !h.roundActive {
		errorMessage := WSMessage{
			Version:   "1.0",
			Type:      "error",
			Username:  client.username,
			Data:      "Message sent outside active round.",
			ErrorCode: "OUTSIDE_ROUND",
		}
		jsonError, _ := json.Marshal(errorMessage)
		_ = client.conn.WriteMessage(websocket.TextMessage, jsonError)
		return
	}

	// Enforce message length constraint.
	if len(clientMsg.Data) > 200 {
		errorMessage := WSMessage{
			Version:   "1.0",
			Type:      "error",
			Username:  client.username,
			Data:      "Message exceeds 200 characters.",
			ErrorCode: "MSG_TOO_LONG",
		}
		jsonError, _ := json.Marshal(errorMessage)
		_ = client.conn.WriteMessage(websocket.TextMessage, jsonError)
		return
	}

	// Check if the client already submitted a message this round.
	if h.messageLimiter[client.username] {
		errorMessage := WSMessage{
			Version:   "1.0",
			Type:      "error",
			Username:  client.username,
			Data:      "You have already sent a message this round.",
			ErrorCode: "ALREADY_SENT",
		}
		jsonError, _ := json.Marshal(errorMessage)
		_ = client.conn.WriteMessage(websocket.TextMessage, jsonError)
		return
	}

	// Record the valid message.
	message := &Message{
		Username: client.username,
		Content:  clientMsg.Data,
	}
	h.messageLimiter[client.username] = true

	// Log and publish via JetStream
	h.logEvent("message_received", client.username, message.Content)

	// Publish to JetStream
	if h.js != nil {
		msgData, _ := json.Marshal(message)
		subject := fmt.Sprintf("messages.%d", h.currentRoundID)
		_, err := h.js.Publish(subject, msgData)
		if err != nil {
			h.logger.Errorf("Error publishing message to JetStream: %v", err)
		}
	}

	// Send acknowledgement to client.
	ackMessage := WSMessage{
		Version:  "1.0",
		Type:     "ack",
		Username: client.username,
		Data:     "Message accepted.",
	}
	jsonAck, _ := json.Marshal(ackMessage)
	_ = client.conn.WriteMessage(websocket.TextMessage, jsonAck)
}

func (h *Hub) runRoundManager() {
	// TODO: implement round manager logic
}

func (h *Hub) writePump(client *Client) {
	defer func() {
		h.unregister <- client
		client.conn.Close()
	}()

	for message := range client.send {
		// Write message to websocket
		err := client.conn.WriteMessage(websocket.TextMessage, message)
		if err != nil {
			h.logger.Errorf("Error writing to websocket: %v", err)
			return
		}
	}
	// Channel closed, close websocket
	_ = client.conn.WriteMessage(websocket.CloseMessage, []byte{})
}
