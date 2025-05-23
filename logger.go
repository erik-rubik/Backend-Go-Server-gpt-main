// logger.go
// Implements logging configuration and initialization using zerolog and lumberjack for log rotation and formatting.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/natefinch/lumberjack.v2"
)

// LogConfig holds configuration for the logger
type LogConfig struct {
	Level      string // debug, info, warn, error, fatal
	LogToFile  bool
	LogToJSON  bool
	FilePath   string
	MaxSize    int  // megabytes
	MaxBackups int  // number of backups
	MaxAge     int  // days
	Compress   bool // compress old log files
}

// DefaultLogConfig returns a default logging configuration
func DefaultLogConfig() LogConfig {
	return LogConfig{
		Level:      "info",
		LogToFile:  true,
		LogToJSON:  true,
		FilePath:   "server.log",
		MaxSize:    10, // 10 MB
		MaxBackups: 5,  // 5 backups
		MaxAge:     30, // 30 days
		Compress:   true,
	}
}

// InitLogger initializes the zerolog logger with the given configuration
func InitLogger(config LogConfig) {
	// Set time format for logs
	zerolog.TimeFieldFormat = time.RFC3339

	// Parse log level
	level, err := zerolog.ParseLevel(config.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	// Configure outputs
	var writers []io.Writer

	// Console writer
	if !config.LogToJSON {
		// Use enhanced console writer for more readable output
		consoleWriter := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "15:04:05", // Shorter time format for console
			NoColor:    false,
			PartsOrder: []string{
				zerolog.TimestampFieldName,
				zerolog.LevelFieldName,
				"component",
				zerolog.MessageFieldName,
			},
			FieldsExclude: []string{"component"},
			FormatLevel: func(i interface{}) string {
				level := strings.ToUpper(fmt.Sprintf("%s", i))
				switch level {
				case "DEBUG":
					return "\033[36m[ " + fmt.Sprintf("%-5s", level) + " ]\033[0m" // Cyan
				case "INFO":
					return "\033[32m[ " + fmt.Sprintf("%-5s", level) + " ]\033[0m" // Green
				case "WARN":
					return "\033[33m[ " + fmt.Sprintf("%-5s", level) + " ]\033[0m" // Yellow
				case "ERROR":
					return "\033[31m[ " + fmt.Sprintf("%-5s", level) + " ]\033[0m" // Red
				case "FATAL":
					return "\033[35m[ " + fmt.Sprintf("%-5s", level) + " ]\033[0m" // Magenta
				default:
					return "\033[37m[ " + fmt.Sprintf("%-5s", level) + " ]\033[0m" // White
				}
			},
			FormatTimestamp: func(i interface{}) string {
				return fmt.Sprintf("\033[90m%s\033[0m", i) // Gray timestamp
			},
			FormatMessage: func(i interface{}) string {
				return fmt.Sprintf("\033[1m%s\033[0m", i) // Bold message
			},
			FormatFieldName: func(i interface{}) string {
				return fmt.Sprintf("\033[34m%s\033[0m: ", i) // Blue field names with space
			},
			FormatFieldValue: func(i interface{}) string {
				return fmt.Sprintf("\033[37m%s\033[0m", i) // White field values
			},
			FormatErrFieldName: func(i interface{}) string {
				return fmt.Sprintf("\033[31m%s\033[0m: ", i) // Red error field name
			},
			FormatErrFieldValue: func(i interface{}) string {
				return fmt.Sprintf("\033[31m%s\033[0m", i) // Red error value
			},
		}
		writers = append(writers, consoleWriter)
	} else {
		// Use standard JSON output to console
		writers = append(writers, os.Stdout)
	}

	// File writer with rotation if enabled
	if config.LogToFile && config.FilePath != "" {
		fileWriter := &lumberjack.Logger{
			Filename:   config.FilePath,
			MaxSize:    config.MaxSize,
			MaxBackups: config.MaxBackups,
			MaxAge:     config.MaxAge,
			Compress:   config.Compress,
		}
		writers = append(writers, fileWriter)
	}

	// Create multi-writer if needed
	var output io.Writer
	if len(writers) > 1 {
		output = io.MultiWriter(writers...)
	} else {
		output = writers[0]
	}

	// Set global logger
	log.Logger = zerolog.New(output).With().Timestamp().Logger()

	// No need for special component handling as we've already included it in PartsOrder
}

// Logger is a wrapper around zerolog.Logger that provides additional context
type Logger struct {
	logger zerolog.Logger
}

// NewLogger creates a new logger with the given component name
func NewLogger(component string) *Logger {
	return &Logger{
		logger: log.With().Str("component", component).Logger(),
	}
}

// WithField adds a field to the logger
func (l *Logger) WithField(key string, value interface{}) *Logger {
	return &Logger{
		logger: l.logger.With().Interface(key, value).Logger(),
	}
}

// WithFields adds multiple fields to the logger
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	ctx := l.logger.With()
	for k, v := range fields {
		ctx = ctx.Interface(k, v)
	}
	return &Logger{
		logger: ctx.Logger(),
	}
}

// Debug logs a debug message
func (l *Logger) Debug(msg string) {
	l.logger.Debug().Msg(msg)
}

