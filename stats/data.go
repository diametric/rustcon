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
	Test           bool
	influxDb       influxdb2.Client
	database       string
	stats          []*Stats
	internalStats  []*InternalStats
	monitoredStats []*MonitoredStats
}

// StatsImpl defines the data all stat types use.
type StatsImpl struct {
	scriptpath string
	script     *tengo.Compiled
	modTime    int64
}

// InternalStats stats, or rather stats that just run at an interval with not RCON command.
type InternalStats struct {
	StatsImpl
	interval int
}

// MonitoredStats style stats.
type MonitoredStats struct {
	StatsImpl
	pattern         string
	patternCompiled *regexp.Regexp
}

// Stats contains the configured stats plugins
type Stats struct {
	StatsImpl
	interval int
	command  string
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

// TengoTagEscape defines the object type for escaping tag values
type TengoTagEscape struct {
	tengo.ObjectImpl
}

// TengoFieldEscape defines the object for escaping field values
type TengoFieldEscape struct {
	tengo.ObjectImpl
}
