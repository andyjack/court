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

	client := NewClient(args.url, args.token)

	app := App{
		port:   args.port,
		client: client,
	}

	if err := app.Serve(); err != nil {
		log.Fatalf("error serving: %s", err)
	}
}

// Args are command line arguments.
type Args struct {
	port  int
	url   string
	token string
}

func getArgs() (Args, error) {
	port := flag.Int("port", 8080, "Port to listen on")
	url := flag.String("url", "http://127.0.0.1:8081/api",
		"Slack API endpoint base URL. Typically https://slack.com/api")
	token := flag.String("token", "", "Slack OAuth token to use with its Web API")

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
		port:  *port,
		url:   *url,
		token: *token,
	}, nil
}
