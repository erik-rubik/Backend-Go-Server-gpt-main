// main.go
// Application entry point: initializes logger, seeds random, and starts the server.
package main

import (
	"fmt"

	"github.com/erilali/internal/api"
	"github.com/erilali/internal/hub"
	"github.com/erilali/internal/logger"
	"github.com/erilali/internal/util"
	"github.com/nats-io/nats.go"
)

// Global logger for non-hub components
var serverLogger *logger.Logger

func main() {
	// Load logger configuration
	config, err := util.LoadLoggerConfig("logger_config.json")
	if err != nil {
		fmt.Printf("Error loading logger config: %v, using defaults\n", err)
	}

	// Override specific settings for development (remove these lines for production)
	config.LogToJSON = false
	config.LogToFile = false

	logger.InitLogger(config)
	serverLogger = logger.NewLogger("server")
	serverLogger.Info("Logger initialized with configuration")
	serverLogger.WithFields(map[string]interface{}{
		"level":       config.Level,
		"log_to_file": config.LogToFile,
		"log_to_json": config.LogToJSON,
		"file_path":   config.FilePath,
	}).Info("Logger configuration details")

	// In Go 1.20+, the global random number generator in the math/rand package is
	// automatically seeded. Explicit seeding is no longer necessary for most use cases.

	// Use the new modularized API and Hub packages
	api.StartServer(serverLogger, func(nc *nats.Conn, js nats.JetStreamContext, logger *logger.Logger) interface{} {
		return hub.NewHub(nc, js, logger)
	})
}