// Debugf logs a formatted debug message
func (l *Logger) Debugf(format string, v ...interface{}) {
	l.logger.Debug().Msgf(format, v...)
}

// Info logs an info message
func (l *Logger) Info(msg string) {
	l.logger.Info().Msg(msg)
}

// Infof logs a formatted info message
func (l *Logger) Infof(format string, v ...interface{}) {
	l.logger.Info().Msgf(format, v...)
}

// Warn logs a warning message
func (l *Logger) Warn(msg string) {
	l.logger.Warn().Msg(msg)
}

// Warnf logs a formatted warning message
func (l *Logger) Warnf(format string, v ...interface{}) {
	l.logger.Warn().Msgf(format, v...)
}

// Error logs an error message
func (l *Logger) Error(msg string) {
	l.logger.Error().Msg(msg)
}

// Errorf logs a formatted error message
func (l *Logger) Errorf(format string, v ...interface{}) {
	l.logger.Error().Msgf(format, v...)
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(msg string) {
	l.logger.Fatal().Msg(msg)
}

// Fatalf logs a formatted fatal message and exits
func (l *Logger) Fatalf(format string, v ...interface{}) {
	l.logger.Fatal().Msgf(format, v...)
}

// LogEvent logs an event with context
func (l *Logger) LogEvent(level string, event string, username string, detail string) {
	// Create a meaningful message - keep it simple for routine events
	var message string

	// Special case handling for different event types
	switch event {
	case "round_started":
		// Just show "Round X" in yellow for round start
		roundNum := extractRoundNumber(detail)
		if roundNum != "" {
			message = fmt.Sprintf("Round \033[93m%s\033[0m started", roundNum)
		} else {
			message = "New round started"
		}

	case "round_ended":
		// Just show "Round X ended" in yellow for round end
		roundNum := extractRoundNumber(detail)
		if roundNum != "" {
			message = fmt.Sprintf("Round \033[93m%s\033[0m ended", roundNum)
		} else {
			message = "Round ended"
		}

	case "client_connected":
		// Just show username for connections
		if username != "" {
			message = fmt.Sprintf("\033[96m%s\033[0m connected", username)
		} else {
			message = "User connected"
		}

	case "client_disconnected":
		// Just show username for disconnections
		if username != "" {
			message = fmt.Sprintf("\033[96m%s\033[0m disconnected", username)
		} else {
			message = "User disconnected"
		}

	case "message_received":
		// For regular messages, format them concisely
		if username != "" {
			if detail != "" {
				message = fmt.Sprintf("\033[95m%s\033[0m: \033[97m%s\033[0m", username, detail)
			} else {
				message = fmt.Sprintf("Message from \033[95m%s\033[0m", username)
			}
		} else {
			message = "Message received"
		}

	case "round_message_selected":
		// For selected messages, give more context
		if username != "" {
			if detail != "" {
				message = fmt.Sprintf("Selected: \033[95m%s\033[0m: \033[97m%s\033[0m", username, detail)
			} else {
				message = fmt.Sprintf("Selected message from \033[95m%s\033[0m", username)
			}
		} else if detail == "No valid messages" {
			// Simplify the no messages case
			message = "No messages this round"
		} else {
			message = "Message selected"
		}

	case "read_error":
		// Full context for errors
		evt := l.logger.With().Str("event", event)
		if username != "" {
			evt = evt.Str("username", username)
		}
		if detail != "" {
			evt = evt.Str("detail", detail)
			message = fmt.Sprintf("ERROR: \033[31m%s\033[0m", detail)
		} else {
			message = "ERROR: Read error occurred"
		}
		logger := evt.Logger()
		logger.Error().Msg(message)
		return

	default:
		// For other cases where full context might be important
		evt := l.logger.With().Str("event", event)
		if username != "" {
			evt = evt.Str("username", username)
		}
		if detail != "" {
			evt = evt.Str("detail", detail)
			message = fmt.Sprintf("%s: %s",
				strings.ReplaceAll(event, "_", " "), detail)
		} else {
			message = strings.ReplaceAll(event, "_", " ")
		}

		// Log with full context
		logger := evt.Logger()
		switch level {
		case "debug":
			logger.Debug().Msg(message)
		case "info":
			logger.Info().Msg(message)
		case "warn":
			logger.Warn().Msg(message)
		case "error":
			logger.Error().Msg(message)
		case "fatal":
			logger.Fatal().Msg(message)
		default:
			logger.Info().Msg(message)
		}
		return
	}

	// For routine events, log with simplified context
	logger := l.logger
	switch level {
	case "debug":
		logger.Debug().Msg(message)
	case "info":
		logger.Info().Msg(message)
	case "warn":
		logger.Warn().Msg(message)
	case "error":
		logger.Error().Msg(message)
	case "fatal":
		logger.Fatal().Msg(message)
	default:
		logger.Info().Msg(message)
	}
}

// extractRoundNumber extracts round number from details string
func extractRoundNumber(detail string) string {
	var roundNum string
	_, err := fmt.Sscanf(detail, "Round %s started", &roundNum)
	if err != nil {
		_, err = fmt.Sscanf(detail, "Round %s ended", &roundNum)
		if err != nil {
			return ""
		}
	}
	return roundNum
}
