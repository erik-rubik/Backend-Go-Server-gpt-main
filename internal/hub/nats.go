// internal/hub/nats.go
package hub

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	winnerSelectionDelay         = 1 * time.Second // Reduced delay, assuming NATS persistence is fast
	maxMessagesToFetchForWinner  = 200             // Increased limit for fetching messages for winner selection
	winnerSelectorConsumerPrefix = "WINNER_SELECTOR_"
	winnerFetchMaxWait           = 2 * time.Second
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

// SelectWinner orchestrates the process of selecting a winner for a given round.
// It introduces a small delay for NATS processing, then fetches messages for the round from NATS JetStream
// using a temporary, uniquely named consumer.
// If messages are found, it randomly selects one, acknowledges all fetched messages,
// publishes the winner information to a NATS subject ("winners.ROUND_ID"),
// and broadcasts the winner announcement to all connected clients.
// If no messages are found, or if NATS is unavailable, appropriate fallback messages are broadcast.
// The temporary consumer is cleaned up afterwards.
func (h *Hub) SelectWinner(roundID int64) {
	// Give a very short time for NATS to process, if necessary.
	// Ideally, NATS operations are fast enough that this might not be strictly needed,
	// but it can prevent race conditions in some edge cases.
	time.Sleep(winnerSelectionDelay)

	// Fetch messages from NATS to select a winner
	if h.NatsConn != nil && h.Js != nil {
		subject := fmt.Sprintf("messages.%d", roundID)
		consumerName := fmt.Sprintf("%s%d_%d", winnerSelectorConsumerPrefix, roundID, time.Now().UnixNano())

		// Create a consumer for this round's messages
		_, err := h.Js.AddConsumer("MESSAGES", &nats.ConsumerConfig{
			Name:          consumerName,
			DeliverPolicy: nats.DeliverAllPolicy,
			AckPolicy:     nats.AckExplicitPolicy,
			FilterSubject: subject,
			MaxDeliver:    1, // We'll process these messages once here for winner selection
		})
		if err != nil {
			h.Logger.Errorf("Error creating winner selection consumer %s: %v", consumerName, err)
			return
		}

		// Subscribe and fetch messages
		sub, err := h.Js.PullSubscribe(subject, consumerName)
		if err != nil {
			h.Logger.Errorf("Error subscribing for winner selection with consumer %s: %v", consumerName, err)
			h.Js.DeleteConsumer("MESSAGES", consumerName) // Attempt cleanup
			return
		}

		// It's important to unsubscribe and delete the consumer.
		defer func() {
			if unsubErr := sub.Unsubscribe(); unsubErr != nil {
				h.Logger.Warnf("Error unsubscribing winner selector %s: %v", consumerName, unsubErr)
			}
			if delErr := h.Js.DeleteConsumer("MESSAGES", consumerName); delErr != nil {
				h.Logger.Warnf("Error deleting winner selector consumer %s: %v", consumerName, delErr)
			}
		}()

		// Fetch messages for this round. Consider fetching in batches if rounds can have extremely large numbers of messages.
		// For now, fetching up to maxMessagesToFetchForWinner.
		msgs, err := sub.Fetch(maxMessagesToFetchForWinner, nats.MaxWait(winnerFetchMaxWait))

		if err != nil && err != nats.ErrTimeout {
			h.Logger.Errorf("Error fetching messages for winner selection with consumer %s: %v", consumerName, err)
			return
		}

		if len(msgs) == 0 {
			h.Logger.Infof("No messages found for round %d", roundID)
			winnerMessage := map[string]any{
				"version": "1.0",
				"type":    "selected_text",
				"data":    "No messages submitted for this round.",
			}
			h.BroadcastMessage(winnerMessage)
			return
		}

		// Select a random winner
		selectedIdx := rand.Intn(len(msgs))
		selectedMsg := msgs[selectedIdx]

		var messageData map[string]any
		if err := json.Unmarshal(selectedMsg.Data, &messageData); err != nil {
			h.Logger.Errorf("Error unmarshaling selected message: %v", err)
			return
		}

		// Acknowledge all fetched messages so NATS doesn't redeliver them
		for _, msg := range msgs {
			msg.Ack()
		}

		// Store winner data in NATS and broadcast to clients
		h.publishWinnerToNATS(roundID, messageData)

		// Broadcast winner announcement
		winnerMessage := map[string]any{
			"version": "1.0",
			"type":    "selected_text",
			"data":    fmt.Sprintf("Winner: %s - %s", messageData["username"], messageData["content"]),
		}
		h.BroadcastMessage(winnerMessage)

		h.Logger.Infof("Selected winner for round %d: %s", roundID, messageData["username"])
	} else {
		// Fallback if NATS is not available
		winnerMessage := map[string]any{
			"version": "1.0",
			"type":    "selected_text",
			"data":    "Random winner selected for the round!",
		}
		h.BroadcastMessage(winnerMessage)
	}
}

// publishWinnerToNATS serializes winner data (round_id, username, content, timestamp)
// into JSON and publishes it to a NATS JetStream subject.
// The subject is dynamically created based on the round ID (e.g., "winners.ROUND_ID").
// Errors during marshaling or publishing are logged.
func (h *Hub) publishWinnerToNATS(roundID int64, messageData map[string]any) {
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
