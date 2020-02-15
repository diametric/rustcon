package webrcon

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

// RconClient maintains the connection to the Rust server.
type RconClient struct {
	connected  bool
	identifier int
	rconPath   url.URL
	con        *websocket.Conn
	callbacks  map[int]RconCallback
}

// RconCallback struct contains the information about a registered callback,
// and its TTL
type RconCallback struct {
	ttl      int
	callback func(response *Response)
}

// InitClient sets up the RconClient
func (client *RconClient) InitClient(host string, port int, password string) {
	client.rconPath = url.URL{
		Scheme: "ws",
		Host:   fmt.Sprintf("%s:%d", host, port),
		Path:   fmt.Sprintf("/%s", password)}

	// Default initial identifier value
	client.identifier = 1000
	client.callbacks = make(map[int]RconCallback)
	client.connected = false

	log.Printf("Initialized RCON client to %s:%d", host, port)
}

// MaintainConnection is intended to be run as a goroutine and will loop
// forever, maintaining a connection and restablishing whenever the websocket
// goes down.
func (client *RconClient) MaintainConnection(done chan struct{}) {
	for {
		if client.connected {
			time.Sleep(5 * time.Second)
			continue
		}

		log.Println("Connecting to RCON")
		err := client.connect()
		if err != nil {
			log.Println("Error connecting to RCON:", err)
			time.Sleep(5 * time.Second)
		} else {
			go client.rconReader()
		}
	}
}

func (client *RconClient) connect() error {
	con, _, err := websocket.DefaultDialer.Dial(client.rconPath.String(), nil)
	if err != nil {
		return fmt.Errorf("Error connecting to RCON: %s", err)
	}
	client.con = con
	client.connected = true

	return nil
}

func (client *RconClient) disconnect() {
	log.Println("Disconnecting RCON client")
	err := client.con.Close()
	if err != nil {
		fmt.Println("Error closing connection:", err)
	}

	client.connected = false
}

// Send a command with no callback
func (client *RconClient) Send(command string) {
	if !client.connected {
		log.Println("Client is disconnected, unable to send command.")
		return
	}

	cmd := Command{
		Identifier: -1,
		Message:    command,
		Name:       "WebRcon"}

	client.con.WriteJSON(&cmd)
}

func (client *RconClient) rconReader() {
	log.Println("Starting up rconReader()")
	for {
		_, message, err := client.con.ReadMessage()
		if err != nil {
			log.Println("READ ERROR!\nread:", err)
			client.disconnect()
			return
		}

		var p Response

		if err := json.Unmarshal(message, &p); err != nil {
			fmt.Println("Error decoding webrcon: ", err)
		}

		fmt.Printf("ID %d, Message: %s", p.Identifier, p.Message)
		fmt.Printf("Raw: %s", message)
	}
}
