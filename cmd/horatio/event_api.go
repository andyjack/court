package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/horgh/irc"
)

// EventAPI represents an Event API. This dispatches events to bots that expect
// to receive Slack Event API type events via HTTP.
type EventAPI struct {
	endpointURL string
}

// NewEventAPI creates a new EventAPI.
func NewEventAPI(endpointURL string) *EventAPI {
	return &EventAPI{
		endpointURL: endpointURL,
	}
}

// MessageEvent represents the payload we send for a message event.
//
// It's structured to be similar to the Slack Event API event of this type.
type MessageEvent struct {
	Type  string `json:"type"`
	Event Event  `json:"event"`
}

// Event is part of MessageEvent
type Event struct {
	Type    string `json:"type"`
	Channel string `json:"channel"`
	User    string `json:"user"`
	Text    string `json:"text"`
}

var httpClient = &http.Client{
	Timeout: 10 * time.Second,
}

// DispatchMessageEvent notifies the event listener of a message event.
func (e *EventAPI) DispatchMessageEvent(m irc.Message) error {
	event := MessageEvent{
		Type: "event_callback",
		Event: Event{
			Type:    "message",
			Channel: m.Params[0],
			User:    m.Prefix,
			Text:    m.Params[1],
		},
	}

	buf, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("error marshaling: %s", err)
	}

	req, err := http.NewRequest(
		http.MethodPost,
		e.endpointURL,
		bytes.NewBuffer(buf),
	)
	if err != nil {
		return fmt.Errorf("error creating request: %s", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error performing HTTP request: %s", err)
	}

	if _, err := ioutil.ReadAll(resp.Body); err != nil {
		_ = resp.Body.Close()
		return fmt.Errorf("error reading body: %s", err)
	}

	if err := resp.Body.Close(); err != nil {
		return fmt.Errorf("error closing body: %s", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from API", resp.StatusCode)
	}

	log.Printf("Dispatched message event: POST %s: %+v", e.endpointURL, m)
	return nil
}
