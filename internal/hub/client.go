// internal/hub/client.go
package hub

import (
	"time"

	"github.com/gorilla/websocket"
)

// Client represents a connected user.
type Client struct {
	Username   string
	Conn       *websocket.Conn
	Send       chan []byte
	LastActive time.Time
}
