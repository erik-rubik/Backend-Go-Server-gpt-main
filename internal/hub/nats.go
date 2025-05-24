// internal/hub/nats.go
package hub

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/nats-io/nats.go"
)

// publishMessageToNATS publishes a client message to NATS
func (h *Hub) publishMessageToNATS(client *Client, content string) {
	if h.NatsConn != nil && h.Js != nil {
		messageData := map[string]interface{}{
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

// publishRoundStartToNATS publishes round start event to NATS
func (h *Hub) publishRoundStartToNATS() {
	if h.NatsConn != nil && h.Js != nil {
		subject := fmt.Sprintf("rounds.started.%d", h.CurrentRoundID)
		roundData := map[string]interface{}{
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

// publishRoundEndToNATS publishes round end event to NATS
func (h *Hub) publishRoundEndToNATS(roundID int64) {
	if h.NatsConn != nil && h.Js != nil {
		subject := fmt.Sprintf("rounds.ended.%d", roundID)
		roundData := map[string]interface{}{
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

// SelectWinner selects a random winner from the submitted messages.
func (h *Hub) SelectWinner(roundID int64) {
	// Give some time for processing
	time.Sleep(2 * time.Second)

	// Fetch messages from NATS to select a winner
	if h.NatsConn != nil && h.Js != nil {
		subject := fmt.Sprintf("messages.%d", roundID)
		consumerName := fmt.Sprintf("WINNER_SELECTOR_%d", time.Now().UnixNano())

		// Create a consumer for this round's messages
		_, err := h.Js.AddConsumer("MESSAGES", &nats.ConsumerConfig{
			Name:          consumerName,
			DeliverPolicy: nats.DeliverAllPolicy,
			AckPolicy:     nats.AckExplicitPolicy,
			FilterSubject: subject,
			MaxDeliver:    1,
		})
		if err != nil {
			h.Logger.Errorf("Error creating winner selection consumer: %v", err)
			return
		}

		// Subscribe and fetch messages
		sub, err := h.Js.PullSubscribe(subject, consumerName)
		if err != nil {
			h.Logger.Errorf("Error subscribing for winner selection: %v", err)
			h.Js.DeleteConsumer("MESSAGES", consumerName)
			return
		}

		// Fetch all messages for this round
		msgs, err := sub.Fetch(100, nats.MaxWait(2*time.Second))
		sub.Unsubscribe()
		h.Js.DeleteConsumer("MESSAGES", consumerName)

		if err != nil && err != nats.ErrTimeout {
			h.Logger.Errorf("Error fetching messages for winner selection: %v", err)
			return
		}

		if len(msgs) == 0 {
			h.Logger.Infof("No messages found for round %d", roundID)
			winnerMessage := map[string]interface{}{
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

		var messageData map[string]interface{}
		if err := json.Unmarshal(selectedMsg.Data, &messageData); err != nil {
			h.Logger.Errorf("Error unmarshaling selected message: %v", err)
			return
		}

		// Acknowledge the selected message
		selectedMsg.Ack()

		// Acknowledge remaining messages
		for i, msg := range msgs {
			if i != selectedIdx {
				msg.Ack()
			}
		}

		// Store winner data in NATS
		h.publishWinnerToNATS(roundID, messageData)

		// Broadcast winner announcement
		winnerMessage := map[string]interface{}{
			"version": "1.0",
			"type":    "selected_text",
			"data":    fmt.Sprintf("Winner: %s - %s", messageData["username"], messageData["content"]),
		}
		h.BroadcastMessage(winnerMessage)

		h.Logger.Infof("Selected winner for round %d: %s", roundID, messageData["username"])
	} else {
		// Fallback if NATS is not available
		winnerMessage := map[string]interface{}{
			"version": "1.0",
			"type":    "selected_text",
			"data":    "Random winner selected for the round!",
		}
		h.BroadcastMessage(winnerMessage)
	}
}

// publishWinnerToNATS publishes winner data to NATS
func (h *Hub) publishWinnerToNATS(roundID int64, messageData map[string]interface{}) {
	winnerData := map[string]interface{}{
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
