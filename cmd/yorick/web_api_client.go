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

// ChatPostMessage sends a message to a channel (chat.postMessage).
func (w *WebAPIClient) ChatPostMessage(channel, text string) error {
	type Payload struct {
		Channel string `json:"channel"`
		Text    string `json:"text"`
	}
	payload := Payload{
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

	var p map[string]interface{}
	if err := json.Unmarshal(body, &p); err != nil {
		return fmt.Errorf("error unmarshaling body: %s", err)
	}

	ok, exists := p["ok"]
	if !exists {
		return fmt.Errorf("response did not include ok")
	}
	success, isBool := ok.(bool)
	if !isBool {
		return fmt.Errorf("response ok was not bool")
	}
	if !success {
		return fmt.Errorf("API said !ok: %+v (I sent %s)", p, buf)
	}

	return nil
}
