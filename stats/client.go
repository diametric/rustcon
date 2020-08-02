package stats

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"log"
	"regexp"
	"sync"
	"time"

	"github.com/d5/tengo"
	"github.com/d5/tengo/stdlib"

	"github.com/diametric/rustcon/webrcon"
	"github.com/fatih/structs"

	influxdb2 "github.com/influxdata/influxdb-client-go"
)

// Client maintains the InfluxDB client connection
type Client struct {
	Tag           string
	Rcon          *webrcon.RconClient
	influxDb      influxdb2.Client
	database      string
	stats         []Stats
	internalStats []InternalStats
	tengoGlobals  map[string]interface{}
	tengomu       sync.Mutex
}

// InternalStats stats, or rather stats that just run at an interval with not RCON command.
type InternalStats struct {
	interval   int
	scriptpath string
	script     *tengo.Script
}

// MonitoredStats style stats.
type MonitoredStats struct {
	pattern    *regexp.Regexp
	scriptpath string
	script     *tengo.Script
}

// Stats contains the configured stats plugins
type Stats struct {
	interval   int
	command    string
	scriptpath string
	script     *tengo.Script
}

func (client *Client) getScript(scriptpath string) (*tengo.Script, error) {
	scriptdata, err := ioutil.ReadFile(scriptpath)
	if err != nil {
		return nil, err
	}

	script := tengo.NewScript(scriptdata)
	script.EnableFileImport(true)
	script.SetImports(stdlib.GetModuleMap(stdlib.AllModuleNames()...))

	return script, nil
}

// RegisterInternalStat registers an internal type stat.
func (client *Client) RegisterInternalStat(scriptpath string, interval int) {
	script, err := client.getScript(scriptpath)
	if err != nil {
		fmt.Printf("Unable to add internal stat %s, error reading script: %s", scriptpath, err)
		return
	}

	client.internalStats = append(client.internalStats, InternalStats{
		interval:   interval,
		scriptpath: scriptpath,
		script:     script,
	})
}

// RegisterInvokedStat registers an invoked type stat.
func (client *Client) RegisterInvokedStat(command string, scriptpath string, interval int) {
	script, err := client.getScript(scriptpath)
	if err != nil {
		fmt.Printf("Unable to add invoked stat %s, error reading script: %s", scriptpath, err)
		return
	}

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

func (client *Client) runScript(script *tengo.Script) {
	_ = script.Add("_TAG", client.Tag)

	client.tengomu.Lock()
	_ = script.Add("_GLOBALS", client.tengoGlobals)
	client.tengomu.Unlock()

	compiled, err := script.Run()
	if err != nil {
		log.Printf("Error running tengo script: %s\n", err)
		return
	}

	client.tengomu.Lock()
	client.tengoGlobals = compiled.Get("_GLOBALS").Map()
	client.tengomu.Unlock()

	// Here we allow the individual script to determine the InfluxDB bucket
	// to store data. By default we use autogen.
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
	}
}

func (client *Client) runInternalStat(stat InternalStats) {
	//_ = stat.script.Add("_INTERNAL_STATS", structs.Map())
	_ = stat.script.Add("_RCON_STATS", structs.Map(client.Rcon.Stats))

	client.runScript(stat.script)
}

func (client *Client) runInvokedStat(stat Stats) {
	log.Printf("STATS: Running %s\n", stat.command)
	client.Rcon.SendCallback(stat.command, stat.interval-1, func(response *webrcon.Response) {
		log.Printf("Running callback for %s\n", stat.command)
		_ = stat.script.Add("_INPUT", response.Message)
		client.runScript(stat.script)
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

		for _, internalStat := range client.internalStats {
			if ticks%int64(internalStat.interval) == 0 {
				client.runInternalStat(internalStat)
			}
		}

		time.Sleep(1 * time.Second)
		log.Printf("STATS: Tick Count: %d\n", ticks)
	}
}
