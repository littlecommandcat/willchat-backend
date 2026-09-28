# How to use

See examples [here](examples/example.md)

# Requirements

```ini
go mod download
```

# Build

`build/<system>.bat` and output at `bin/<system>.exe`

# Docker

Install Docker with Docker Compose support (on Windows, use Docker Desktop with Linux containers), then run from the repository root:

```sh
docker compose up -d --build
```

This starts the backend and Redis. The backend waits for Redis to become healthy before starting.
The backend image also checks `/health` every 30 seconds; use `docker compose ps` to see container health.

- API: `http://localhost:8080`
- Health check: `http://localhost:8080/health`
- WebSocket: `ws://localhost:8080/ws`

The host port defaults to `8080`; set `PORT` in your shell or root `.env` to change it. No `.env` file is required. Compose supplies the container's Redis settings directly; the existing `REDIS_ADDR`, `REDIS_PASSWORD`, and `REDIS_DB` values in `.env` are not used by this stack. The Redis address in `compose.yaml` is XOR-encoded to match the application's current configuration format.

Redis is accessible only within the Compose network, and its data is persisted in the `redis-data` volume. Local `.env` files and credentials are excluded from the image build context. The backend image uses a multi-stage build and runs as a non-root user.

```sh
# View backend logs
docker compose logs -f backend

# Stop the stack (keep Redis data)
docker compose down

# Stop the stack and delete Redis data
docker compose down -v
```

# References

## Gin
https://gin-gonic.com/

https://ithelp.ithome.com.tw/articles/10244740

https://golang.com.tw/doc/tutorial/web-service-gin

## Websocket
https://willh.gitbook.io/build-web-application-with-golang-zhtw/08.0/08.2

https://github.com/gorilla/websocket

https://ithelp.ithome.com.tw/articles/10305994

https://golangbot.com/go-websocket-server/
