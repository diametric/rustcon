package webrcon

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const (
	// StartingIdentifier sets the starting RCON ID
	StartingIdentifier = 1000
)

// RconStats holds various stats about the operation of the RCON client.
type RconStats struct {
	CommandsRun        int
	CommandTimeouts    int
	Disconnects        int
	Messages           int
	CacheHits          int
	CacheMisses        int
	OnConnectCallback  int
	OnMessageCallbacks int
	OnInvokeCallbacks  int
}

// RconClient maintains the connection to the Rust server.
type RconClient struct {
	Connected             bool
	CallOnMessageOnInvoke bool
	OnConnectDelay        int
	Stats                 RconStats
	identifier            int
	rconPath              string
	con                   *websocket.Conn
	callbacks             map[int]RconCallback
	onconnect             []OnConnectCallback
	onmessage             []OnMessageCallback
	mu                    sync.Mutex // So many mutexes, there must be a better
	cmu                   sync.Mutex // way..
	cachemu               sync.Mutex
	dcmu                  sync.Mutex
	cache                 map[string]commandCache
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
			client.Stats.CacheMisses++
			return nil
		}

		client.Stats.CacheHits++
		return cacheData.response
	}

	client.Stats.CacheMisses++
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
	client.Stats = RconStats{}

	zap.S().Infof("Initialized RCON client to %s:%d", host, port)
}

// MaintainConnection is intended to be run as a goroutine and will loop
// forever, maintaining a connection and restablishing whenever the websocket
// goes down.
func (client *RconClient) MaintainConnection(done chan struct{}, wg *sync.WaitGroup) {
	zap.S().Info("Starting up RCON client")
	wg.Add(1)

	for {
		select {
		case <-done:
			zap.S().Info("Shutting down RCON client.")
			wg.Done()
			if client.Connected {
				// This should interrupt the read on the rconReader goroutine
				client.disconnect()
			}
			return
		default:
		}

		if client.Connected {
			time.Sleep(5 * time.Second)
			continue
		}

		zap.S().Info("Connecting to RCON")
		err := client.connect()
		if err != nil {
			zap.S().Error("Error connecting to RCON: ", err)
			time.Sleep(5 * time.Second)
		} else {
			zap.S().Info("RCON connection established.")
			go client.rconReader(done, wg)
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
	if client.OnConnectDelay > 0 {
		time.Sleep(time.Duration(client.OnConnectDelay) * time.Second)
	}

	client.SendCallback(cb.Command, 0, cb.Callback)
}

func (client *RconClient) connect() error {
	zap.S().Debugf("Connecting to %s", client.rconPath)
	con, _, err := websocket.DefaultDialer.Dial(client.rconPath, nil)
	if err != nil {
		return fmt.Errorf("Error connecting to RCON: %s", err)
	}
	client.con = con
	client.Connected = true

	for _, v := range client.onconnect {
		client.Stats.OnConnectCallback++
		go client.runOnConnectCB(v)
	}

	return nil
}

func (client *RconClient) disconnect() {

	client.dcmu.Lock()
	defer client.dcmu.Unlock()

	if !client.Connected {
		zap.S().Warn("Attempting to disconnect an already disconnected connection.")
		return
	}

	client.Stats.Disconnects++

	zap.S().Info("Disconnecting RCON client")
	err := client.con.Close()
	if err != nil {
		zap.S().Warnf("Error closing connection: %s", err)
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
		return
	}

	if !client.Connected {
		zap.S().Info("Client is disconnected, unable to send command.")
		return
	}

	client.cmu.Lock()
	client.identifier++
	zap.S().Debugf("Set ID to %d for callback.", client.identifier)

	cmd := Command{
		Identifier: client.identifier,
		Message:    command,
		Name:       "WebRcon"}

	cb := RconCallback{
		ttl:       10,
		timestamp: time.Now().Unix(),
		callback:  client.cacheWrapper(command, cacheFor, callback)}

	client.callbacks[client.identifier] = cb
	client.cmu.Unlock()

	client.writeJSON(&cmd)

	client.Stats.CommandsRun++
}

// Send a command with no callback
func (client *RconClient) Send(command string) {
	if !client.Connected {
		zap.S().Info("Client is disconnected, unable to send command.")
		return
	}

	cmd := Command{
		Identifier: -1,
		Message:    command,
		Name:       "WebRcon"}

	client.writeJSON(&cmd)

	client.Stats.CommandsRun++
}

func (client *RconClient) rconReader(done chan struct{}, wg *sync.WaitGroup) {
	var sendOnMessage bool = true
	defer wg.Done()

	zap.S().Debug("Starting up RCON reader")
	wg.Add(1)

	for {
		select {
		case <-done:
			zap.S().Info("Shutting down RCON reader.")
			return
		default:
		}

		_, message, err := client.con.ReadMessage()

		if err != nil {
			zap.S().Errorf("RCON Read Error! Disconnecting from RCON. Error: %s", err)
			if client.Connected {
				client.disconnect()
			}

			return
		}

		client.Stats.Messages++
		zap.S().Debug("Received RCON message: ", string(message))

		var p Response

		if err := json.Unmarshal(message, &p); err != nil {
			zap.S().Errorf("Error decoding RCON websocket response: %s", err)
			continue
		}

		zap.S().Debugf("Registered invoke callbacks: %+v", client.callbacks)

		if p.Identifier >= StartingIdentifier {
			sendOnMessage = client.CallOnMessageOnInvoke

			zap.S().Debugf("Received RCON ID %d.", p.Identifier)

			client.cmu.Lock()
			if val, exists := client.callbacks[p.Identifier]; exists {
				client.cmu.Unlock()
				zap.S().Debugf("Calling callback %+v for ID %d", val, p.Identifier)
				client.Stats.OnInvokeCallbacks++
				go val.callback(&p)

				client.cmu.Lock()
				delete(client.callbacks, p.Identifier)
				client.cmu.Unlock()
			} else {
				client.cmu.Unlock()
				zap.S().Errorf("No callback found for %d, this shouldn't happen. Message: %s", p.Identifier, p.Message)
			}
		} else {
			sendOnMessage = true
		}

		if sendOnMessage {
			for _, v := range client.onmessage {
				client.Stats.OnMessageCallbacks++
				go v.Callback(message)
			}
		}

		client.cmu.Lock()
		for i, v := range client.callbacks {
			if v.ttl <= 0 {
				continue
			}

			if time.Now().Unix()-v.timestamp >= int64(v.ttl) {
				// Maybe change this to Debugf, but for now I think its worth
				// notifying in normal logging when callbacks expire. Its normal
				// to happen under high load or during initial connect.
				zap.S().Infof("Expiring callback ID %d, timed out.", i)

				delete(client.callbacks, i)
				client.Stats.CommandTimeouts++
			}
		}
		client.cmu.Unlock()
	}
}
