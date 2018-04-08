package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/horgh/irc"
)

func main() {
	args, err := getArgs()
	if err != nil {
		log.Fatalf("%s", err)
	}

	client, err := NewClient(args.nick, args.channel, args.ircHost, args.ircPort)
	if err != nil {
		log.Fatalf("error connecting: %s", err)
	}

	for {
		m, ok := <-client.readChan
		if !ok {
			break
		}
		log.Printf("got message: %s", m)

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

	go client.reader()
	go client.writer()

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

func (c *Client) reader() {
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

func (c *Client) writer() {
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

var httpClient = &http.Client{
	Timeout: 10 * time.Second,
}

func dispatchEvent(url string, m irc.Message) error {
	return nil
}
