package stats

import (
	"regexp"
	"sync"

	"github.com/d5/tengo"
	"github.com/diametric/rustcon/webrcon"
	influxdb2 "github.com/influxdata/influxdb-client-go"
)

// Client maintains the InfluxDB client connection
type Client struct {
	Tag            string
	Rcon           *webrcon.RconClient
	influxDb       influxdb2.Client
	database       string
	stats          []Stats
	internalStats  []InternalStats
	monitoredStats []MonitoredStats
	tengoGlobals   map[string]interface{}
	tengomu        sync.Mutex
}

// InternalStats stats, or rather stats that just run at an interval with not RCON command.
type InternalStats struct {
	interval   int
	scriptpath string
	script     *tengo.Script
}

// MonitoredStats style stats.
type MonitoredStats struct {
	pattern         string
	patternCompiled *regexp.Regexp
	scriptpath      string
	script          *tengo.Script
}

// Stats contains the configured stats plugins
type Stats struct {
	interval   int
	command    string
	scriptpath string
	script     *tengo.Script
}
