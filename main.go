package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/diametric/rustcon/webrcon"
)

// CommandLineConfig options
type CommandLineConfig struct {
	RconHost     string
	RconPort     int
	RconPassfile string
	Tag          string
}

// Config file definition
type Config struct {
	QueuesPrefix string      `json:"queues_prefix"`
	StaticQueues []string    `json:"static_queues"`
	RedisConfig  RedisConfig `json:"redis"`
}

// RedisConfig settings to connect to Redis
type RedisConfig struct {
	Host     string `json:"hostname"`
	Port     int    `json:"port"`
	Database int    `json:"db"`
	Password string `json:"password"`
}

func loadconfig() (*Config, error) {
	info, err := os.Stat("rustcon.conf")

	if os.IsNotExist(err) {
		return nil, fmt.Errorf("rustconf.conf not found")
	}

	if info.IsDir() {
		return nil, fmt.Errorf("rustconf.conf is a directory")
	}

	file, _ := os.Open("rustcon.conf")
	defer file.Close()

	decoder := json.NewDecoder(file)
	config := Config{}

	err = decoder.Decode(&config)
	if err != nil {
		return nil, fmt.Errorf("JSON parse error: %s", err)
	}

	return &config, nil
}

func main() {

	config, err := loadconfig()
	if err != nil {
		fmt.Println("Error loading config:", err)
		return
	}

	fmt.Println(config.QueuesPrefix)
	fmt.Println(config.StaticQueues)

	rcon := webrcon.RconClient{}

	interrupt := make(chan os.Signal, 1)

	rcon.InitClient("localhost", 28030, "mypassword")

	done := make(chan struct{})
	go rcon.MaintainConnection(done)

	go func() {
		for {
			rcon.Send("fps")
			time.Sleep(1 * time.Second)
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
