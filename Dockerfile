FROM golang:1.24-alpine AS builder

WORKDIR /app

# Cache dependency downloads
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build static binary — pure Go, no CGO required
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /chaind .

# ─── Runtime ───
FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=builder /chaind /usr/local/bin/chaind

# Health check — ping the daemon status endpoint
HEALTHCHECK --interval=30s --timeout=5s \
  CMD wget -qO- http://localhost:7432/api/v1/adapters/status || exit 1

# Environment — HTTP mirror is enabled by default in containers since Unix sockets
# can't easily traverse Docker network boundaries.
ENV CHAIND_PREFER_HTTP=true
ENV CHAIND_HTTP_PORT=7432

# Persistent volumes for SQLite database and configuration/credentials
VOLUME ["/root/.local/share/chaind", "/root/.config/chaind"]

EXPOSE 7432

ENTRYPOINT ["chaind", "daemon", "start"]
