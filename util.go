// util.go
// Utility functions, such as loading logger configuration from a JSON file.
package main

import (
	"encoding/json"
	"os"

	"github.com/erilali/logger"
)

// loadLoggerConfig loads the logger configuration from a JSON file
func loadLoggerConfig(filePath string) (logger.LogConfig, error) {
	config := logger.DefaultLogConfig()
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return config, nil
		}
		return config, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return config, err
	}
	return config, nil
}
