package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
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

	client, err := NewClient(args.nick, args.channel, args.ircHost, args.ircPort,
		&wg)
	if err != nil {
		log.Fatalf("error connecting: %s", err)
	}

	go func() {
		if err := httpServer(args.listenPort, client.writeChan); err != nil {
			log.Fatalf("error serving HTTP: %s", err)
		}
	}()

	for {
		m, ok := <-client.readChan
		if !ok {
			break
		}

		if m.Command == "PING" {
			client.writeChan <- irc.Message{
				Command: "PONG",
				Params:  []string{m.Params[0]},
			}
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
		log.Printf("dispatched PRIVMSG")
	}

	close(client.writeChan)
	_ = client.conn.Close()
	wg.Wait()
}

// Args are command line arguments.
type Args struct {
	listenPort int
	url        string
	ircHost    string
	ircPort    int
	nick       string
	channel    string
}

func getArgs() (Args, error) {
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
		listenPort: *listenPort,
		url:        *url,
		ircHost:    *ircHost,
		ircPort:    *ircPort,
		nick:       *nick,
		channel:    *channel,
	}, nil
}

// Client is an IRC client.
type Client struct {
	nick      string
	conn      net.Conn
	rw        *bufio.ReadWriter
	readChan  chan irc.Message
	writeChan chan irc.Message
}

var dialer = &net.Dialer{
	Timeout:   10 * time.Second,
	KeepAlive: 10 * time.Second,
}

// NewClient creates an IRC client. It connects and joins a channel.
func NewClient(
	nick,
	channel,
	host string,
	port int,
	wg *sync.WaitGroup,
) (*Client, error) {
	hostAndPort := fmt.Sprintf("%s:%d", host, port)
	conn, err := dialer.Dial("tcp", hostAndPort)
	if err != nil {
		return nil, fmt.Errorf("error dialing: %s", err)
	}

	client := &Client{
		nick: nick,
		conn: conn,
		rw: bufio.NewReadWriter(
			bufio.NewReader(conn),
			bufio.NewWriter(conn),
		),
		readChan:  make(chan irc.Message, 1024),
		writeChan: make(chan irc.Message, 1024),
	}

	wg.Add(1)
	go client.reader(wg)
	wg.Add(1)
	go client.writer(wg)

	if err := client.init(channel); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return client, nil
}

func (c *Client) init(channel string) error {
	c.writeChan <- irc.Message{
		Command: "NICK",
		Params:  []string{c.nick},
	}

	c.writeChan <- irc.Message{
		Command: "USER",
		Params:  []string{c.nick, c.nick, "0", c.nick},
	}

	c.writeChan <- irc.Message{
		Command: "JOIN",
		Params:  []string{channel},
	}

	timeoutChan := time.After(5 * time.Second)

	for {
		select {
		case <-timeoutChan:
			return fmt.Errorf("timeout waiting for connection init")
		case m, ok := <-c.readChan:
			if !ok {
				return fmt.Errorf("read channel closed")
			}

			if m.Command == "001" {
				return nil
			}

			if m.Command == "NOTICE" {
				continue
			}

			return fmt.Errorf("received unexpected message: %s", m)
		}
	}
}

func (c *Client) reader(wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		m, err := c.readMessage()
		if err != nil {
			log.Printf("error reading: %s", err)
			close(c.readChan)
			return
		}

		log.Printf("read message: %s", m)
		c.readChan <- m
	}
}

var readTimeout = 5 * time.Minute

func (c *Client) readMessage() (irc.Message, error) {
	if err := c.conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
		return irc.Message{}, fmt.Errorf("error setting read deadline: %s", err)
	}

	line, err := c.rw.ReadString('\n')
	if err != nil {
		return irc.Message{}, err
	}

	m, err := irc.ParseMessage(line)
	if err != nil && err != irc.ErrTruncated {
		return irc.Message{}, fmt.Errorf("unable to parse message: %s: %s", line,
			err)
	}

	return m, nil
}

func (c *Client) writer(wg *sync.WaitGroup) {
	defer wg.Done()

	for m := range c.writeChan {
		if err := c.writeMessage(m); err != nil {
			log.Printf("error writing: %s", err)
			break
		}

		log.Printf("wrote message: %s", m)
	}

	for range c.writeChan {
	}
}

var writeTimeout = time.Minute

func (c *Client) writeMessage(m irc.Message) error {
	buf, err := m.Encode()
	if err != nil && err != irc.ErrTruncated {
		return fmt.Errorf("error encoding message: %s", err)
	}

	if err := c.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return fmt.Errorf("error setting write deadline: %s", err)
	}

	sz, err := c.rw.WriteString(buf)
	if err != nil {
		return fmt.Errorf("error writing: %s", err)
	}

	if sz != len(buf) {
		return fmt.Errorf("short write")
	}

	if err := c.rw.Flush(); err != nil {
		return fmt.Errorf("error flushing: %s", err)
	}

	return nil
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

	return nil
}

// App is an HTTP server.
type App struct {
	writeChan chan<- irc.Message
}

func httpServer(port int, writeChan chan<- irc.Message) error {
	app := &App{
		writeChan: writeChan,
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

	a.writeChan <- irc.Message{
		Command: "PRIVMSG",
		Params:  []string{p.Channel, p.Text},
	}

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

	log.Printf("processed chat.postMessage: %+v", p)
}
