// internal/hub/websocket.go
package hub

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	webSocketReadDeadline  = 60 * time.Second
	webSocketWriteDeadline = 10 * time.Second
	webSocketPingPeriod    = (webSocketReadDeadline * 9) / 10 // Must be less than readDeadline
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// TODO: Implement proper origin check for production
		// For development, allow all origins
		return true
	},
}

// ServeWs upgrades the HTTP connection to a WebSocket and registers the client.
func (h *Hub) ServeWs(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")
	if username == "" {
		http.Error(w, "username is required", http.StatusBadRequest)
		return
	}

	// Validate username
	if !validateUsername(username) {
		http.Error(w, "invalid username: must be 3-20 characters, alphanumeric and underscore only", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.Logger.Errorf("WebSocket upgrade error: %v", err)
		return
	}

	client := &Client{
		Username:   username,
		Conn:       conn,
		Send:       make(chan []byte, 256),
		LastActive: time.Now(),
	}
	h.Register <- client
	go h.ReadPump(client)
	go h.WritePump(client)
}

// ReadPump reads messages from the WebSocket connection.
func (h *Hub) ReadPump(client *Client) {
	defer func() {
		h.Unregister <- client
		client.Conn.Close()
	}()

	// Set read deadline and pong handler
	client.Conn.SetReadLimit(512) // Max message size
	client.Conn.SetReadDeadline(time.Now().Add(webSocketReadDeadline))
	client.Conn.SetPongHandler(func(string) error {
		client.Conn.SetReadDeadline(time.Now().Add(webSocketReadDeadline))
		return nil
	})

	for {
		var message map[string]interface{}
		err := client.Conn.ReadJSON(&message)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				h.Logger.Errorf("WebSocket error for %s: %v", client.Username, err)
			}
			break
		}

		client.LastActive = time.Now()
		h.HandleClientMessage(client, message)
	}
}

// WritePump writes messages to the WebSocket connection.
func (h *Hub) WritePump(client *Client) {
	ticker := time.NewTicker(webSocketPingPeriod)
	defer func() {
		ticker.Stop()
		client.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-client.Send:
			client.Conn.SetWriteDeadline(time.Now().Add(webSocketWriteDeadline))
			if !ok {
				// The hub closed the channel.
				client.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := client.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued messages to the current write
			n := len(client.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-client.Send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			client.Conn.SetWriteDeadline(time.Now().Add(webSocketWriteDeadline))
			if err := client.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return // Client connection is likely broken
			}
		}
	}
}
