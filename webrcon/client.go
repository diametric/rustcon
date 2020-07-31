package webrcon

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// StartingIdentifier sets the starting RCON ID
	StartingIdentifier = 1000
)

// RconClient maintains the connection to the Rust server.
type RconClient struct {
	Connected  bool
	identifier int
	rconPath   string
	con        *websocket.Conn
	callbacks  map[int]RconCallback
	onconnect  []OnConnectCallback
	onmessage  []OnMessageCallback
	mu         sync.Mutex
}

// OnMessageCallback contains the callbacks run on every RCON message
type OnMessageCallback struct {
	Callback func(message []byte)
}

// OnConnectCallback contains the on connect callback functions
type OnConnectCallback struct {
	Command  string
	Callback func(response *Response)
}

// RconCallback struct contains the information about a registered callback,
// and its TTL
type RconCallback struct {
	ttl       int
	timestamp int64
	callback  func(response *Response)
}

// InitClient sets up the RconClient
func (client *RconClient) InitClient(host string, port int, password string) {
	client.rconPath = fmt.Sprintf("ws://%s:%d/%s", host, port, password)

	// Default initial identifier value
	client.identifier = StartingIdentifier
	client.callbacks = make(map[int]RconCallback)
	client.Connected = false

	log.Printf("Initialized RCON client to %s:%d", host, port)
}

// MaintainConnection is intended to be run as a goroutine and will loop
// forever, maintaining a connection and restablishing whenever the websocket
// goes down.
func (client *RconClient) MaintainConnection(done chan struct{}) {
	for {
		if client.Connected {
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

// OnConnect registers a callback to be called on connect.
func (client *RconClient) OnConnect(cb OnConnectCallback) {
	client.onconnect = append(client.onconnect, cb)
}

// OnMessage registers a callback to be called on every raw rcon message
func (client *RconClient) OnMessage(cb OnMessageCallback) {
	client.onmessage = append(client.onmessage, cb)
}

func (client *RconClient) runOnConnectCB(cb OnConnectCallback) {
	client.SendCallback(cb.Command, cb.Callback)
}

func (client *RconClient) connect() error {
	fmt.Printf("Connecting to %s\n", client.rconPath)
	con, _, err := websocket.DefaultDialer.Dial(client.rconPath, nil)
	if err != nil {
		return fmt.Errorf("Error connecting to RCON: %s", err)
	}
	client.con = con
	client.Connected = true

	fmt.Println("Running OnConnect callbacks")
	for _, v := range client.onconnect {
		fmt.Printf("-> Running %s\n", v.Command)
		client.runOnConnectCB(v)
	}

	return nil
}

func (client *RconClient) disconnect() {
	log.Println("Disconnecting RCON client")
	err := client.con.Close()
	if err != nil {
		fmt.Println("Error closing connection:", err)
	}

	client.Connected = false
}

func (client *RconClient) writeJSON(v interface{}) error {
	client.mu.Lock()
	defer client.mu.Unlock()

	return client.con.WriteJSON(v)
}

// SendCallback sends a command with a callback
func (client *RconClient) SendCallback(command string, callback func(response *Response)) {
	if !client.Connected {
		log.Println("Client is disconnected, unable to send command.")
		return
	}

	client.identifier++

	cmd := Command{
		Identifier: client.identifier,
		Message:    command,
		Name:       "WebRcon"}

	cb := RconCallback{
		ttl:       10,
		timestamp: time.Now().Unix(),
		callback:  callback}
	client.callbacks[client.identifier] = cb

	client.writeJSON(&cmd)
}

// Send a command with no callback
func (client *RconClient) Send(command string) {
	if !client.Connected {
		log.Println("Client is disconnected, unable to send command.")
		return
	}

	cmd := Command{
		Identifier: -1,
		Message:    command,
		Name:       "WebRcon"}

	client.writeJSON(&cmd)
}

func (client *RconClient) rconReader() {
	log.Println("Starting up RCON reader")
	for {
		_, message, err := client.con.ReadMessage()

		if err != nil {
			log.Println("RCON Read Error!\nError:", err)
			log.Println("Disconnecting from RCON")
			client.disconnect()
			return
		}

		for _, v := range client.onmessage {
			v.Callback(message)
		}

		var p Response

		if err := json.Unmarshal(message, &p); err != nil {
			fmt.Println("Error decoding webrcon: ", err)
		}

		if p.Identifier >= StartingIdentifier {
			fmt.Println("Caught callback")
			if val, exists := client.callbacks[p.Identifier]; exists {
				val.callback(&p)
				delete(client.callbacks, p.Identifier)
			} else {
				fmt.Printf("No callback found for %d, this shouldn't happen.", p.Identifier)
			}
		}

		fmt.Printf("ID %d, Message: %s\n", p.Identifier, p.Message)
		fmt.Printf("Raw: %s\n", message)

		fmt.Println("Expiring old callbacks")
		for i, v := range client.callbacks {
			if v.ttl <= 0 {
				continue
			}

			if time.Now().Unix()-v.timestamp >= int64(v.ttl) {
				fmt.Printf("-> Expiring %d\n", i)
				delete(client.callbacks, i)
			}
		}
	}
}
