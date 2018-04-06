package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
)

func main() {
	args, err := getArgs()
	if err != nil {
		log.Fatalf("invalid argument: %s", err)
	}

	app := App{
		client: NewClient(args.token),
	}

	http.HandleFunc("/event", app.EventHandler)

	if err := http.ListenAndServe(fmt.Sprintf(":%d", args.port),
		nil); err != nil {
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
	token := flag.String("token", "", "Slack token to use with API")

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
