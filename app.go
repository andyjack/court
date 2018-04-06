package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

// App holds global app state.
type App struct {
	client *Client
}

// EventHandler handles an HTTP request sent to the /event endpoint.
func (a *App) EventHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.Log(r, "invalid request method")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	buf, err := ioutil.ReadAll(r.Body)
	if err != nil {
		a.Log(r, "error reading request: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Parse payload.
	var p map[string]interface{}
	if err := json.Unmarshal(buf, &p); err != nil {
		a.Log(r, "invalid JSON: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	eventType, ok := p["type"]
	if !ok {
		a.Log(r, "no event type found")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	eventTypeString, ok := eventType.(string)
	if !ok {
		a.Log(r, "event type is not a string")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	switch eventTypeString {
	case "url_verification":
		a.EventURLVerification(w, r, p)
		return
	case "event_callback":
		a.EventEventCallback(w, r, p)
		return
	default:
		a.Log(r, "unexpected event type: %s: %s", eventTypeString, buf)
		// Just say OK
		return
	}
}

// Log logs a message associated with the given request.
func (a *App) Log(r *http.Request, f string, args ...interface{}) {
	log.Print(
		fmt.Sprintf("HTTP %s %s from %s: ", r.Method, r.URL.Path, r.RemoteAddr) +
			fmt.Sprintf(f, args...),
	)
}

// EventURLVerification handles an url_verification" event. This event happens
// when enabling event subscriptions on the app.
//
// We echo the challenge back.
func (a *App) EventURLVerification(
	w http.ResponseWriter,
	r *http.Request,
	p map[string]interface{},
) {
	challenge, ok := p["challenge"]
	if !ok {
		a.Log(r, "url_verification event is missing challenge")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	challengeString, ok := challenge.(string)
	if !ok {
		a.Log(r, "url_verification event challenge is not a string")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// We echo the challenge.

	type Response struct {
		Challenge string `json:"challenge"`
	}

	resp := Response{
		Challenge: challengeString,
	}

	buf, err := json.Marshal(resp)
	if err != nil {
		a.Log(r, "error marshaling url_verification response: %s")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	n, err := w.Write(buf)
	if err != nil {
		a.Log(r, "error writing url_verification response: %s", err)
		return
	}

	if n != len(buf) {
		a.Log(r, "error writing url_verification response: short write")
		return
	}

	a.Log(r, "Successfully replied to url_verification")
}

// EventEventCallback is the event that happens when we receive a regular
// authorized user event.
func (a *App) EventEventCallback(
	w http.ResponseWriter,
	r *http.Request,
	p map[string]interface{},
) {
	event, ok := p["event"]
	if !ok {
		a.Log(r, "event_callback.event not found")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	eventMap, ok := event.(map[string]interface{})
	if !ok {
		a.Log(r, "event_callback.event is not an object")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	eventType, ok := eventMap["type"]
	if !ok {
		a.Log(r, "event_callback.event.type not found")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	eventTypeString, ok := eventType.(string)
	if !ok {
		a.Log(r, "event_callback.event.type is not a string")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	switch eventTypeString {
	case "message":
		a.EventMessageChannels(w, r, p, eventMap)
		return
	default:
		a.Log(r, "event_callback.event.type not recognized")
		// Just say OK
		return
	}
}

// EventMessageChannels is the event that we receive when a message is posted
// to a channel.
func (a *App) EventMessageChannels(
	w http.ResponseWriter,
	r *http.Request,
	p map[string]interface{},
	event map[string]interface{},
) {
	a.Log(r, "Got message event: %+v", p)

	if subType, ok := event["subtype"]; ok {
		if subType, ok := subType.(string); ok {
			if subType == "bot_message" {
				a.Log(r, "got channel event.subtype bot_message, ignoring it")
				return
			}
		}
	}

	ch, ok := event["channel"]
	if !ok {
		a.Log(r, "message event does not have channel")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	chString, ok := ch.(string)
	if !ok {
		a.Log(r, "message event channel is not a string")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	go func() {
		if err := a.client.ChatPostMessage(
			chString,
			"hi there",
		); err != nil {
			a.Log(r, "error posting message to channel: %s", err)
			return
		}
		a.Log(r, "sent message to channel in reply")
	}()
}
