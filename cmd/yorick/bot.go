package main

import (
	"fmt"
	"log"
	"regexp"
	"sync"
)

var whohas string
var taken bool
var mutex = &sync.Mutex{}
var re = regexp.MustCompile(`^(.+?)!`)

// messageEvent gets called when we see a message in a channel.
//
// We can use the WebAPIClient to reply in the channel if we like.
func messageEvent(
	client *WebAPIClient,
	channel string,
	user string,
	text string,
) {
	// logic:
	// take and taken: says who took it already and reply sorry
	// take and not taken: say to take it and you have it
	// release and taken: only release if you have it
	// release and not taken: say wha
	// status: report on status:
	switch text {
	case "!status":
		status(client, channel, user)
	case "!take":
		take(client, channel, user)
	case "!release":
		release(client, channel, user)
	}
}

func status(
	client *WebAPIClient,
	channel string,
	user string,
) {
	var reply string
	mutex.Lock()
	if taken {
		reply = fmt.Sprintf("%s has the lab", formatUser(whohas))
	} else {
		reply = "no one has claimed the lab. !take to take"
	}
	mutex.Unlock()
	sendReply(client, channel, reply)
}

func take(
	client *WebAPIClient,
	channel string,
	user string,
) {
	var reply string
	mutex.Lock()
	if taken {
		if user == whohas {
			reply = fmt.Sprintf("%s already has the lab!", formatUser(user))
		} else {
			reply = fmt.Sprintf("Alas %s has already taken lab, go bug them", formatUser(whohas))
		}
	} else {
		taken = true
		whohas = user
		reply = fmt.Sprintf("%s now has the lab, go forth and prosper", formatUser(user))
	}
	mutex.Unlock()
	sendReply(client, channel, reply)
}

func release(
	client *WebAPIClient,
	channel string,
	user string,
) {
	var reply string
	mutex.Lock()
	if taken {
		if user == whohas {
			taken = false
			whohas = ""
			reply = fmt.Sprintf("Release successful")
		} else {
			reply = fmt.Sprintf("You cannot release when %s has the lab", formatUser(whohas))
		}
	} else {
		reply = "No one has taken the lab, alas, nothing to release"
	}
	mutex.Unlock()
	sendReply(client, channel, reply)
}

func sendReply(
	client *WebAPIClient,
	channel string,
	reply string,
) {
	err := client.ChatPostMessage(channel, reply)
	if err != nil {
		log.Printf("Error posting message to channel: %s", err)
		return
	}
}

func formatUser(user string) string {
	matches := re.FindStringSubmatch(user)
	if matches != nil {
		return matches[1]
	}
	return "BADUSERSTRING"
}
