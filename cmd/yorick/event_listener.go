package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

// EventListener is an HTTP server that receives Slack Event API HTTP events.
// It can use Slack's Web API to do things in response.
type EventListener struct {
	port         int
	webAPIClient *WebAPIClient
}

// NewEventListener creates an EventListener.
func NewEventListener(port int, webAPIClient *WebAPIClient) *EventListener {
	return &EventListener{
		port:         port,
		webAPIClient: webAPIClient,
	}
}

// Serve starts serving requests.
//
// It does not return unless there is an error.
func (e *EventListener) Serve() error {
	http.HandleFunc("/event", e.eventHandler)

	hostAndPort := fmt.Sprintf(":%d", e.port)

	log.Printf("Starting to listen on port %d for POST /event", e.port)
	if err := http.ListenAndServe(hostAndPort, nil); err != nil {
		return fmt.Errorf("error serving: %s", err)
	}

	return nil
}

// eventHandler handles an HTTP request sent to the /event endpoint.
func (e *EventListener) eventHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		e.log(r, "invalid request method")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	buf, err := ioutil.ReadAll(r.Body)
	if err != nil {
		e.log(r, "error reading request: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var p map[string]interface{}
	if err := json.Unmarshal(buf, &p); err != nil {
		e.log(r, "invalid JSON: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	e.log(r, "received event: %+v", p)

	eventType, ok := p["type"]
	if !ok {
		e.log(r, "no event type found")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	eventTypeString, ok := eventType.(string)
	if !ok {
		e.log(r, "event type is not a string")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	switch eventTypeString {
	case "url_verification":
		e.eventURLVerification(w, r, p)
		return
	case "event_callback":
		e.eventEventCallback(w, r, p)
		return
	default:
		e.log(r, "unexpected event type: %s: %s", eventTypeString, buf)
		// Just say OK. It's likely we just don't support it but it is fine.
		return
	}
}

// log logs a message associated with the given request.
func (e *EventListener) log(r *http.Request, f string, args ...interface{}) {
	log.Print(
		fmt.Sprintf("HTTP %s %s from %s: ", r.Method, r.URL.Path, r.RemoteAddr) +
			fmt.Sprintf(f, args...),
	)
}

// eventURLVerification handles an url_verification event. This event happens
// when enabling event subscriptions on the app.
//
// We echo the challenge back.
func (e *EventListener) eventURLVerification(
	w http.ResponseWriter,
	r *http.Request,
	p map[string]interface{},
) {
	challenge, ok := p["challenge"]
	if !ok {
		e.log(r, "url_verification event is missing challenge")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	challengeString, ok := challenge.(string)
	if !ok {
		e.log(r, "url_verification event challenge is not a string")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	type Response struct {
		Challenge string `json:"challenge"`
	}
	resp := Response{
		Challenge: challengeString,
	}

	buf, err := json.Marshal(resp)
	if err != nil {
		e.log(r, "error marshaling url_verification response: %s")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	n, err := w.Write(buf)
	if err != nil {
		e.log(r, "error writing url_verification response: %s", err)
		// Can't send an HTTP status code as we should have written.
		return
	}
	if n != len(buf) {
		e.log(r, "error writing url_verification response: short write")
		// Can't send an HTTP status code as we should have written.
		return
	}

	e.log(r, "Processed url_verification event")
}

// eventEventCallback is the event that happens when we receive a regular
// authorized user event. It holds an event object inside it which can be of
// different types, so we dispatch to different functions depending on that
// event.
func (e *EventListener) eventEventCallback(
	w http.ResponseWriter,
	r *http.Request,
	p map[string]interface{},
) {
	event, ok := p["event"]
	if !ok {
		e.log(r, "event_callback event not found")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	eventMap, ok := event.(map[string]interface{})
	if !ok {
		e.log(r, "event_callback event is not an object")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	eventType, ok := eventMap["type"]
	if !ok {
		e.log(r, "event_callback event.type not found")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	eventTypeString, ok := eventType.(string)
	if !ok {
		e.log(r, "event_callback event.type is not a string")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	switch eventTypeString {
	case "message":
		e.eventMessage(w, r, eventMap)
		return
	default:
		e.log(r, "event_callback event.type not recognized")
		// Just say OK. We probably just don't support it but it is okay.
		return
	}
}

// eventMessage is the event that we receive when a message is posted to a
// channel.
func (e *EventListener) eventMessage(
	w http.ResponseWriter,
	r *http.Request,
	event map[string]interface{},
) {
	// subtypes can include our own messages (bot_message). To simplify things,
	// only deal with regular channel messages (which have no subtype).
	if _, ok := event["subtype"]; ok {
		e.log(r, "got channel event with subtype, ignoring it")
		return
	}

	ch, ok := event["channel"]
	if !ok {
		e.log(r, "message event does not have channel")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	chString, ok := ch.(string)
	if !ok {
		e.log(r, "message event channel is not a string")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Respond in a goroutine so we reply to the event request ASAP.
	go func() {
		m := "hi there"
		if err := e.webAPIClient.ChatPostMessage(chString, m); err != nil {
			e.log(r, "error posting message to channel: %s", err)
			return
		}
		e.log(r, "Sent message via Web API: %s: %s", chString, m)
	}()

	e.log(r, "Processed event_callback message event")
}
