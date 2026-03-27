FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Build static binary ignoring CGO to allow using modernc.org/sqlite natively
RUN CGO_ENABLED=0 go build -o /chaind .

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=builder /chaind /usr/local/bin/chaind

# Setup environment
ENV CHAIND_PREFER_HTTP=true
ENV CHAIND_HTTP_PORT=7432

# Store data volume
VOLUME ["/root/.local/share/chaind", "/root/.config/chaind"]

EXPOSE 7432

ENTRYPOINT ["chaind", "daemon", "start", "--foreground"]
