// main.go
// Application entry point: initializes logger, seeds random, and starts the server.
package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/erilali/logger"
)

// Global logger for non-hub components
var serverLogger *logger.Logger

func main() {
	// Load logger configuration
	config, err := loadLoggerConfig("logger_config.json")
	if err != nil {
		fmt.Printf("Error loading logger config: %v, using defaults\n", err)
	}
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

	rand.Seed(time.Now().UnixNano())

	// Call server setup and start (move all setup logic to api.go or a new setup.go if needed)
	StartServer()
}
