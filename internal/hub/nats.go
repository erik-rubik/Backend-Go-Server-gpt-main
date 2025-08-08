// internal/hub/nats.go
package hub

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"time"
)

// publishMessageToNATS serializes client message data (username, content, timestamp, round_id)
// into JSON and publishes it to a NATS JetStream subject.
// The subject is dynamically created based on the current round ID (e.g., "messages.ROUND_ID").
// Errors during marshaling or publishing are logged.
func (h *Hub) publishMessageToNATS(client *Client, content string) {
	if h.NatsConn != nil && h.Js != nil {
		messageData := map[string]any{
			"username":  client.Username,
			"content":   content,
			"timestamp": time.Now().Unix(),
			"round_id":  h.CurrentRoundID,
		}

		subject := fmt.Sprintf("messages.%d", h.CurrentRoundID)
		if data, err := json.Marshal(messageData); err == nil {
			if _, err := h.Js.Publish(subject, data); err != nil {
				h.Logger.Errorf("Failed to publish message to NATS: %v", err)
			}
		} else {
			h.Logger.Errorf("Failed to marshal message data: %v", err)
		}
	}
}

// publishRoundStartToNATS serializes round start event data (round_id, timestamp, status)
// into JSON and publishes it to a NATS JetStream subject.
// The subject is dynamically created based on the current round ID (e.g., "rounds.started.ROUND_ID").
// Errors during marshaling or publishing are logged.
func (h *Hub) publishRoundStartToNATS() {
	if h.NatsConn != nil && h.Js != nil {
		subject := fmt.Sprintf("rounds.started.%d", h.CurrentRoundID)
		roundData := map[string]any{
			"round_id":  h.CurrentRoundID,
			"timestamp": time.Now().Unix(),
			"status":    "started",
		}
		if data, err := json.Marshal(roundData); err == nil {
			if _, err := h.Js.Publish(subject, data); err != nil {
				h.Logger.Errorf("Failed to publish round start to NATS: %v", err)
			}
		} else {
			h.Logger.Errorf("Failed to marshal round start data: %v", err)
		}
	}
}

// publishRoundEndToNATS serializes round end event data (round_id, timestamp, status)
// into JSON and publishes it to a NATS JetStream subject.
// The subject is dynamically created based on the provided round ID (e.g., "rounds.ended.ROUND_ID").
// Errors during marshaling or publishing are logged.
func (h *Hub) publishRoundEndToNATS(roundID int64) {
	if h.NatsConn != nil && h.Js != nil {
		subject := fmt.Sprintf("rounds.ended.%d", roundID)
		roundData := map[string]any{
			"round_id":  roundID,
			"timestamp": time.Now().Unix(),
			"status":    "ended",
		}
		if data, err := json.Marshal(roundData); err == nil {
			if _, err := h.Js.Publish(subject, data); err != nil {
				h.Logger.Errorf("Failed to publish round end to NATS: %v", err)
			}
		} else {
			h.Logger.Errorf("Failed to marshal round end data: %v", err)
		}
	}
}

// SelectWinner selects and announces a winner from the round messages.
func (h *Hub) SelectWinner(roundID int64) {
	// Wait a moment for any final messages to be processed
	time.Sleep(500 * time.Millisecond)

	h.Mu.Lock()
	messages, exists := h.RoundMessages[roundID]
	if !exists || len(messages) == 0 {
		h.Mu.Unlock()
		h.Logger.Infof("No messages found for round %d, no winner selected", roundID)

		// Send "no winner" message
		noWinnerMessage := map[string]interface{}{
			"version":        "1.0",
			"type":           "winner_announcement",
			"round_id":       roundID,
			"winner":         nil,
			"total_messages": 0,
			"message":        "No messages submitted this round",
		}
		h.BroadcastMessage(noWinnerMessage)
		return
	}

	// Select random winner
	winnerIndex := rand.Intn(len(messages))
	winner := messages[winnerIndex]
	totalMessages := len(messages)
	h.Mu.Unlock()

	h.Logger.Infof("Selected winner for round %d: %s with message: %s", roundID, winner.Username, winner.Message)

	// Create winner announcement
	announcement := map[string]interface{}{
		"version":        "1.0",
		"type":           "winner_announcement",
		"round_id":       roundID,
		"winner":         winner,
		"total_messages": totalMessages,
	}

	// Broadcast winner announcement
	h.BroadcastMessage(announcement)

	// Publish winner to NATS
	winnerData := map[string]interface{}{
		"username":  winner.Username,
		"content":   winner.Message,
		"timestamp": winner.Timestamp.Unix(),
	}
	h.publishWinnerToNATS(roundID, winnerData)

	// Clean up old round messages (keep only last 3 rounds)
	h.cleanupOldMessages(roundID)
}

// publishWinnerToNATS serializes winner data (round_id, username, content, timestamp)
// into JSON and publishes it to a NATS JetStream subject.
// The subject is dynamically created based on the round ID (e.g., "winners.ROUND_ID").
// Errors during marshaling or publishing are logged.
func (h *Hub) publishWinnerToNATS(roundID int64, messageData map[string]interface{}) {
	if h.NatsConn != nil && h.Js != nil {
		winnerData := map[string]any{
			"round_id":  roundID,
			"username":  messageData["username"],
			"content":   messageData["content"],
			"timestamp": time.Now().Unix(),
		}

		winnerSubject := fmt.Sprintf("winners.%d", roundID)
		if data, err := json.Marshal(winnerData); err == nil {
			if _, err := h.Js.Publish(winnerSubject, data); err != nil {
				h.Logger.Errorf("Failed to publish winner to NATS: %v", err)
			}
		} else {
			h.Logger.Errorf("Failed to marshal winner data: %v", err)
		}
	}
}
