package stats

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"log"
	"sync"
	"time"

	"github.com/d5/tengo"
	"github.com/d5/tengo/stdlib"

	"github.com/diametric/rustcon/webrcon"

	influxdb2 "github.com/influxdata/influxdb-client-go"
)

// Client maintains the InfluxDB client connection
type Client struct {
	Tag          string
	Rcon         *webrcon.RconClient
	influxDb     influxdb2.Client
	database     string
	stats        []Stats
	tengoGlobals map[string]interface{}
	tengomu      sync.Mutex
}

// Stats contains the configured stats plugins
type Stats struct {
	interval   int
	command    string
	scriptpath string
	script     *tengo.Script
}

// RegisterInvokedStat registers an invoked type stat.
func (client *Client) RegisterInvokedStat(command string, scriptpath string, interval int) {
	scriptdata, err := ioutil.ReadFile(scriptpath)
	if err != nil {
		log.Printf("Unable to load script from %s for %s, registration ignored.\n", scriptpath, command)
		return
	}

	script := tengo.NewScript(scriptdata)
	script.EnableFileImport(true)
	script.SetImports(stdlib.GetModuleMap(stdlib.AllModuleNames()...))

	client.stats = append(client.stats, Stats{
		interval:   interval,
		command:    command,
		scriptpath: scriptpath,
		script:     script,
	})
}

// InitClient establishes the InfluxDB connection and sets up queues
func (client *Client) InitClient(host string, port int, database string, username string, password string, ssl bool) {
	var ssls string = ""
	if ssl {
		ssls = "s"
	}

	url := fmt.Sprintf("http%s://%s:%d", ssls, host, port)
	client.influxDb = influxdb2.NewClientWithOptions(url, fmt.Sprintf("%s:%s", username, password),
		influxdb2.DefaultOptions().
			SetUseGZip(true).
			SetTLSConfig(&tls.Config{
				InsecureSkipVerify: true,
			}))

	client.database = database
	client.tengoGlobals = make(map[string]interface{})
}

func (client *Client) runInvokedStat(stat Stats) {
	log.Printf("STATS: Running %s\n", stat.command)
	client.Rcon.SendCallback(stat.command, stat.interval-1, func(response *webrcon.Response) {
		log.Printf("Running callback for %s\n", stat.command)
		_ = stat.script.Add("_INPUT", response.Message)
		_ = stat.script.Add("_TAG", client.Tag)

		client.tengomu.Lock()
		_ = stat.script.Add("_GLOBALS", client.tengoGlobals)
		client.tengomu.Unlock()

		compiled, err := stat.script.Run()
		if err != nil {
			log.Printf("Error running tengo script: %s\n", err)
			return
		}

		client.tengomu.Lock()
		client.tengoGlobals = compiled.Get("_GLOBALS").Map()
		client.tengomu.Unlock()

		var bucket string
		b := compiled.Get("_BUCKET")
		if b == nil {
			bucket = "autogen"
		} else {
			bucket = b.String()
		}

		writeAPI := client.influxDb.WriteAPIBlocking(
			"", fmt.Sprintf("%s/%s", client.database, bucket))

		measurements := compiled.Get("_MEASUREMENTS")
		if measurements != nil {
			for _, m := range measurements.Array() {
				mstring := fmt.Sprintf("%v", m)
				log.Printf("Writing out record %s\n", mstring)
				writeAPI.WriteRecord(context.Background(), mstring)
			}
		} else {
			log.Printf("Warning: %s:%s returned no _MEASUREMENTS\n", stat.command, stat.scriptpath)
		}
	})
}

// CollectStats begins running the configured stats.
func (client *Client) CollectStats(done chan struct{}) {
	var ticks int64

	for {
		ticks++

		for _, stat := range client.stats {
			if ticks%int64(stat.interval) == 0 {
				client.runInvokedStat(stat)
			}
		}

		time.Sleep(1 * time.Second)
		log.Printf("STATS: Tick Count: %d\n", ticks)
	}
}
