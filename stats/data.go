package stats

import (
	"regexp"

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
	stats          []*Stats
	internalStats  []*InternalStats
	monitoredStats []*MonitoredStats
}

// InternalStats stats, or rather stats that just run at an interval with not RCON command.
type InternalStats struct {
	interval   int
	scriptpath string
	script     *tengo.Compiled
	modTime    int64
}

// MonitoredStats style stats.
type MonitoredStats struct {
	pattern         string
	patternCompiled *regexp.Regexp
	scriptpath      string
	script          *tengo.Compiled
	modTime         int64
}

// Stats contains the configured stats plugins
type Stats struct {
	interval   int
	command    string
	scriptpath string
	script     *tengo.Compiled
	modTime    int64
}

// Tengo related data structures

// TengoLogger defines the object type for the logger functions
type TengoLogger struct {
	tengo.ObjectImpl
}

// TengoGlobals defines the object that holds globals. We need this to enforce
// concurrency safety.
type TengoGlobals struct {
	tengo.ObjectImpl
}

// TengoLock defines the object type for a generic mutex lock function
type TengoLock struct {
	tengo.ObjectImpl
}

// TengoUnlock defines the object type for a generic mutex unlock function
type TengoUnlock struct {
	tengo.ObjectImpl
}
