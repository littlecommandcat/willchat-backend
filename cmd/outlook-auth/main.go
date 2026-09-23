// Run once to authorize a personal Outlook account; keep the printed refresh token private.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"willchat-backend/database"
)

func main() {
	if err := database.LoadEnv(); err != nil {
		log.Fatal(err)
	}
	clientID := os.Getenv("OUTLOOK_CLIENT_ID")
	if clientID == "" {
		log.Fatal("set OUTLOOK_CLIENT_ID first")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	base := "https://login.microsoftonline.com/consumers/oauth2/v2.0/"
	client := &http.Client{Timeout: 10 * time.Second}
	post := func(endpoint string, form url.Values, result any) error {
		req, err := http.NewRequestWithContext(ctx, "POST", base+endpoint, strings.NewReader(form.Encode()))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return err
		}
		if resp.StatusCode != 200 {
			return fmt.Errorf("Microsoft returned HTTP %d", resp.StatusCode)
		}
		return nil
	}
	var device struct {
		DeviceCode string `json:"device_code"`
		Message    string `json:"message"`
		Interval   int    `json:"interval"`
	}
	if err := post("devicecode", url.Values{"client_id": {clientID}, "scope": {"offline_access Mail.Send"}}, &device); err != nil {
		log.Fatal(err)
	}
	fmt.Println(device.Message)
	if device.Interval < 5 {
		device.Interval = 5
	}
	for {
		select {
		case <-ctx.Done():
			log.Fatal("authorization timed out")
		case <-time.After(time.Duration(device.Interval) * time.Second):
		}
		form := url.Values{"client_id": {clientID}, "grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}, "device_code": {device.DeviceCode}}
		req, err := http.NewRequestWithContext(ctx, "POST", base+"token", strings.NewReader(form.Encode()))
		if err != nil {
			log.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := client.Do(req)
		if err != nil {
			log.Fatal(err)
		}
		var token struct {
			RefreshToken string `json:"refresh_token"`
			Error        string `json:"error"`
		}
		err = json.NewDecoder(resp.Body).Decode(&token)
		resp.Body.Close()
		if err != nil {
			log.Fatal(err)
		}
		if token.RefreshToken != "" {
			fmt.Println("OUTLOOK_REFRESH_TOKEN=" + token.RefreshToken)
			return
		}
		if token.Error == "slow_down" {
			device.Interval += 5
			continue
		}
		if token.Error != "authorization_pending" {
			log.Fatalf("authorization failed: %s", token.Error)
		}
	}
}
