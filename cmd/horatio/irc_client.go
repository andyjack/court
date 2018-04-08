package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/horgh/irc"
)

// IRCClient is an IRC client.
type IRCClient struct {
	verbose   bool
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

// NewIRCClient creates an IRC client. It connects and joins a channel.
func NewIRCClient(
	verbose bool,
	nick,
	channel,
	host string,
	port int,
	wg *sync.WaitGroup,
) (*IRCClient, error) {
	hostAndPort := fmt.Sprintf("%s:%d", host, port)
	log.Printf("Connecting to IRC server %s...", hostAndPort)
	conn, err := dialer.Dial("tcp", hostAndPort)
	if err != nil {
		return nil, fmt.Errorf("error dialing: %s", err)
	}

	client := &IRCClient{
		verbose: verbose,
		nick:    nick,
		conn:    conn,
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

func (i *IRCClient) init(channel string) error {
	i.Write(irc.Message{
		Command: "NICK",
		Params:  []string{i.nick},
	})

	i.Write(irc.Message{
		Command: "USER",
		Params:  []string{i.nick, i.nick, "0", i.nick},
	})

	i.Write(irc.Message{
		Command: "JOIN",
		Params:  []string{channel},
	})

	timeoutChan := time.After(5 * time.Second)

	for {
		select {
		case <-timeoutChan:
			return fmt.Errorf("timeout waiting for connection init")
		case m, ok := <-i.readChan:
			if !ok {
				return fmt.Errorf("read channel closed")
			}

			if m.Command == "001" {
				log.Printf("Connected to IRC server")
				return nil
			}

			if m.Command == "NOTICE" {
				continue
			}

			return fmt.Errorf("received unexpected message: %s", m)
		}
	}
}

// Read reads an IRC message.
func (i *IRCClient) Read() (irc.Message, bool) {
	m, ok := <-i.readChan
	return m, ok
}

func (i *IRCClient) reader(wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		m, err := i.readMessage()
		if err != nil {
			log.Printf("error reading: %s", err)
			close(i.readChan)
			return
		}

		if i.verbose {
			log.Printf("read message: %s", m)
		}
		i.readChan <- m
	}
}

var readTimeout = 5 * time.Minute

func (i *IRCClient) readMessage() (irc.Message, error) {
	if err := i.conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
		return irc.Message{}, fmt.Errorf("error setting read deadline: %s", err)
	}

	line, err := i.rw.ReadString('\n')
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

// Write writes a message to the connection.
func (i *IRCClient) Write(m irc.Message) {
	i.writeChan <- m
}

func (i *IRCClient) writer(wg *sync.WaitGroup) {
	defer wg.Done()

	for m := range i.writeChan {
		if err := i.writeMessage(m); err != nil {
			log.Printf("error writing: %s", err)
			break
		}

		if i.verbose {
			log.Printf("wrote message: %s", m)
		}
	}

	for range i.writeChan {
	}
}

var writeTimeout = time.Minute

func (i *IRCClient) writeMessage(m irc.Message) error {
	buf, err := m.Encode()
	if err != nil && err != irc.ErrTruncated {
		return fmt.Errorf("error encoding message: %s", err)
	}

	if err := i.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return fmt.Errorf("error setting write deadline: %s", err)
	}

	sz, err := i.rw.WriteString(buf)
	if err != nil {
		return fmt.Errorf("error writing: %s", err)
	}

	if sz != len(buf) {
		return fmt.Errorf("short write")
	}

	if err := i.rw.Flush(); err != nil {
		return fmt.Errorf("error flushing: %s", err)
	}

	return nil
}

// Close cleans up the client.
func (i *IRCClient) Close() {
	close(i.writeChan)
	_ = i.conn.Close()
}
