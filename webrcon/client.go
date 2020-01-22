package webrcon

import (
	"encoding/json"

	"github.com/gorilla/websocket"
)

// RconClient maintains the connection to the Rust server.
type RconClient struct {
	con 
}

func (client *RconClient) Connect() {

}

