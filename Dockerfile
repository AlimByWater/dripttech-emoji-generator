FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install required build dependencies
RUN apk add --no-cache gcc musl-dev

# Copy go mod files
COPY . .

# Download dependencies
RUN go mod download


# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o dripttech-emoji-generator .

# Create final lightweight image
FROM alpine:latest

WORKDIR /app

RUN apk update
RUN apk upgrade

# Install runtime dependencies if needed
RUN apk add --no-cache ca-certificates ffmpeg

# Copy the binary from builder
COPY --from=builder /app/dripttech-emoji-generator /app/dripttech-emoji-generator
COPY --from=builder /app/.env /app/.env

# Create necessary directories
COPY --from=builder  /app/session /app/session
COPY --from=builder  /app/session/user /app/session/user
COPY --from=builder  /app/session session
COPY --from=builder  /app/session/user session/user
RUN mkdir -p /app/tmp


EXPOSE 8080

CMD ["/app/dripttech-emoji-generator"]
