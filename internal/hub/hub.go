// internal/hub/hub.go
// Provides the Hub struct and related logic as a package.
package hub

import (
	"sync"
	"time"

	"github.com/erilali/internal/logger"
	"github.com/nats-io/nats.go"
)

// Hub represents the main hub that manages clients, rounds, and messaging
type Hub struct {
	Clients     map[*Client]bool
	Register    chan *Client
	Unregister  chan *Client
	Broadcast   chan []byte
	RoundActive bool
	Mu          sync.Mutex

	NatsConn       *nats.Conn
	Js             nats.JetStreamContext
	StartTime      time.Time
	CurrentRoundID int64           // current round ID (timestamp)
	MessageLimiter map[string]bool // maps username to round submission status
	Logger         *logger.Logger  // custom logger
}

// NewHub creates a new Hub instance
func NewHub(nc *nats.Conn, js nats.JetStreamContext, logger *logger.Logger) *Hub {
	return &Hub{
		Clients:        make(map[*Client]bool),
		Register:       make(chan *Client),
		Unregister:     make(chan *Client),
		Broadcast:      make(chan []byte),
		RoundActive:    false,
		NatsConn:       nc,
		Js:             js,
		StartTime:      time.Now(),
		CurrentRoundID: 0,
		MessageLimiter: make(map[string]bool),
		Logger:         logger,
	}
}

// Run handles registration, unregistration, and broadcasts for the hub.
func (h *Hub) Run() {
	// Start the round timer
	go h.StartRoundTimer()

	for {
		select {
		case client := <-h.Register:
			h.Mu.Lock()
			h.Clients[client] = true
			h.Mu.Unlock()
			h.Logger.Infof("Client registered: %s", client.Username)

		case client := <-h.Unregister:
			h.Mu.Lock()
			if _, ok := h.Clients[client]; ok {
				delete(h.Clients, client)
				close(client.Send)
				h.Logger.Infof("Client unregistered: %s", client.Username)
			}
			h.Mu.Unlock()

		case message := <-h.Broadcast:
			h.Mu.Lock()
			for client := range h.Clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.Clients, client)
				}
			}
			h.Mu.Unlock()
		}
	}
}
