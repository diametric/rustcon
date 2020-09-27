package middleware

import (
	"encoding/json"
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
	Tag              string
	Rcon             *webrcon.RconClient
	CallbackQueueKey string
	pool             *redis.Pool
	tickcallbacks    []TickCallback
}

// TickCallback processes the callbacks that run per tick.
type TickCallback struct {
	command    string
	interval   int
	storagekey string
}

// CallbackRequest contains the information to handle an RCON callback request.
type CallbackRequest struct {
	ID      int    `json:"id"`
	Command string `json:"command"`
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

// StartPipeline begins a redis pipeline. The caller is responsible for
// closing the connection.
func (processor *Processor) StartPipeline() (redis.Conn, error) {
	conn := processor.pool.Get()

	err := conn.Send("MULTI")
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// Send automatically allocates a pool resource and runs Send() on it.
func (processor *Processor) Send(command string, args ...interface{}) error {
	conn := processor.pool.Get()
	defer conn.Close()

	return conn.Send(command, args...)
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

func (processor *Processor) buildRequestCallback(command string, id int) func(*webrcon.Response) {
	return func(response *webrcon.Response) {
		resultkey := fmt.Sprintf("%s:results:%d", processor.CallbackQueueKey, id)
		_, err := processor.Do("SET", resultkey, response.Message)
		if err != nil {
			zap.S().Errorf("Error writing to redis request callback response for command %s, id %d response %v: %s", command, id, response, err)
		}
	}
}

func (processor *Processor) processCallbackRequests() {
	conn, err := processor.StartPipeline()
	if err != nil {
		zap.S().Errorf("Error starting redis pipeline while processing callback requests: %s", err)
		return
	}
	defer conn.Close()

	conn.Send("LRANGE", processor.CallbackQueueKey, 0, -1)
	conn.Send("DEL", processor.CallbackQueueKey)
	results, err := redis.Values(conn.Do("EXEC"))

	if err != nil {
		zap.S().Errorf("Error getting RCON callback requests: %s", err)
		return
	}

	requests, err := redis.Strings(results[0], err)
	for _, request := range requests {
		var r CallbackRequest
		if err := json.Unmarshal([]byte(request), &r); err != nil {
			zap.S().Errorf("Error decoding RCON callback request: %s", err)
			continue
		}

		processor.Rcon.SendCallback(r.Command, 0, processor.buildRequestCallback(r.Command, r.ID))
	}
}

// Process the various redis related functions, state maintenance, etc.
func (processor *Processor) Process(done chan struct{}, wg *sync.WaitGroup) {
	var ticks int64

	zap.S().Info("Starting up middleware processor")
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

		processor.processCallbackRequests()

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
