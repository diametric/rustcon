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
	cmu        sync.Mutex
	cachemu    sync.Mutex
	cache      map[string]commandCache
}

type commandCache struct {
	ttl       int
	timestamp int64
	response  *Response
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

func (client *RconClient) checkCache(command string) *Response {
	client.cachemu.Lock()
	defer client.cachemu.Unlock()

	if cacheData, ok := client.cache[command]; ok {
		if time.Now().Unix()-cacheData.timestamp >= int64(cacheData.ttl) {
			delete(client.cache, command)
			return nil
		}

		return cacheData.response
	}

	return nil
}

// InitClient sets up the RconClient
func (client *RconClient) InitClient(host string, port int, password string) {
	client.rconPath = fmt.Sprintf("ws://%s:%d/%s", host, port, password)

	// Default initial identifier value
	client.identifier = StartingIdentifier
	client.callbacks = make(map[int]RconCallback)
	client.cache = make(map[string]commandCache)
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
	client.SendCallback(cb.Command, 0, cb.Callback)
}

func (client *RconClient) connect() error {
	log.Printf("Connecting to %s\n", client.rconPath)
	con, _, err := websocket.DefaultDialer.Dial(client.rconPath, nil)
	if err != nil {
		return fmt.Errorf("Error connecting to RCON: %s", err)
	}
	client.con = con
	client.Connected = true

	for _, v := range client.onconnect {
		client.runOnConnectCB(v)
	}

	return nil
}

func (client *RconClient) disconnect() {
	log.Println("Disconnecting RCON client")
	err := client.con.Close()
	if err != nil {
		log.Println("Error closing connection:", err)
	}

	client.Connected = false
}

func (client *RconClient) writeJSON(v interface{}) error {
	client.mu.Lock()
	defer client.mu.Unlock()

	return client.con.WriteJSON(v)
}

func (client *RconClient) cacheWrapper(command string, cacheFor int, cb func(response *Response)) func(response *Response) {
	return func(response *Response) {
		if cacheFor > 0 {
			client.cachemu.Lock()
			client.cache[command] = commandCache{
				timestamp: time.Now().Unix(),
				ttl:       cacheFor,
				response:  response,
			}
			client.cachemu.Unlock()
		}
		cb(response)
	}
}

// SendCallback sends a command with a callback
func (client *RconClient) SendCallback(command string, cacheFor int, callback func(response *Response)) {
	// Short-circuit the whole process, callback on the cached results.
	cacheData := client.checkCache(command)
	if cacheData != nil {
		go callback(cacheData)
	}

	if !client.Connected {
		log.Println("Client is disconnected, unable to send command.")
		return
	}

	client.identifier++
	log.Printf("Set ID to %d for callback.\n", client.identifier)

	cmd := Command{
		Identifier: client.identifier,
		Message:    command,
		Name:       "WebRcon"}

	cb := RconCallback{
		ttl:       10,
		timestamp: time.Now().Unix(),
		callback:  client.cacheWrapper(command, cacheFor, callback)}

	client.cmu.Lock()
	client.callbacks[client.identifier] = cb
	client.cmu.Unlock()

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

		log.Println("Received RCON message: ", string(message))

		for _, v := range client.onmessage {
			go v.Callback(message)
		}

		var p Response

		if err := json.Unmarshal(message, &p); err != nil {
			log.Println("Error decoding webrcon: ", err)
		}

		fmt.Printf("DEBUG: %+v\n", client.callbacks)

		if p.Identifier >= StartingIdentifier {
			log.Printf("Received RCON ID %d.\n", p.Identifier)

			client.cmu.Lock()
			if val, exists := client.callbacks[p.Identifier]; exists {
				client.cmu.Unlock()
				log.Printf("Calling callback %+v for ID %d\n", val, p.Identifier)
				go val.callback(&p)
				log.Printf("Callback is done.\n")

				client.cmu.Lock()
				delete(client.callbacks, p.Identifier)
				client.cmu.Unlock()
			} else {
				client.cmu.Unlock()
				log.Printf("No callback found for %d, this shouldn't happen.\n", p.Identifier)
			}
		}

		client.cmu.Lock()
		for i, v := range client.callbacks {
			if v.ttl <= 0 {
				continue
			}

			if time.Now().Unix()-v.timestamp >= int64(v.ttl) {
				log.Printf("Expiring callback ID %d, timed out.\n", i)
				delete(client.callbacks, i)
			}
		}
		client.cmu.Unlock()
	}
}
