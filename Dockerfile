FROM golang:1.26.5-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY data ./data
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.23

RUN apk add --no-cache ca-certificates \
    && addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /out/server ./server

ENV PORT=8080 GIN_MODE=release
USER app
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -q -O /dev/null "http://127.0.0.1:${PORT}/health" || exit 1
ENTRYPOINT ["/app/server"]
