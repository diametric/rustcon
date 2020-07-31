package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/diametric/rustcon/middleware"
	"github.com/diametric/rustcon/stats"
	"github.com/diametric/rustcon/version"
	"github.com/diametric/rustcon/webrcon"
)

// CommandLineConfig options
type CommandLineConfig struct {
	ConfigFile   *string
	RconHost     *string
	RconPort     *int
	RconPassfile *string
	Tag          *string
	Version      *bool
}

// Config file definition
type Config struct {
	EnableRedisQueue  bool                     `json:"enable_redis_queue"`
	EnableInfluxStats bool                     `json:"enable_influx_stats"`
	QueuesPrefix      string                   `json:"queues_prefix"`
	IntervalCallbacks []IntervalCallbackConfig `json:"interval_callbacks"`
	StaticQueues      []string                 `json:"static_queues"`
	MaxQueueSize      int                      `json:"max_queue_size"`
	RedisConfig       RedisConfig              `json:"redis"`
	InfluxConfig      InfluxConfig             `json:"influx"`
	StatsConfig       StatsConfig              `json:"stats"`
}

// StatsConfig definition
type StatsConfig struct {
	Invoked   []InvokedStatConfig
	Monitored []MonitoredStatConfig
}

// InvokedStatConfig definition
type InvokedStatConfig struct {
	Command  string `json:"command"`
	Script   string `json:"script"`
	Interval int    `json:"interval"`
}

// MonitoredStatConfig definition
type MonitoredStatConfig struct {
	Pattern string `json:"pattern"`
	Script  string `json:"script"`
}

// IntervalCallbackConfig definition
type IntervalCallbackConfig struct {
	Command      string `json:"command"`
	StorageKey   string `json:"storage_key"`
	Interval     int    `json:"interval"`
	RunOnConnect bool   `json:"run_on_connect"`
}

// RedisConfig settings to connect to Redis
type RedisConfig struct {
	Host     string `json:"hostname"`
	Port     int    `json:"port"`
	Database int    `json:"db"`
	Password string `json:"password"`
}

// InfluxConfig settings to connect to InfluxDB for stats
type InfluxConfig struct {
	Host     string `json:"hostname"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Database string `json:"database"`
	SSL      bool   `json:"ssl"`
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
	data, err := ioutil.ReadFile(passfile)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(data)), nil
}

// I don't like this. Need to rework this at some point.
func buildOnConnectCallback(tag string, middleware middleware.Processor, cb IntervalCallbackConfig) webrcon.OnConnectCallback {
	return webrcon.OnConnectCallback{
		Command: cb.Command,
		Callback: func(response *webrcon.Response) {
			fmt.Printf("Got callback for %s, writing to Redis\n", cb.Command)
			_, err := middleware.Do("SET", strings.ReplaceAll(
				cb.StorageKey,
				"{tag}",
				tag), response.Message)
			if err != nil {
				fmt.Printf("Error writing to redis in callback: %s", err)
			}
		},
	}
}

func buildQueueCallback(queue string, queuemax int, middleware middleware.Processor) webrcon.OnMessageCallback {
	return webrcon.OnMessageCallback{
		Callback: func(message []byte) {
			middleware.Do("MULTI")
			middleware.Do("LTRIM", queue, 0, queuemax-2)
			middleware.Do("LPUSH", queue, message)
			middleware.Do("EXEC")
		},
	}
}

func main() {
	opts := CommandLineConfig{}

	opts.ConfigFile = flag.String("config", "rustcon.conf", "Path to the configuration file")
	opts.RconHost = flag.String("hostname", "localhost", "RCON hostname")
	opts.RconPort = flag.Int("port", 28016, "RCON port")
	opts.RconPassfile = flag.String("passfile", ".rconpass", "Path to a file containing the RCON password")
	opts.Tag = flag.String("tag", "", "A unique identifier that tags this server")
	opts.Version = flag.Bool("version", false, "Display version information")

	flag.Parse()

	if *opts.Version {
		fmt.Printf("rustcon version %s, build time %s, git revision %s, made with love by Diametric.\n",
			version.BuildVersion, version.BuildTime, version.GitRevision)
		return
	}

	if *opts.Tag == "" {
		*opts.Tag = fmt.Sprintf("%s:%d", *opts.RconHost, *opts.RconPort)
	}

	config, err := loadconfig(*opts.ConfigFile)
	if err != nil {
		fmt.Println("Error loading config:", err)
		return
	}

	if !config.EnableRedisQueue && !config.EnableInfluxStats {
		fmt.Println("You must have at least one of enable_redis_queue or enable_influx_stats enabled.")
		return
	}

	fmt.Println(config.QueuesPrefix)
	fmt.Println(config.StaticQueues)

	rconPassword, err := loadrconpass(*opts.RconPassfile)
	if err != nil {
		fmt.Println("Unable to read rcon passfile: ", err)
		return
	}

	rcon := webrcon.RconClient{}

	interrupt := make(chan os.Signal, 1)
	done := make(chan struct{})

	rcon.InitClient(*opts.RconHost, *opts.RconPort, rconPassword)

	if config.EnableRedisQueue {
		middleware := middleware.Processor{Tag: *opts.Tag, Rcon: &rcon}
		middleware.InitProcessor(
			config.RedisConfig.Host,
			config.RedisConfig.Port,
			config.RedisConfig.Database,
			config.RedisConfig.Password)

		_, err := middleware.Do("PING")
		if err != nil {
			fmt.Println(err)
		}

		for _, v := range config.IntervalCallbacks {
			fmt.Printf("Registering callback %s with interval %d and storagekey %s\n",
				v.Command, v.Interval, v.StorageKey)
			if v.Interval > 0 {
				middleware.AddIntervalCallback(v.Command, v.Interval, v.StorageKey)
			} else {
				if !v.RunOnConnect {
					fmt.Printf("%s callback has 0 interval, and false run_on_connect. This callback will never run and is probably not what you intended.\n", v.Command)
				}
			}
			if v.RunOnConnect {
				rcon.OnConnect(buildOnConnectCallback(*opts.Tag, middleware, v))
			}
		}

		for _, queue := range config.StaticQueues {
			rcon.OnMessage(buildQueueCallback(
				strings.ReplaceAll(fmt.Sprintf("%s:%s", config.QueuesPrefix, queue), "{tag}", *opts.Tag),
				config.MaxQueueSize,
				middleware))
		}

		go middleware.Process(done)
	}

	if config.EnableInfluxStats {
		statsclient := stats.Client{Tag: *opts.Tag, Rcon: &rcon}
		statsclient.InitClient(
			config.InfluxConfig.Host,
			config.InfluxConfig.Port,
			config.InfluxConfig.Database,
			config.InfluxConfig.Username,
			config.InfluxConfig.Password,
			config.InfluxConfig.SSL)

		for _, v := range config.StatsConfig.Invoked {
			statsclient.RegisterInvokedStat(v.Command, v.Script, v.Interval)
		}

		go statsclient.CollectStats(done)
	}

	go rcon.MaintainConnection(done)

	for {
		select {
		case <-done:
			return
		case <-interrupt:
			log.Print("Interrupt?")
		}
	}

}