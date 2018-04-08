package main

import (
	"flag"
	"fmt"
	"log"
)

func main() {
	args, err := getArgs()
	if err != nil {
		log.Fatalf("%s", err)
	}

	webAPIClient := NewWebAPIClient(args.url, args.token)

	eventListener := NewEventListener(args.verbose, args.port, webAPIClient)

	if err := eventListener.Serve(); err != nil {
		log.Fatalf("error serving: %s", err)
	}
}

// Args are command line arguments.
type Args struct {
	verbose bool
	port    int
	url     string
	token   string
}

func getArgs() (Args, error) {
	verbose := flag.Bool("verbose", false, "Enable verbose output")
	port := flag.Int("port", 8080, "Port to listen on")
	url := flag.String("url", "http://127.0.0.1:8081/api",
		"Slack API endpoint base URL. Typically https://slack.com/api")
	token := flag.String("token", "", "OAuth token to use with the Web API")

	flag.Parse()

	if *port <= 0 {
		flag.PrintDefaults()
		return Args{}, fmt.Errorf("port must be > 0")
	}

	if *url == "" {
		flag.PrintDefaults()
		return Args{}, fmt.Errorf("you must specify a URL")
	}

	// Allow token to be optional as it's not needed when running with horatio.

	return Args{
		verbose: *verbose,
		port:    *port,
		url:     *url,
		token:   *token,
	}, nil
}
