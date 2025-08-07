# Dockerfile for the Go application

# Build stage
FROM golang:1.23.4-alpine AS builder

# Set the working directory
WORKDIR /app

# Copy the Go module files
COPY go.mod go.sum ./

# Download the dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# Final stage
FROM scratch

# Copy the built binary from the builder stage
COPY --from=builder /app/main .

# Copy the logger configuration and test UI
COPY logger_config.json .
COPY test-ui.html .

# Expose port 8080
EXPOSE 8080

# Run the application
CMD ["/main"]
