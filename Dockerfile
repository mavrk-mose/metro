FROM golang:1.24.5-alpine AS builder

# Install necessary build tools (e.g., for CGO if needed)
RUN apk add --no-cache git build-base

WORKDIR /app

# Download dependencies first to leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV SERVICE=metro \
    PORT=8200

    # Build the Go app with static linking (important for Alpine)
    RUN CGO_ENABLED=0 GOOS=linux go build -o main ./cmd/main.go

    FROM alpine:3.20

    # Install certificates (required if your app makes HTTPS calls)
    RUN apk add --no-cache ca-certificates

    WORKDIR /app

    COPY --from=builder /app/main .

    COPY --from=builder /app/pkg/db/migrations /pkg/db/migrations

    ENV SERVICE=safari_server \
        PORT=8200

        EXPOSE 8200

        CMD ["./main"]