# How to use

See examples [here](examples/example.md)

# Requirements

```ini
go mod download
```

# Build

`build/<system>.bat` and output at `bin/<system>.exe`

# Email verification (personal Outlook)

1. Register an app at [Microsoft Entra App registrations](https://entra.microsoft.com/) with supported account type **Personal Microsoft accounts only**. Under Authentication → Advanced settings enable **Allow public client flows**. Grant delegated Microsoft Graph `Mail.Send` permission.
2. Put the app's Application (client) ID in `.env` as `OUTLOOK_CLIENT_ID`. Run `go run ./cmd/outlook-auth`, sign in with the Outlook sender account at the displayed URL, and copy the printed `OUTLOOK_REFRESH_TOKEN` into `.env` (keep it private; do not commit it). Restart the server.
3. `POST /auth/verification-code` with `{ "email": "recipient@example.com" }` sends a six-digit code, valid 10 minutes (one send per minute). `POST /auth/register` requires `username`, `email`, `password`, and `code`. Changing email via `PUT /auth/me` also requires a code for the new address.

Microsoft Graph requires OAuth2 for personal Outlook mail; an Outlook password/app password is not supported here. If the refresh token is revoked or expires, repeat step 2. Graph's HTTP 202 means the send request was accepted, not guaranteed delivered.

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