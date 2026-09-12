package api

import (
	"net/http"

	"willchat-backend/internal/core"

	"github.com/gin-gonic/gin"
)

func Health(c *gin.Context) {
	c.JSON(http.StatusOK, core.Response{
		Status: "ok",
		Code:   "success",
	})
}
