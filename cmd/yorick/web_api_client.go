package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

// WebAPIClient is a Slack Web API client.
type WebAPIClient struct {
	endpointURL string
	token       string
}

// NewWebAPIClient creates a WebAPIClient.
func NewWebAPIClient(endpointURL, token string) *WebAPIClient {
	return &WebAPIClient{
		endpointURL: endpointURL,
		token:       token,
	}
}

var httpClient = &http.Client{
	Timeout: 10 * time.Second,
}

// PostMessagePayload represents a chat.postMessage payload.
type PostMessagePayload struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

// APIResponse represents an API response.
type APIResponse struct {
	OK bool `json:"ok"`
}

// ChatPostMessage sends a message to a channel (chat.postMessage).
func (w *WebAPIClient) ChatPostMessage(channel, text string) error {
	payload := PostMessagePayload{
		Channel: channel,
		Text:    text,
	}

	buf, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error marshaling payload: %s", err)
	}

	req, err := http.NewRequest(
		http.MethodPost,
		fmt.Sprintf("%s/chat.postMessage", w.endpointURL),
		bytes.NewBuffer(buf),
	)
	if err != nil {
		return fmt.Errorf("error creating request: %s", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", w.token))

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error performing HTTP request: %s", err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		_ = resp.Body.Close()
		return fmt.Errorf("error reading body: %s", err)
	}

	if err := resp.Body.Close(); err != nil {
		return fmt.Errorf("error closing body: %s", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from API", resp.StatusCode)
	}

	var apiResponse APIResponse
	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return fmt.Errorf("error unmarshaling body: %s", err)
	}

	if !apiResponse.OK {
		return fmt.Errorf("API said !ok: %s (I sent %s)", body, buf)
	}

	return nil
}
