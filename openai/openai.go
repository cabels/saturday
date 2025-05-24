package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	APIKey string
}

func New(apiKey string) *Client {
	return &Client{
		APIKey: apiKey,
	}
}

type Result struct {
	On        bool      `json:"on"`
	Bri       int       `json:"bri"`
	XY        []float64 `json:"xy,omitempty"`
	SongTitle string    `json:"song_title,omitempty"`
	Artist    string    `json:"artist,omitempty"`
}

func (c *Client) Post(prompt string) (Result, error) {
	if c.APIKey == "" {
		return Result{}, errors.New("OpenAI API key is required")
	}
	if prompt == "" {
		return Result{}, errors.New("prompt cannot be empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	body := openAIRequest{
		Model:    "gpt-3.5-turbo",
		Messages: []openAIMessage{{Role: "system", Content: `You are an assistant that extracts lighting parameters and song information from user requests. Respond ONLY with a minified JSON object with these fields and no explanation: on (boolean, if the light is on or off), bri (integer, 1-254, brightness of the light), xy (array of two floats, color in CIE 1931 color space, e.g. [0.35,0.35]), song_title (string, the title of the song if mentioned, otherwise empty), artist (string, the artist of the song if mentioned, otherwise empty). Always use the field name "xy" as an array of two floats. Do not use "x" or "y" fields. Example: {"on":true,"bri":200,"xy":[0.35,0.35],"song_title":"Imagine","artist":"John Lennon"}. If ambiguous, default to {"on":true,"bri":254,"xy":[0.35,0.35],"song_title":"","artist":""}.`}, {Role: "user", Content: prompt}},
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return Result{}, fmt.Errorf("failed to marshal OpenAI request body: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return Result{}, fmt.Errorf("failed to create OpenAI request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.APIKey)
	request.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(request)
	if err != nil {
		return Result{}, fmt.Errorf("OpenAI request failed: %w", err)
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, fmt.Errorf("failed to read OpenAI response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("OpenAI API error: status %d, body: %s", resp.StatusCode, string(respBytes))
	}
	var result openAIResponse
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return Result{}, fmt.Errorf("OpenAI response unmarshal error: %w, body: %s", err, string(respBytes))
	}
	if len(result.Choices) == 0 {
		return Result{}, fmt.Errorf("no choices returned from OpenAI, body: %s", string(respBytes))
	}
	var params Result
	if err := json.Unmarshal([]byte(result.Choices[0].Message.Content), &params); err != nil {
		return Result{}, fmt.Errorf("OpenAI content unmarshal error: %w, content: %s", err, result.Choices[0].Message.Content)
	}
	return params, nil
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}
