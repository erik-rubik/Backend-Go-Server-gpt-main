// internal/hub/rounds.go
package hub

import (
	"time"
)

const (
	roundDuration         = 15 * time.Second
	countdownStartSeconds = 10
)

// StartRoundTimer starts the round management timer.
func (h *Hub) StartRoundTimer() {
	ticker := time.NewTicker(roundDuration)
	defer ticker.Stop()

	// Start first round immediately
	h.StartRound()

	// End the current round and start a new one on each tick.
	for range ticker.C {
		h.EndRound()
		h.StartRound()
	}
}

// StartRound begins a new message round.
func (h *Hub) StartRound() {
	h.Mu.Lock()
	h.RoundActive = true
	h.CurrentRoundID = time.Now().Unix()
	h.MessageLimiter = make(map[string]bool) // Reset submission tracker
	h.Mu.Unlock()

	// Broadcast round start
	roundMessage := map[string]interface{}{
		"version": "1.0",
		"type":    "round_start",
		"data":    h.CurrentRoundID,
	}

	h.BroadcastMessage(roundMessage)

	// Publish round start to NATS
	h.publishRoundStartToNATS()

	h.Logger.Infof("Round %d started", h.CurrentRoundID)

	// Start countdown
	go h.StartCountdown(h.CurrentRoundID)
}

// EndRound ends the current message round and selects a winner.
func (h *Hub) EndRound() {
	h.Mu.Lock()
	h.RoundActive = false
	roundID := h.CurrentRoundID
	h.Mu.Unlock()

	// Broadcast round end
	roundMessage := map[string]interface{}{
		"version": "1.0",
		"type":    "round_end",
		"data":    roundID,
	}

	h.BroadcastMessage(roundMessage)

	// Publish round end to NATS
	h.publishRoundEndToNATS(roundID)

	h.Logger.Infof("Round %d ended", roundID)

	// Select and announce winner (simplified random selection)
	go h.SelectWinner(roundID)
}

// StartCountdown sends countdown messages to clients.
func (h *Hub) StartCountdown(roundID int64) {
	// Countdown text updates disabled per UI simplification request (graphical timer only)
	for i := countdownStartSeconds; i >= 1; i-- {
		// Maintain timing alignment without broadcasting messages
		time.Sleep(1 * time.Second)
		h.Mu.Lock()
		if !h.RoundActive || h.CurrentRoundID != roundID {
			h.Mu.Unlock()
			return
		}
		h.Mu.Unlock()
	}
}
