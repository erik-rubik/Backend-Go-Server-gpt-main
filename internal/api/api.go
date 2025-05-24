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

// StartServer starts the websocket and HTTP server.
func StartServer(serverLogger *logger.Logger, hubFactory func(*nats.Conn, nats.JetStreamContext, *logger.Logger) interface{}) {
	// Connect to NATS using environment variable or default URL
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}

	serverLogger.Infof("Connecting to NATS at %s", natsURL)
	nc, err := nats.Connect(natsURL)
	if err != nil {
		serverLogger.Errorf("Error connecting to NATS: %v", err)
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

			// Set up JetStream streams with 30-minute retention
			retention := 30 * time.Minute
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
					MaxAge:   retention,
				}
				_, err := js.StreamInfo(streamConfig.Name)
				if err != nil {
					_, err = js.AddStream(streamConfig)
					if err != nil {
						serverLogger.Errorf("Error creating stream %s: %v", s.Name, err)
					} else {
						serverLogger.Infof("Created stream: %s", s.Name)
					}
				} else {
					_, err = js.UpdateStream(streamConfig)
					if err != nil {
						serverLogger.Errorf("Error updating stream %s: %v", s.Name, err)
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
		consumerName := fmt.Sprintf("API_CONSUMER_%d", time.Now().UnixNano())
		_, err := js.AddConsumer("MESSAGES", &nats.ConsumerConfig{
			Name:          consumerName,
			DeliverPolicy: nats.DeliverAllPolicy,
			AckPolicy:     nats.AckExplicitPolicy, FilterSubject: subject,
			MaxDeliver: 1,
		})
		if err != nil {
			serverLogger.Errorf("Error creating consumer: %v", err)
			http.Error(w, "Error retrieving messages", http.StatusInternalServerError)
			return
		}
		sub, err := js.PullSubscribe(subject, consumerName)
		if err != nil {
			serverLogger.Errorf("Error subscribing: %v", err)
			js.DeleteConsumer("MESSAGES", consumerName)
			http.Error(w, "Error retrieving messages", http.StatusInternalServerError)
			return
		}

		// Ensure cleanup happens even if other operations fail
		defer func() {
			if unsubErr := sub.Unsubscribe(); unsubErr != nil {
				serverLogger.Errorf("Error unsubscribing: %v", unsubErr)
			}
			if delErr := js.DeleteConsumer("MESSAGES", consumerName); delErr != nil {
				serverLogger.Errorf("Error deleting consumer %s: %v", consumerName, delErr)
			}
		}()

		msgs, err := sub.Fetch(100, nats.MaxWait(2*time.Second))
		if err != nil && err != nats.ErrTimeout {
			serverLogger.Errorf("Error fetching messages: %v", err)
			http.Error(w, "Error retrieving messages", http.StatusInternalServerError)
			return
		}
		var messages []map[string]interface{}
		for _, msg := range msgs {
			var message map[string]interface{}
			if err := json.Unmarshal(msg.Data, &message); err != nil {
				serverLogger.Errorf("Error unmarshaling message: %v", err)
				continue
			}
			messages = append(messages, message)
			msg.Ack()
		}
		var winner map[string]interface{}
		winnerSub, err := js.PullSubscribe(fmt.Sprintf("winners.%s", roundID), "")
		if err == nil {
			winnerMsgs, err := winnerSub.Fetch(1, nats.MaxWait(1*time.Second))
			if err == nil && len(winnerMsgs) > 0 {
				var winnerMsg map[string]interface{}
				if err := json.Unmarshal(winnerMsgs[0].Data, &winnerMsg); err == nil {
					winner = winnerMsg
				}
				winnerMsgs[0].Ack()
			}
			winnerSub.Unsubscribe()
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

	fs := http.FileServer(http.Dir("frontend"))
	http.Handle("/", fs)

	addr := ":8080"
	serverLogger.Infof("Server started at %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		serverLogger.Fatalf("ListenAndServe: %v", err)
	}
}
