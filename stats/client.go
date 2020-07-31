package stats

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/d5/tengo"
	"github.com/d5/tengo/stdlib"

	"github.com/diametric/rustcon/webrcon"

	influxdb2 "github.com/influxdata/influxdb-client-go"
)

// Client maintains the InfluxDB client connection
type Client struct {
	Tag      string
	Rcon     *webrcon.RconClient
	influxDb influxdb2.Client
	queue    chan string
	stats    []Stats
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
		fmt.Printf("Unable to load script from %s for %s, registration ignored.", scriptpath, command)
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
	var ssls string
	if ssl {
		ssls = "s"
	} else {
		ssls = ""
	}

	url := fmt.Sprintf("http%s://%s:%d", ssls, host, port)
	client.influxDb = influxdb2.NewClientWithOptions(url, fmt.Sprintf("%s:%s", username, password),
		influxdb2.DefaultOptions().
			SetUseGZip(true).
			SetTLSConfig(&tls.Config{
				InsecureSkipVerify: true,
			}))

	client.queue = make(chan string)
}

func (client *Client) runInvokedStat(stat Stats) {
	fmt.Printf("STATS: Running %s\n", stat.command)
	client.Rcon.SendCallback(stat.command, func(response *webrcon.Response) {
		_ = stat.script.Add("_INPUT", response.Message)
		_ = stat.script.Add("_TAG", client.Tag)

		compiled, err := stat.script.RunContext(context.Background())
		if err != nil {
			fmt.Printf("Error running tengo script: %s\n", err)
			return
		}

		writeAPI := client.influxDb.WriteAPIBlocking("", "blueberry_v2/autogen")

		measurements := compiled.Get("_MEASUREMENTS")
		for _, m := range measurements.Array() {
			mstring := fmt.Sprintf("%v", m)
			fmt.Printf("Writing out record %s\n", mstring)
			writeAPI.WriteRecord(context.Background(), mstring)
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
	}
}