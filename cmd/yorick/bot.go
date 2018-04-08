package main

import "log"

// messageEvent gets called when we see a message in a channel.
//
// We can use the WebAPIClient to reply in the channel if we like.
func messageEvent(
	client *WebAPIClient,
	channel string,
	user string,
	text string,
) {
	if text == "hello" {
		reply := "hi there"

		err := client.ChatPostMessage(channel, reply)
		if err != nil {
			log.Printf("Error posting message to channel: %s", err)
			return
		}
		return
	}

	reply := "huh?"
	err := client.ChatPostMessage(channel, reply)
	if err != nil {
		log.Printf("Error posting message to channel: %s", err)
		return
	}
}
