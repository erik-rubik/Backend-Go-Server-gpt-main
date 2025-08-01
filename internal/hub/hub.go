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

// NewHub creates a new Hub instance and initializes its fields.
// It sets up channels for client registration, unregistration, and message broadcasting.
// It also initializes NATS connection details, logger, and other hub-specific properties.
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

// Run starts the main event loop for the Hub.
// It listens for new client registrations, client unregistrations, and messages to broadcast.
// It also launches a goroutine to manage round timing.
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
			// To avoid holding the lock while sending on channels,
			// we make a copy of the clients map.
			h.Mu.Lock()
			clientsToBroadcast := make([]*Client, 0, len(h.Clients))
			for client := range h.Clients {
				clientsToBroadcast = append(clientsToBroadcast, client)
			}
			h.Mu.Unlock()

			for _, client := range clientsToBroadcast {
				select {
				case client.Send <- message:
				default:
					// Assume client is disconnected or slow.
					// Let the read/write pumps handle the cleanup.
					// We can trigger it by sending to the unregister channel.
					h.Unregister <- client
				}
			}
		}
	}
}
