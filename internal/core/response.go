package core

// import (
// 	"encoding/json"
// )

type Response struct {
	Status  string `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}
