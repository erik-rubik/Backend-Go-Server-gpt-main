// client.go
// Defines the Client struct for WebSocket users and handles WebSocket upgrades and client connection management.
package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// Client represents a connected user.
type Client struct {
	username   string
	conn       *websocket.Conn
	send       chan []byte
	lastActive time.Time
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // In production, enforce CORS.
	},
}

func (h *Hub) serveWs(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")
	if username == "" {
		http.Error(w, "username is required", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Errorf("WebSocket upgrade error: %v", err)
		return
	}

	client := &Client{
		username:   username,
		conn:       conn,
		send:       make(chan []byte, 256),
		lastActive: time.Now(),
	}
	h.register <- client

	go h.readPump(client)
}

func (h *Hub) readPump(client *Client) {
	defer func() {
		h.unregister <- client
		client.conn.Close()
	}()

	client.conn.SetReadLimit(512)
	_ = client.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	client.conn.SetPongHandler(func(string) error {
		_ = client.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, msg, err := client.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				h.logEvent("read_error", client.username, err.Error())
			}
			break
		}

		client.lastActive = time.Now() // update activity timestamp

		var clientMsg ClientMessage
		if err := json.Unmarshal(msg, &clientMsg); err != nil {
			// If message is not valid JSON, send error directly.
			errorMessage := WSMessage{
				Version:   "1.0",
				Type:      "error",
				Username:  client.username,
				Data:      "Invalid JSON format.",
				ErrorCode: "INVALID_JSON",
			}
			jsonError, _ := json.Marshal(errorMessage)
			_ = client.conn.WriteMessage(websocket.TextMessage, jsonError)
			continue
		}

		// Validate message version.
		if clientMsg.Version != "1.0" {
			errorMessage := WSMessage{
				Version:   "1.0",
				Type:      "error",
				Username:  client.username,
				Data:      "Unsupported version.",
				ErrorCode: "INVALID_VERSION",
			}
			jsonError, _ := json.Marshal(errorMessage)
			_ = client.conn.WriteMessage(websocket.TextMessage, jsonError)
			continue
		}

		// Process client message.
		h.handleClientMessage(client, clientMsg)
	}
}
