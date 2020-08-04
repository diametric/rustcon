package middleware

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/diametric/rustcon/webrcon"
	"github.com/gomodule/redigo/redis"
	"go.uber.org/zap"
)

// Processor contains the state data of the middleware instance
type Processor struct {
	Tag           string
	Rcon          *webrcon.RconClient
	pool          *redis.Pool
	tickcallbacks []TickCallback
}

// TickCallback processes the callbacks that run per tick.
type TickCallback struct {
	command    string
	interval   int
	storagekey string
}

// InitProcessor initializes the middleware processor, and establishes the redis connection pool
func (processor *Processor) InitProcessor(host string, port int, database int, password string) {
	processor.pool = &redis.Pool{
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial(
				"tcp",
				fmt.Sprintf("%s:%d", host, port),
				redis.DialDatabase(database),
				redis.DialPassword(password))
			if err != nil {
				return nil, err
			}
			return c, nil
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			if time.Since(t) < time.Minute {
				return nil
			}
			_, err := c.Do("PING")
			return err
		},
	}
}

// AddIntervalCallback registers a callback func to be called at a specified tick interval
func (processor *Processor) AddIntervalCallback(c string, i int, s string) {
	processor.tickcallbacks = append(processor.tickcallbacks, TickCallback{
		command:    c,
		interval:   i,
		storagekey: s,
	})
}

// Do automatically allocates a pool resource and runs Do() on it.
func (processor *Processor) Do(command string, args ...interface{}) (interface{}, error) {
	conn := processor.pool.Get()
	defer conn.Close()

	return conn.Do(command, args...)
}

// We need this to pass by reference the callback or everything breaks.
func (processor *Processor) runTickCallback(callback TickCallback) {
	zap.S().Debugf("PROCESSOR: Time to run %s, interval %d\n", callback.command, callback.interval)
	processor.Rcon.SendCallback(callback.command, callback.interval-1, func(response *webrcon.Response) {
		_, err := processor.Do("SET", strings.ReplaceAll(
			callback.storagekey,
			"{tag}",
			processor.Tag), response.Message)
		if err != nil {
			zap.S().Errorf("Error writing to redis in callback: %s\n", err)
		}
	})
}

// Process the various redis related functions, state maintenance, etc.
func (processor *Processor) Process(done chan struct{}, wg *sync.WaitGroup) {
	var ticks int64

	zap.S().Info("Starting up middlewarep rocessor")
	wg.Add(1)

	for {
		select {
		case <-done:
			zap.S().Info("Shutting down middleware processor.")
			wg.Done()
			return
		default:
		}

		ticks++

		for _, callback := range processor.tickcallbacks {
			// Intervals defined 0 or less are skipped and typically defined as
			// onconnect callbacks.
			if callback.interval <= 0 {
				continue
			}

			if ticks%int64(callback.interval) == 0 {
				processor.runTickCallback(callback)
			}
		}

		time.Sleep(1 * time.Second)
		zap.S().Debugf("MIDDLEWARE: Tick Count: %d\n", ticks)
	}
}
