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
	verbose      bool
	port         int
	webAPIClient *WebAPIClient
}

// NewEventListener creates an EventListener.
func NewEventListener(
	verbose bool,
	port int,
	webAPIClient *WebAPIClient,
) *EventListener {
	return &EventListener{
		verbose:      verbose,
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

// EventPayload represents an Event API request payload.
type EventPayload struct {
	// Top level event type.
	Type string `json:"type"`

	// url_verification events include a challenge field.
	Challenge string `json:"challenge"`

	Event Event `json:"event"`
}

// Event represents the actual event. It's part of an Event API payload.
type Event struct {
	Type    string `json:"type"`
	SubType string `json:"subtype"`
	Channel string `json:"channel"`
	User    string `json:"user"`
	Text    string `json:"text"`
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

	if e.verbose {
		e.log(r, "Received event with body: %s", buf)
	}

	var p EventPayload
	if err := json.Unmarshal(buf, &p); err != nil {
		e.log(r, "invalid JSON: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	e.log(r, "Received event: %+v", p)

	switch p.Type {
	case "url_verification":
		e.eventURLVerification(w, r, p)
		return
	case "event_callback":
		e.eventEventCallback(w, r, p)
		return
	default:
		e.log(r, "unexpected event type: %s: %s", p.Type)
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

// URLVerificationResponse represents the response we send to a
// url_verification event.
type URLVerificationResponse struct {
	Challenge string `json:"challenge"`
}

// eventURLVerification handles an url_verification event. This event happens
// when enabling event subscriptions on the app.
//
// We echo the challenge back.
func (e *EventListener) eventURLVerification(
	w http.ResponseWriter,
	r *http.Request,
	p EventPayload,
) {
	resp := URLVerificationResponse{
		Challenge: p.Challenge,
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
	p EventPayload,
) {
	switch p.Event.Type {
	case "message":
		e.eventMessage(w, r, p.Event)
		return
	default:
		e.log(r, "event_callback event.type not recognized")
		// Just say OK. We probably just don't support it but it is okay.
		return
	}
}

// eventMessage is the event that we receive when a message is posted to a
// channel.
//
// See https://api.slack.com/events/message
func (e *EventListener) eventMessage(
	w http.ResponseWriter,
	r *http.Request,
	event Event,
) {
	// subtypes can include our own messages (bot_message). To simplify things,
	// only deal with regular channel messages (which have no subtype).
	if event.SubType != "" {
		e.log(r, "Received message event with subtype %s, ignoring it",
			event.SubType)
		return
	}

	// Respond in a goroutine so we reply to the request ASAP.
	go func() {
		messageEvent(e.webAPIClient, event.Channel, event.User, event.Text)
	}()

	e.log(r, "Processed message event")
}
