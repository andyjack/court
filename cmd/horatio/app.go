package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/horgh/irc"
)

// App is an HTTP server.
type App struct {
	verbose bool
	client  *Client
}

func httpServer(verbose bool, port int, client *Client) error {
	app := &App{
		verbose: verbose,
		client:  client,
	}

	http.HandleFunc("/api/chat.postMessage", app.postMessageHandler)

	hostAndPort := fmt.Sprintf(":%d", port)

	log.Printf("Starting to listen on port %d for POST /api/chat.postMessage",
		port)
	if err := http.ListenAndServe(hostAndPort, nil); err != nil {
		return fmt.Errorf("error serving: %s", err)
	}

	return nil
}

// PostMessagePayload represents the payload sent in a chat.postMessage
// request.
type PostMessagePayload struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

// APIResponse is a response that is similar to Slack's Web API's response.
type APIResponse struct {
	OK bool `json:"ok"`
}

func (a *App) postMessageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		log.Printf("invalid request method")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	buf, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("error reading request: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var p PostMessagePayload
	if err := json.Unmarshal(buf, &p); err != nil {
		log.Printf("invalid JSON: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	a.client.Write(irc.Message{
		Command: "PRIVMSG",
		Params:  []string{p.Channel, p.Text},
	})

	resp := APIResponse{OK: true}
	{
		buf, err := json.Marshal(resp)
		if err != nil {
			log.Printf("error marshaling response: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		n, err := w.Write(buf)
		if err != nil {
			log.Printf("error writing response: %s", err)
			return
		}
		if n != len(buf) {
			log.Printf("error writing response: short write")
			return
		}
	}

	log.Printf("Received POST /api/chat.postMessage: %+v", p)
}
