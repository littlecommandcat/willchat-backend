package api

import (
	"net/http"

	"willchat-backend/data"
	"willchat-backend/internal/core"

	"github.com/gin-gonic/gin"
)

func Health(c *gin.Context) {
	c.JSON(http.StatusOK, core.Response{
		Status:  "ok",
		Code:    "success",
		Message: "Service is online",
		Data: gin.H{
			"message":     "Service is online",
			"version":     data.LatestVersion,
			"connections": core.ConnectionCount,
		},
	})
}
