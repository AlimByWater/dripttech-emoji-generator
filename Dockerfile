FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install required build dependencies
RUN apk add --no-cache gcc musl-dev

# Copy go mod files
COPY . .

# Download dependencies
RUN go mod download


# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o app .

# Create final lightweight image
FROM alpine:latest

WORKDIR /app

RUN apk update
RUN apk upgrade

# Install runtime dependencies if needed
RUN apk add --no-cache ca-certificates ffmpeg

# Copy the binary from builder
COPY --from=builder /app/app .
COPY --from=builder /app/.env .
COPY --from=builder /app/migrations ./migrations

# Create necessary directories
RUN mkdir -p /app/session
RUN mkdir -p session
RUN mkdir -p /app/tmp

# Set executable permissions
RUN chmod +x /app

EXPOSE 8080

CMD ["./app"]