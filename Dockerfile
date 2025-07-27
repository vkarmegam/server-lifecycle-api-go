# Stage 1: Build the Go application
FROM golang:1.24-alpine AS builder
 # Changed from 1.22-alpine to 1.23-alpine

# Set necessary environment variables for CGO if you use it (e.g., for race detector)
ENV CGO_ENABLED=1
ENV GOOS=linux

# Install build dependencies, including gcc for CGO
RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# Copy go.mod and go.sum first to leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application source code
COPY . .

# Generate sqlc code (if not already committed)
# RUN sqlc generate

# Generate Swagger documentation (if not already committed)
# RUN swag init --parseDependency --parseInternal

# Build the application
# Use -a -installsuffix cgo for static linking with CGO
RUN go build -o /go-virtual-server ./cmd/server/main.go

# Stage 2: Create the final lean image
FROM alpine:latest

# Install ca-certificates for HTTPS connections (e.g., if connecting to external services)
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the built binary from the builder stage
COPY --from=builder /go-virtual-server .

# Copy the .env file if you want it inside the container (though passing via environment is better)
COPY .env /root/.env

# Expose the port your application listens on
EXPOSE 8080
 # Or use ${HTTP_PORT} if you want to be dynamic, but EXPOSE is typically static

# Command to run the application
CMD ["./go-virtual-server"]
