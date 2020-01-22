package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	"github.com/diametric/rustcon/webrcon"
	"github.com/gorilla/websocket"
)

func main() {

	interrupt := make(chan os.Signal, 1)

	rconPath := url.URL{Scheme: "ws", Host: "localhost:28030", Path: "/mypassword"}

	fmt.Println("Connecting")

	c, _, err := websocket.DefaultDialer.Dial(rconPath.String(), nil)

	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	fmt.Println("Connected.")

	done := make(chan struct{})

	go func() {
		for {
			cmd := webrcon.Command{Identifier: -1, Message: "fps", Name: "WebRcon"}

			c.WriteJSON(&cmd)
			time.Sleep(1 * time.Second)
		}
	}()

	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}

			var p webrcon.Response

			fmt.Println(p)

			if err := json.Unmarshal(message, &p); err != nil {
				fmt.Println("Error decoding webrcon: ", err)
			}

			if p.Type == "Chat" {
				var c webrcon.ChatMessage

				if err := json.Unmarshal([]byte(p.Message), &c); err != nil {
					fmt.Println("Error decoding chat json: ", err)
					fmt.Println("JSON message: ", p.Message)
				}
				fmt.Printf("\nReceived chat:\nChannel: %d\nUserId: %s\nUsername: %s\nMessage: %s", c.Channel, c.UserID, c.Username, c.Message)
			}

			fmt.Printf("ID: %d, Message: %s", p.Identifier, p.Message)
			fmt.Printf("recv: %s", message)
		}
	}()

	for {
		select {
		case <-done:
			return
		case <-interrupt:
			log.Print("Interrupt?")
		}
	}

}
