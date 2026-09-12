package core

// import (
// 	"encoding/json"
// )

type Response struct {
	Status string `json:"status"`
	Code   string `json:"code"`
	Data   any    `json:"data,omitempty"`
}
