package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/diametric/rustcon/webrcon"
)

// CommandLineConfig options
type CommandLineConfig struct {
	ConfigFile   *string
	RconHost     *string
	RconPort     *int
	RconPassfile *string
	Tag          *string
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

// Same as os.Open but with some common sanity checks before it.
func saneOpen(f string) (*os.File, error) {
	info, err := os.Stat(f)

	if os.IsNotExist(err) {
		return nil, fmt.Errorf("%s not found", f)
	}

	if info.IsDir() {
		return nil, fmt.Errorf("%s is a directory", f)
	}

	file, err := os.Open(f)
	// Some unknown uncommon error
	if err != nil {
		return nil, err
	}

	return file, nil
}

func loadconfig(filename string) (*Config, error) {
	file, err := saneOpen(filename)
	if err != nil {
		return nil, err
	}

	defer file.Close()

	decoder := json.NewDecoder(file)
	config := Config{}

	err = decoder.Decode(&config)
	if err != nil {
		return nil, fmt.Errorf("JSON parse error: %s", err)
	}

	return &config, nil
}

func loadrconpass(passfile string) (string, error) {
	file, err := saneOpen(passfile)
	if err != nil {
		return "", err
	}

	defer file.Close()

}

func main() {
	opts := CommandLineConfig{}

	opts.ConfigFile = flag.String("config", "rustcon.conf", "Path to the configuration file")
	opts.RconHost = flag.String("hostname", "localhost", "RCON hostname")
	opts.RconPort = flag.Int("port", 28016, "RCON port")
	opts.RconPassfile = flag.String("passfile", ".rconpass", "Path to a file containing the RCON password")

	flag.Parse()

	config, err := loadconfig(*opts.ConfigFile)
	if err != nil {
		fmt.Println("Error loading config:", err)
		return
	}

	fmt.Println(config.QueuesPrefix)
	fmt.Println(config.StaticQueues)

	rcon := webrcon.RconClient{}

	interrupt := make(chan os.Signal, 1)

	rcon.InitClient(*opts.RconHost, *opts.RconPort, "mypassword")

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
