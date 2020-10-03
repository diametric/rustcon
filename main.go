package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/diametric/rustcon/middleware"
	"github.com/diametric/rustcon/stats"
	"github.com/diametric/rustcon/version"
	"github.com/diametric/rustcon/webrcon"
	"github.com/gomodule/redigo/redis"
)

// CommandLineConfig options
type CommandLineConfig struct {
	ConfigFile   *string
	RconHost     *string
	RconPort     *int
	RconPassfile *string
	Tag          *string
	Version      *bool
	Debug        *bool
	Test         *bool
}

// Config file definition
type Config struct {
	EnableRedisQueue        bool                     `json:"enable_redis_queue"`
	EnableInfluxStats       bool                     `json:"enable_influx_stats"`
	QueuesPrefix            string                   `json:"queues_prefix"`
	IntervalCallbacks       []IntervalCallbackConfig `json:"interval_callbacks"`
	StaticQueues            []string                 `json:"static_queues"`
	DynamicQueueKey         string                   `json:"dynamic_queue_key"`
	CallbackQueueKey        string                   `json:"callback_queue_key"`
	CallbackExpire          int                      `json:"callback_expire"`
	LoggingConfig           zap.Config               `json:"logging"`
	MaxQueueSize            int                      `json:"max_queue_size"`
	CallOnMessageOnInvoke   bool                     `json:"call_onmessage_on_invoke"`
	IgnoreEmptyRconMessages bool                     `json:"ignore_empty_rcon_messages"`
	OnConnectDelay          int                      `json:"onconnect_delay"`
	RedisConfig             RedisConfig              `json:"redis"`
	InfluxConfig            InfluxConfig             `json:"influx"`
	StatsConfig             StatsConfig              `json:"stats"`
}

// StatsConfig definition
type StatsConfig struct {
	Internal  []InternalStatsConfig
	Invoked   []InvokedStatConfig
	Monitored []MonitoredStatConfig
}

// ScriptedStatImpl defines the base implementation all stat configs use
type ScriptedStatImpl struct {
	Script   string `json:"script"`
	Disabled bool   `json:"disabled"`
}

// InternalStatsConfig definition
type InternalStatsConfig struct {
	ScriptedStatImpl
	Interal int `json:"interval"`
}

// InvokedStatConfig definition
type InvokedStatConfig struct {
	ScriptedStatImpl
	Command  string `json:"command"`
	Interval int    `json:"interval"`
}

// MonitoredStatConfig definition
type MonitoredStatConfig struct {
	ScriptedStatImpl
	Pattern string `json:"pattern"`
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
			_, err := middleware.Do("SET", strings.ReplaceAll(
				cb.StorageKey,
				"{tag}",
				tag), response.Message)
			if err != nil {
				zap.S().Errorf("Error writing to redis in callback: %s", err)
			}
		},
	}
}

func buildDynamicQueueCallback(queuekey string, queueprefix string, tag string, queuemax int, middleware middleware.Processor) webrcon.OnMessageCallback {
	return webrcon.OnMessageCallback{
		Callback: func(message []byte) {
			queues, err := redis.Strings(middleware.Do("SMEMBERS", queuekey))
			if err != nil {
				zap.S().Errorf("Error getting dynamic queues: %s", err)
				return
			}
			for _, queue := range queues {
				fqueue := strings.ReplaceAll(fmt.Sprintf("%s:%s", queueprefix, queue), "{tag}", tag)

				conn, err := middleware.StartPipeline()
				if err != nil {
					zap.S().Errorf("Unable to start redis pipeline while processing queue callback: %s", err)
					continue
				}

				conn.Send("LTRIM", fqueue, 0, queuemax-2)
				conn.Send("LPUSH", fqueue, message)
				conn.Do("EXEC")
				conn.Close()
			}
		},
	}
}

func buildQueueCallback(queue string, queuemax int, middleware middleware.Processor) webrcon.OnMessageCallback {
	return webrcon.OnMessageCallback{
		Callback: func(message []byte) {
			conn, err := middleware.StartPipeline()
			if err != nil {
				zap.S().Errorf("Unable to start redis pipeline while processing queue callback: %s", err)
				return
			}
			defer conn.Close()

			conn.Send("LTRIM", queue, 0, queuemax-2)
			conn.Send("LPUSH", queue, message)
			conn.Do("EXEC")
		},
	}
}

