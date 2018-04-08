package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/horgh/irc"
)

func main() {
	args, err := getArgs()
	if err != nil {
		log.Fatalf("%s", err)
	}

	var wg sync.WaitGroup

	client, err := NewClient(args.verbose, args.nick, args.channel, args.ircHost,
		args.ircPort, &wg)
	if err != nil {
		log.Fatalf("error connecting: %s", err)
	}

	app := NewApp(args.verbose, client)

	go func() {
		if err := app.Serve(args.listenPort); err != nil {
			log.Fatalf("error serving HTTP: %s", err)
		}
	}()

	for {
		m, ok := <-client.readChan
		if !ok {
			break
		}

		if m.Command == "PING" {
			client.Write(irc.Message{
				Command: "PONG",
				Params:  []string{m.Params[0]},
			})
			continue
		}

		if m.Command != "PRIVMSG" {
			continue
		}

		if m.Params[0][0] != '#' {
			continue
		}

		if err := dispatchEvent(args.url, m); err != nil {
			log.Printf("error dispatching event: %s", err)
			continue
		}
	}

	client.Close()
	wg.Wait()
}

// Args are command line arguments.
type Args struct {
	verbose    bool
	listenPort int
	url        string
	ircHost    string
	ircPort    int
	nick       string
	channel    string
}

func getArgs() (Args, error) {
	verbose := flag.Bool("verbose", false, "Enable verbose output")
	listenPort := flag.Int("listen-port", 8081, "Port to listen on (HTTP)")
	url := flag.String("url", "http://localhost:8080/event",
		"URL to send message events to")
	ircHost := flag.String("irc-host", "localhost", "IRC server host")
	ircPort := flag.Int("irc-port", 6667, "IRC server port")
	nick := flag.String("nick", "bot", "Nickname to use")
	channel := flag.String("channel", "#test", "Channel to join")

	flag.Parse()

	if *listenPort <= 0 {
		flag.PrintDefaults()
		return Args{}, fmt.Errorf("listen port must be > 0")
	}

	if *url == "" {
		flag.PrintDefaults()
		return Args{}, fmt.Errorf("you must provide a URL")
	}

	if *ircHost == "" {
		flag.PrintDefaults()
		return Args{}, fmt.Errorf("you must provide an IRC host")
	}

	if *ircPort <= 0 {
		flag.PrintDefaults()
		return Args{}, fmt.Errorf("you must provide an IRC port")
	}

	if *nick == "" {
		flag.PrintDefaults()
		return Args{}, fmt.Errorf("you must provide a nick")
	}

	if *channel == "" {
		flag.PrintDefaults()
		return Args{}, fmt.Errorf("you must provide a channel")
	}

	return Args{
		verbose:    *verbose,
		listenPort: *listenPort,
		url:        *url,
		ircHost:    *ircHost,
		ircPort:    *ircPort,
		nick:       *nick,
		channel:    *channel,
	}, nil
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

func dispatchEvent(url string, m irc.Message) error {
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
		url,
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

	log.Printf("Dispatched message event: POST %s: %+v", url, m)
	return nil
}
