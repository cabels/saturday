package hue

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	BridgeIP     string
	Username     string
	DefaultLight string
}

func New(bridgeIP, username, defaultLight string) *Client {
	return &Client{
		BridgeIP:     bridgeIP,
		Username:     username,
		DefaultLight: defaultLight,
	}
}

func (c *Client) SetState(on bool, bri int, xy []float64) error {
	payload := map[string]interface{}{
		"on":  on,
		"bri": bri,
		"xy":  xy,
	}
	url, err := url.JoinPath("http://", c.BridgeIP, "api", c.Username, "groups", c.DefaultLight, "action")
	if err != nil {
		return fmt.Errorf("failed to construct URL: %w", err)
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal state to JSON: %w", err)
	}
	const maxRetries = 3
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonPayload))
		if err != nil {
			lastErr = fmt.Errorf("failed to create HTTP request (attempt %d): %w", attempt, err)
			break
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("failed to send HTTP request (attempt %d): %w", attempt, err)
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			lastErr = fmt.Errorf("received non-OK HTTP status (attempt %d): %s, body: %s", attempt, resp.Status, string(respBody))
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			continue
		}
		return nil
	}
	return lastErr
}
