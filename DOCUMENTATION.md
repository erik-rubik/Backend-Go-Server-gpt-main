# Backend Go Server Documentation

This document provides a high-level overview of the major components of the Backend Go Server application.

## Project Structure

The project is structured into several packages to separate concerns and improve modularity.

```
.
├── go.mod
├── go.sum
├── logger_config.json
├── main.go
├── MODULARIZATION_SUMMARY.md
└── internal/
    ├── api/
    │   └── api.go
    ├── hub/
    │   ├── client.go
    │   ├── hub.go
    │   ├── messaging.go
    │   ├── nats.go
    │   ├── rounds.go
    │   └── websocket.go
    ├── logger/
    │   └── logger.go
    ├── message/
    │   └── message.go
    └── util/
        └── util.go
```

## Core Components

### `main.go`

This is the entry point of the application. Its primary responsibilities are:

-   **Initialization**: It initializes the logger and the random number generator.
-   **Server Startup**: It starts the web server by calling `api.StartServer`.
-   **Dependency Injection**: It provides the `hub.NewHub` function to the API layer, allowing the API to create new Hub instances.

### `internal/api` package

This package is responsible for handling all HTTP requests and managing the connection to the NATS server.

-   **`api.go`**:
    -   **`StartServer`**: This function initializes the connection to NATS and JetStream, sets up the necessary streams, and starts the HTTP server.
    -   **HTTP Handlers**: It defines several HTTP handlers:
        -   `/ws`: Handles WebSocket connections by upgrading them and passing them to the Hub.
        -   `/api/rounds/{roundID}`: Provides an API endpoint to retrieve the history of messages and the winner for a specific round.
        -   `/health`: A health check endpoint that provides the status of the server and its connection to NATS.

### `internal/hub` package

The `hub` package is the central component for managing WebSocket clients, game rounds, and real-time messaging.

-   **`hub.go`**:
    -   **`Hub` struct**: This struct maintains the state of the application, including the list of connected clients, the current round status, and the connection to NATS.
    -   **`NewHub`**: A factory function to create a new `Hub`.
    -   **`Run`**: The main event loop for the hub. It handles client registration, unregistration, and broadcasting messages to clients.

-   **`client.go`**: Defines the `Client` struct, which represents a single WebSocket client connected to the server.

-   **`websocket.go`**: Contains the logic for handling WebSocket connections, including reading messages from clients and writing messages to them.

-   **`rounds.go`**: Manages the game round logic, including starting and ending rounds, and selecting a winner.

-   **`messaging.go`**: Handles the processing of incoming messages from clients.

-   **`nats.go`**: Contains functions for publishing messages to NATS subjects.

### `internal/logger` package

This package provides a configurable logger for the application.

-   **`logger.go`**: Defines a `Logger` struct and functions for initializing and using the logger. It can be configured to log to the console or a file, and to format logs as JSON.

### `internal/message` package

This package defines the structure of messages exchanged between the clients and the server.

-   **`message.go`**: Defines the `Message` struct.

### `internal/util` package

This package contains utility functions used throughout the application.

-   **`util.go`**:
    -   **`LoadLoggerConfig`**: Loads the logger configuration from a JSON file.
    -   **`GenerateUsername`**: Generates a random username for a new client.
