// internal/hub/rounds.go
package hub

import (
	"fmt"
	"time"
)

// StartRoundTimer starts the round management timer.
func (h *Hub) StartRoundTimer() {
	ticker := time.NewTicker(30 * time.Second) // 30-second rounds
	defer ticker.Stop()

	// Start first round immediately
	h.StartRound()

	for {
		select {
		case <-ticker.C:
			if h.RoundActive {
				h.EndRound()
			} else {
				h.StartRound()
			}
		}
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
		"type":    "round_info",
		"data":    fmt.Sprintf("Round %d started! You have 30 seconds to submit a message.", h.CurrentRoundID),
	}

	h.BroadcastMessage(roundMessage)

	// Publish round start to NATS
	h.publishRoundStartToNATS()

	h.Logger.Infof("Round %d started", h.CurrentRoundID)

	// Start countdown
	go h.StartCountdown()
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
		"type":    "round_info",
		"data":    fmt.Sprintf("Round %d ended!", roundID),
	}

	h.BroadcastMessage(roundMessage)

	// Publish round end to NATS
	h.publishRoundEndToNATS(roundID)

	h.Logger.Infof("Round %d ended", roundID)

	// Select and announce winner (simplified random selection)
	go h.SelectWinner(roundID)
}

// StartCountdown sends countdown messages to clients.
func (h *Hub) StartCountdown() {
	for i := 10; i >= 1; i-- {
		h.Mu.Lock()
		roundActive := h.RoundActive
		h.Mu.Unlock()

		if !roundActive {
			return
		}

		countdownMessage := map[string]interface{}{
			"version": "1.0",
			"type":    "countdown",
			"data":    fmt.Sprintf("%d", i),
		}

		h.BroadcastMessage(countdownMessage)
		time.Sleep(1 * time.Second)
	}
}