func main() {
	opts := CommandLineConfig{}

	opts.ConfigFile = flag.String("config", "rustcon.conf", "Path to the configuration file")
	opts.RconHost = flag.String("hostname", "localhost", "RCON hostname")
	opts.RconPort = flag.Int("port", 28016, "RCON port")
	opts.RconPassfile = flag.String("passfile", ".rconpass", "Path to a file containing the RCON password")
	opts.Tag = flag.String("tag", "", "A unique identifier that tags this server (defaults to hostname:port)")
	opts.Version = flag.Bool("version", false, "Display version information")
	opts.Debug = flag.Bool("debug", false, "Override log level in config, and set to debug")
	opts.Test = flag.Bool("test", false, "Perform only test writes, output to stdout")

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

	config.LoggingConfig.EncoderConfig = zapcore.EncoderConfig{
		MessageKey:   "message",
		LevelKey:     "level",
		EncodeLevel:  zapcore.CapitalColorLevelEncoder,
		TimeKey:      "time",
		EncodeTime:   zapcore.ISO8601TimeEncoder,
		CallerKey:    "caller",
		EncodeCaller: zapcore.ShortCallerEncoder,
	}

	if *opts.Debug {
		fmt.Println("Overriding configured log level, setting to debug.")
		config.LoggingConfig.Level.SetLevel(zap.DebugLevel)
	}

	logger, logerr := config.LoggingConfig.Build()
	if logerr != nil {
		panic(logerr)
	}

	undo := zap.ReplaceGlobals(logger)
	defer undo()
	defer zap.S().Sync()

	rconPassword, err := loadrconpass(*opts.RconPassfile)
	if err != nil {
		fmt.Println("Unable to read rcon passfile: ", err)
		return
	}

	rcon := webrcon.RconClient{
		IgnoreEmptyRconMessages: config.IgnoreEmptyRconMessages,
		CallOnMessageOnInvoke:   config.CallOnMessageOnInvoke,
		OnConnectDelay:          config.OnConnectDelay}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	done := make(chan struct{})
	var wg sync.WaitGroup

	rcon.InitClient(*opts.RconHost, *opts.RconPort, rconPassword)

	if config.EnableRedisQueue {
		middleware := middleware.Processor{
			Tag:              *opts.Tag,
			Rcon:             &rcon,
			CallbackExpire:   config.CallbackExpire,
			CallbackQueueKey: strings.ReplaceAll(config.CallbackQueueKey, "{tag}", *opts.Tag)}

		middleware.InitProcessor(
			config.RedisConfig.Host,
			config.RedisConfig.Port,
			config.RedisConfig.Database,
			config.RedisConfig.Password)

		_, err := middleware.Do("PING")
		if err != nil {
			zap.S().Error("Error while connecting to redis: ", err)
		}

		for _, v := range config.IntervalCallbacks {
			if v.Interval > 0 {
				middleware.AddIntervalCallback(v.Command, v.Interval, v.StorageKey)
			} else {
				if !v.RunOnConnect {
					zap.S().Warnf("%s callback has 0 interval, and false run_on_connect. This callback will never run and is probably not what you intended.", v.Command)
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

		// This is gross but whatever.
		rcon.OnMessage(buildDynamicQueueCallback(config.DynamicQueueKey, config.QueuesPrefix, *opts.Tag, config.MaxQueueSize, middleware))

		go middleware.Process(done, &wg)
	}

	if config.EnableInfluxStats {
		statsclient := stats.Client{Tag: *opts.Tag, Rcon: &rcon, Test: *opts.Test}
		statsclient.InitClient(
			config.InfluxConfig.Host,
			config.InfluxConfig.Port,
			config.InfluxConfig.Database,
			config.InfluxConfig.Username,
			config.InfluxConfig.Password,
			config.InfluxConfig.SSL)

		for _, v := range config.StatsConfig.Invoked {
			if !v.Disabled {
				statsclient.RegisterInvokedStat(v.Command, v.Script, v.Interval)
			}
		}

		for _, v := range config.StatsConfig.Internal {
			if !v.Disabled {
				statsclient.RegisterInternalStat(v.Script, v.Interal)
			}
		}

		for _, v := range config.StatsConfig.Monitored {
			if !v.Disabled {
				statsclient.RegisterMonitoredStat(v.Pattern, v.Script)
			}
		}

		rcon.OnMessage(webrcon.OnMessageCallback{
			Callback: statsclient.OnMessageMonitoredStat})

		go statsclient.CollectStats(done, &wg)
	}

	go rcon.MaintainConnection(done, &wg)

	for {
		select {
		case <-interrupt:
			zap.S().Warn("CTRL-C caught, exiting.")
			log.Println("CTRL-C caught, exiting.")
			zap.S().Sync()
			close(done)
			wg.Wait()
			return
		}
	}
}
