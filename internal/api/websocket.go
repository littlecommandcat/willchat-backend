package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/gin-gonic/gin"

	"willchat-backend/internal/core"
)

func (s *ChatService) WebSocket(c *gin.Context) {
	token := strings.TrimSpace(c.Query("token"))
	if core.SafeCompare(token, "") {
		if headerToken, ok := memberToken(c); ok {
			token = headerToken
		}
	}

	memberID, ok := s.auth.validateToken(token)
	if !ok {
		code := "invalid_token"
		message := "token is invalid"

		if core.SafeCompare(token, "") {
			code = "missing_token"
			message = "a member token is required"
		}

		writeError(c, http.StatusUnauthorized, code, message)
		return
	}

	conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		log.Printf("WebSocket accept failed: %v", err)
		return
	}
	defer conn.CloseNow()
	member, err := s.auth.memberByID(memberID)
	if err != nil {
		log.Printf("WebSocket member lookup failed: %v", err)
		return
	}
	responseMember, err := s.auth.toMemberResponse(member, false)
	if err != nil {
		log.Printf("WebSocket member response failed: %v", err)
		return
	}
	groupRecords := responseMember.GroupRecords
	groupIDs := []string{}
	for i := range groupRecords {
		groupIDs = append(groupIDs, s.snowflake.ToSnowflake(groupRecords[i].GroupID))
	}
	client := s.hub.Register(s.snowflake.ToSnowflake(memberID), groupIDs)
	defer s.hub.Unregister(client)
	log.Printf("Websocket connected: %s", s.snowflake.ToSnowflake(memberID))
	online := make([]memberResponse, 0)
	for _, id := range s.hub.OnlineFor(s.snowflake.ToSnowflake(memberID), groupIDs) {
		member, err := s.auth.publicMemberByID(id)
		if err != nil {
			log.Printf("WebSocket online member lookup failed: %v", err)
			return
		}
		online = append(online, member)
	}

	// Ready event
	s.hub.BroadcastOnlyMember(s.snowflake.ToSnowflake(memberID), core.WSEvent{
		Event: core.WSEventEnum.Ready,
		Payload: gin.H{
			"member": responseMember,
			"groups": responseMember.GroupRecords,
			"online": online,
		},
	})

	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	// Heartbeat
	go websocketHeartbeat(ctx, conn)

	// Server -> Client
	go func() {
		for payload := range client.Send {
			if err := wsjson.Write(ctx, conn, json.RawMessage(payload)); err != nil {
				cancel()
				return
			}
		}

		cancel()
	}()

	// Client -> Server
	for {
		var incoming any

		if err := wsjson.Read(ctx, conn, &incoming); err != nil {
			cancel()
			return
		}
	}
}

func websocketHeartbeat(ctx context.Context, conn *websocket.Conn) {
	ticker := time.NewTicker(websocketPingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(
				ctx,
				websocketPingTimeout,
			)

			err := conn.Ping(pingCtx)

			cancel()

			if err != nil {
				log.Printf("WebSocket heartbeat failed: %v", err)
				return
			}
		}
	}
}
