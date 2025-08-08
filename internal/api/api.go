// internal/api/api.go
// Provides StartServer and HTTP API logic as a package.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/erilali/internal/logger"
	"github.com/nats-io/nats.go"
)

const (
	defaultNatsURL          = nats.DefaultURL
	jetstreamRetention      = 30 * time.Minute
	apiConsumerMaxDeliver   = 1
	apiConsumerFetchMaxWait = 2 * time.Second
	winnerAPIFetchMaxWait   = 1 * time.Second
	apiConsumerPrefix       = "API_CONSUMER_"
)

// StartServer starts the websocket and HTTP server.
func StartServer(serverLogger *logger.Logger, hubFactory func(*nats.Conn, nats.JetStreamContext, *logger.Logger) interface{}) {
	// Connect to NATS using environment variable or default URL
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = defaultNatsURL
	}

	serverLogger.Infof("Connecting to NATS at %s", natsURL)
	nc, err := nats.Connect(natsURL)
	if err != nil {
		serverLogger.Errorf("Error connecting to NATS: %v", err) // Wrapped error
		nc = nil
		serverLogger.Warn("Running without NATS connection. Message persistence will be disabled.")
	} else {
		serverLogger.Info("Successfully connected to NATS")
	}

	// Set up JetStream if NATS is connected
	var js nats.JetStreamContext
	if nc != nil {
		jsContext, err := nc.JetStream()
		if err != nil {
			serverLogger.Errorf("Error getting JetStream context: %v", err)
			serverLogger.Warn("Running without JetStream. Message persistence will be disabled.")
		} else {
			js = jsContext
			serverLogger.Info("Successfully connected to JetStream")

			// Set up JetStream streams with configurable retention
			streams := []struct {
				Name     string
				Subjects []string
			}{
				{Name: "ROUNDS", Subjects: []string{"rounds.started.*", "rounds.ended.*"}},
				{Name: "MESSAGES", Subjects: []string{"messages.*"}},
				{Name: "WINNERS", Subjects: []string{"winners.*"}},
			}
			for _, s := range streams {
				streamConfig := &nats.StreamConfig{
					Name:     s.Name,
					Subjects: s.Subjects,
					Storage:  nats.FileStorage,
					MaxAge:   jetstreamRetention,
				}
				_, err := js.StreamInfo(streamConfig.Name)
				if err != nil {
					_, err = js.AddStream(streamConfig)
					if err != nil {
						serverLogger.Errorf("Error creating stream %s: %v", s.Name, err) // Wrapped error
					} else {
						serverLogger.Infof("Created stream: %s", s.Name)
					}
				} else {
					_, err = js.UpdateStream(streamConfig)
					if err != nil {
						serverLogger.Errorf("Error updating stream %s: %v", s.Name, err) // Wrapped error
					} else {
						serverLogger.Infof("Updated stream: %s", s.Name)
					}
				}
			}
		}
	}
	hub := hubFactory(nc, js, serverLogger)

	// Validate that hub implements required interfaces
	hubRunner, ok := hub.(interface{ Run() })
	if !ok {
		serverLogger.Fatal("Hub does not implement Run() method")
	}

	hubServer, ok := hub.(interface {
		ServeWs(http.ResponseWriter, *http.Request)
	})
	if !ok {
		serverLogger.Fatal("Hub does not implement ServeWs method")
	}

	go hubRunner.Run()

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		hubServer.ServeWs(w, r)
	})

	// Serve the test UI
	// Serve the UI at root and /ui for convenience
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			http.ServeFile(w, r, "test-ui.html")
			return
		}
		http.NotFound(w, r)
	})
	http.HandleFunc("/ui", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "test-ui.html")
	})

	http.HandleFunc("/api/rounds/", func(w http.ResponseWriter, r *http.Request) {
		if js == nil {
			http.Error(w, "JetStream not available", http.StatusServiceUnavailable)
			return
		}
		path := r.URL.Path
		if len(path) <= len("/api/rounds/") {
			http.Error(w, "Round ID required", http.StatusBadRequest)
			return
		}
		roundID := path[len("/api/rounds/"):]
		subject := fmt.Sprintf("messages.%s", roundID)

		// Use a more descriptive and potentially durable consumer name if appropriate
		// For now, keeping it dynamic but with a clear prefix and ensuring it's cleaned up.
		consumerName := fmt.Sprintf("%s%s_%d", apiConsumerPrefix, roundID, time.Now().UnixNano())

		_, err := js.AddConsumer("MESSAGES", &nats.ConsumerConfig{
			Name:          consumerName,
			DeliverPolicy: nats.DeliverAllPolicy,
			AckPolicy:     nats.AckExplicitPolicy,
			FilterSubject: subject,
			MaxDeliver:    apiConsumerMaxDeliver,
		})
		if err != nil {
			serverLogger.Errorf("Error creating consumer %s for subject %s: %v", consumerName, subject, err) // Wrapped error
			http.Error(w, "Error retrieving messages", http.StatusInternalServerError)
			return
		}
		sub, err := js.PullSubscribe(subject, consumerName) // Using the created consumer name
		if err != nil {
			serverLogger.Errorf("Error subscribing with consumer %s to subject %s: %v", consumerName, subject, err) // Wrapped error
			js.DeleteConsumer("MESSAGES", consumerName)                                                             // Attempt cleanup
			http.Error(w, "Error retrieving messages", http.StatusInternalServerError)
			return
		}

		// Ensure cleanup happens even if other operations fail
		defer func() {
			if unsubErr := sub.Unsubscribe(); unsubErr != nil {
				serverLogger.Errorf("Error unsubscribing consumer %s: %v", consumerName, unsubErr) // Wrapped error
			}
			if delErr := js.DeleteConsumer("MESSAGES", consumerName); delErr != nil {
				serverLogger.Errorf("Error deleting consumer %s: %v", consumerName, delErr) // Wrapped error
			}
		}()

		msgs, err := sub.Fetch(100, nats.MaxWait(apiConsumerFetchMaxWait)) // Use constant
		if err != nil && err != nats.ErrTimeout {
			serverLogger.Errorf("Error fetching messages with consumer %s: %v", consumerName, err) // Wrapped error
			http.Error(w, "Error retrieving messages", http.StatusInternalServerError)
			return
		}
		var messages []map[string]interface{}
		for _, msg := range msgs {
			var message map[string]interface{}
			if err := json.Unmarshal(msg.Data, &message); err != nil {
				serverLogger.Errorf("Error unmarshaling message: %v", err) // Wrapped error
				continue
			}
			messages = append(messages, message)
			msg.Ack() // Ack individual messages as they are processed
		}

		var winner map[string]interface{}
		// For fetching winner, using an ephemeral pull subscriber is generally fine if we only need the latest.
		// If multiple API calls could happen concurrently for the same round before a winner is published,
		// and each needs to see the winner, this approach is okay.
		// If a durable view of the winner is needed across multiple API calls even if they are spaced out,
		// a named consumer for winners might be considered, but for now, this is simpler.
		winnerSubject := fmt.Sprintf("winners.%s", roundID)
		winnerConsumerName := fmt.Sprintf("API_WINNER_CONSUMER_%s_%d", roundID, time.Now().UnixNano())

		// Create a consumer for the winner message
		_, err = js.AddConsumer("WINNERS", &nats.ConsumerConfig{
			Name:          winnerConsumerName,
			DeliverPolicy: nats.DeliverAllPolicy, // Or DeliverLastPolicy if only the most recent winner matters
			AckPolicy:     nats.AckExplicitPolicy,
			FilterSubject: winnerSubject,
			MaxDeliver:    1, // Only attempt to deliver once to this ephemeral consumer
		})

		if err != nil {
			serverLogger.Warnf("Error creating winner consumer %s for subject %s: %v. Winner might not be retrieved.", winnerConsumerName, winnerSubject, err)
		} else {
			defer js.DeleteConsumer("WINNERS", winnerConsumerName) // Cleanup winner consumer

			winnerSub, err := js.PullSubscribe(winnerSubject, winnerConsumerName)
			if err != nil {
				serverLogger.Warnf("Error subscribing for winner with consumer %s: %v. Winner might not be retrieved.", winnerConsumerName, err)
			} else {
				defer winnerSub.Unsubscribe()
				winnerMsgs, fetchErr := winnerSub.Fetch(1, nats.MaxWait(winnerAPIFetchMaxWait)) // Use constant
				if fetchErr != nil && fetchErr != nats.ErrTimeout {
					serverLogger.Warnf("Error fetching winner message with consumer %s: %v", winnerConsumerName, fetchErr)
				} else if len(winnerMsgs) > 0 {
					var winnerMsg map[string]interface{}
					if unmarshalErr := json.Unmarshal(winnerMsgs[0].Data, &winnerMsg); unmarshalErr == nil {
						winner = winnerMsg
						winnerMsgs[0].Ack() // Ack the winner message
					} else {
						serverLogger.Errorf("Error unmarshaling winner message: %v", unmarshalErr)
					}
				}
			}
		}

		response := map[string]interface{}{
			"round_id":  roundID,
			"messages":  messages,
			"winner":    winner,
			"count":     len(messages),
			"timestamp": time.Now(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		natsStatus := "disconnected"
		if nc != nil && nc.Status() == nats.CONNECTED {
			natsStatus = "connected"
		}
		health := map[string]interface{}{
			"status":  "ok",
			"nats":    natsStatus,
			"version": "1.0.0",
		}
		if js != nil {
			jsInfo := make(map[string]interface{})
			streams := []string{"ROUNDS", "MESSAGES", "WINNERS"}
			streamInfo := make(map[string]interface{})
			for _, streamName := range streams {
				info, err := js.StreamInfo(streamName)
				if err == nil {
					streamInfo[streamName] = map[string]interface{}{
						"messages":  info.State.Msgs,
						"bytes":     info.State.Bytes,
						"subjects":  info.Config.Subjects,
						"retention": fmt.Sprintf("%v", info.Config.MaxAge),
					}
				} else {
					streamInfo[streamName] = map[string]interface{}{
						"error": err.Error(),
					}
				}
			}
			jsInfo["streams"] = streamInfo
			health["jetstream"] = jsInfo
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(health)
	})

	addr := ":8080"
	serverLogger.Infof("Server started at %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		serverLogger.Fatalf("ListenAndServe: %v", err)
	}
}
