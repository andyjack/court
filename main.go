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

	app := App{
		client: NewClient(args.token),
	}

	if err := app.Serve(); err != nil {
		log.Fatalf("error serving: %s", err)
	}
}

// Args are command line arguments.
type Args struct {
	port  int
	token string
}

func getArgs() (Args, error) {
	port := flag.Int("port", 8080, "Port to listen on")
	token := flag.String("token", "", "Slack OAuth token to use with its Web API")

	flag.Parse()

	if *port <= 0 {
		flag.PrintDefaults()
		return Args{}, fmt.Errorf("port must be > 0")
	}

	if *token == "" {
		flag.PrintDefaults()
		return Args{}, fmt.Errorf("you must provide a token")
	}

	return Args{
		port:  *port,
		token: *token,
	}, nil
}
