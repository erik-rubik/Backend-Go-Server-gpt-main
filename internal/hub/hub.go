// internal/hub/hub.go
// Provides the Hub struct and related logic as a package.
package hub

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/erilali/internal/logger"
	"github.com/nats-io/nats.go"
)

// RoundMessage represents a message submitted during a round
type RoundMessage struct {
	Username  string    `json:"username"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

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
	CurrentRoundID int64                    // current round ID (timestamp)
	MessageLimiter map[string]bool          // maps username to round submission status
	RoundMessages  map[int64][]RoundMessage // stores messages by round ID
	Logger         *logger.Logger           // custom logger
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
		RoundMessages:  make(map[int64][]RoundMessage),
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
			roundActive := h.RoundActive
			currentRoundID := h.CurrentRoundID
			h.Mu.Unlock()

			// Send current round status to the newly connected client
			if roundActive {
				roundMessage := map[string]interface{}{
					"version": "1.0",
					"type":    "round_start",
					"data":    currentRoundID,
				}
				h.sendMessageToClient(client, roundMessage)
			}

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

// sendMessageToClient sends a message directly to a specific client
func (h *Hub) sendMessageToClient(client *Client, message map[string]interface{}) {
	if data, err := json.Marshal(message); err == nil {
		select {
		case client.Send <- data:
		default:
			// Client is slow or disconnected, trigger cleanup
			h.Unregister <- client
		}
	}
}

// addRoundMessage adds a message to the current round
func (h *Hub) addRoundMessage(roundID int64, username, messageText string) {
	h.Mu.Lock()
	defer h.Mu.Unlock()

	if h.RoundMessages[roundID] == nil {
		h.RoundMessages[roundID] = make([]RoundMessage, 0)
	}

	roundMsg := RoundMessage{
		Username:  username,
		Message:   messageText,
		Timestamp: time.Now(),
	}

	h.RoundMessages[roundID] = append(h.RoundMessages[roundID], roundMsg)
}

// cleanupOldMessages removes messages from rounds older than the specified number of rounds
func (h *Hub) cleanupOldMessages(currentRoundID int64) {
	h.Mu.Lock()
	defer h.Mu.Unlock()

	const keepRounds = 3
	var roundIDs []int64
	for id := range h.RoundMessages {
		roundIDs = append(roundIDs, id)
	}

	if len(roundIDs) > keepRounds {
		// Sort and keep only recent rounds
		for _, id := range roundIDs {
			if id < currentRoundID-int64(keepRounds-1) {
				delete(h.RoundMessages, id)
			}
		}
	}
}
