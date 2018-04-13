package main

import (
	"flag"
	"fmt"
	"log"
	"sync"

	"github.com/horgh/irc"
)

func main() {
	args, err := getArgs()
	if err != nil {
		log.Fatalf("%s", err)
	}

	var wg sync.WaitGroup

	ircClient, err := NewIRCClient(args.verbose, args.nick, args.channel,
		args.ircHost, args.ircPort, &wg)
	if err != nil {
		log.Fatalf("error connecting: %s", err)
	}

	webAPI := NewWebAPI(args.verbose, ircClient)
	go func() {
		if err := webAPI.Serve(args.listenPort); err != nil {
			log.Fatalf("error serving HTTP: %s", err)
		}
	}()

	eventAPI := NewEventAPI(args.url)

	for {
		m, ok := ircClient.Read()
		if !ok {
			break
		}

		if m.Command == "PING" {
			ircClient.Write(irc.Message{
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

		if err := eventAPI.DispatchMessageEvent(m); err != nil {
			log.Printf("error dispatching message event: %s", err)
			continue
		}
	}

	ircClient.Close()
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
		"Event API listener URL. We send message events here.")
	ircHost := flag.String("irc-host", "localhost", "IRC server host")
	ircPort := flag.Int("irc-port", 6667, "IRC server port")
	nick := flag.String("nick", "Yorick", "Nickname to use")
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
