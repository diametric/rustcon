package stats

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/d5/tengo"
	"github.com/d5/tengo/stdlib"
	"go.uber.org/zap"

	"github.com/diametric/rustcon/webrcon"
	"github.com/fatih/structs"

	influxdb2 "github.com/influxdata/influxdb-client-go"
)

func (client *Client) checkNeedReload(scriptpath string, modTime int64) (bool, int64) {
	file, err := os.Stat(scriptpath)
	if err != nil {
		zap.S().Errorf("Error checking file modification time for reload on %s: %s", scriptpath, err)
		// We don't want to try to reload the script if doing so would fail. Since
		// we can't Stat() it, likely we can't read it either.
		return false, 0
	}

	if modTime != file.ModTime().Unix() {
		return true, file.ModTime().Unix()
	}

	return false, 0
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

// RegisterMonitoredStat registers a stat based on monitoring the RCON data.
func (client *Client) RegisterMonitoredStat(pattern string, scriptpath string) {
	file, err := os.Stat(scriptpath)
	if err != nil {
		zap.S().Errorf("Error getting file modification time on %s: %s", scriptpath, err)
		return
	}

	script, err := client.getScript(scriptpath)
	if err != nil {
		zap.S().Errorf("Unable to add internal stat %s, error reading script: %s", scriptpath, err)
		return
	}

	compiled, err := regexp.Compile(pattern)
	if err != nil {
		zap.S().Errorf("Unable to compile monitored regex %s: %s", pattern, err)
		return
	}

	client.monitoredStats = append(client.monitoredStats, &MonitoredStats{
		pattern:         pattern,
		patternCompiled: compiled,
		scriptpath:      scriptpath,
		script:          script,
		modTime:         file.ModTime().Unix(),
	})

	zap.S().Infof("Registered monitored stat, pattern = %s, script = %s", pattern, scriptpath)
}

// RegisterInternalStat registers an internal type stat.
func (client *Client) RegisterInternalStat(scriptpath string, interval int) {
	file, err := os.Stat(scriptpath)
	if err != nil {
		zap.S().Errorf("Error getting file modification time on %s: %s", scriptpath, err)
		return
	}

	script, err := client.getScript(scriptpath)
	if err != nil {
		zap.S().Warnf("Unable to add internal stat %s, error reading script: %s", scriptpath, err)
		return
	}

	client.internalStats = append(client.internalStats, &InternalStats{
		interval:   interval,
		scriptpath: scriptpath,
		script:     script,
		modTime:    file.ModTime().Unix(),
	})

	zap.S().Infof("Registered internal stat, interval = %d, script = %s", interval, scriptpath)
}

// RegisterInvokedStat registers an invoked type stat.
func (client *Client) RegisterInvokedStat(command string, scriptpath string, interval int) {
	file, err := os.Stat(scriptpath)
	if err != nil {
		zap.S().Errorf("Error getting file modification time on %s: %s", scriptpath, err)
		return
	}

	script, err := client.getScript(scriptpath)
	if err != nil {
		zap.S().Warnf("Unable to add invoked stat %s, error reading script: %s", scriptpath, err)
		return
	}

	client.stats = append(client.stats, &Stats{
		interval:   interval,
		command:    command,
		scriptpath: scriptpath,
		script:     script,
		modTime:    file.ModTime().Unix(),
	})

	zap.S().Infof("Registered invoked stat, command = %s, interval = %d, script = %s", command, interval, scriptpath)
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
	_ = script.Add("logger", &TengoLogger{})

	client.tengomu.Lock()
	_ = script.Add("_GLOBALS", client.tengoGlobals)
	client.tengomu.Unlock()

	compiled, err := script.Run()
	if err != nil {
		zap.S().Errorf("Error running tengo script: %s", err)
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
		var linedata strings.Builder
		for _, m := range measurements.Array() {
			linedata.WriteString(fmt.Sprintf("%v\n", m))
		}

		writeAPI.WriteRecord(context.Background(), linedata.String())
	}
}

func (client *Client) runInternalStat(stat *InternalStats) {
	if needs, modtime := client.checkNeedReload(stat.scriptpath, stat.modTime); needs {
		var err error

		zap.S().Infof("Change detected in %s, reloading", stat.scriptpath)
		stat.script, err = client.getScript(stat.scriptpath)
		if err != nil {
			zap.S().Errorf("Error reloading new script %s: %s", stat.scriptpath, err)
			return
		}

		stat.modTime = modtime
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	runtimeMem := make(map[string]interface{})
	runtimeMem["alloc"] = int64(m.Alloc)
	runtimeMem["totalAlloc"] = int64(m.TotalAlloc)
	runtimeMem["sys"] = int64(m.Sys)
	runtimeMem["numGC"] = int64(m.NumGC)

	stat.script.Add("_SCRIPT_TYPE", "internal")
	err := stat.script.Add("_RUNTIME_STATS", runtimeMem)
	if err != nil {
		zap.S().Errorf("ERROR: Couldn't populate _RUNTIME_STATS: %s", err)
	}
	_ = stat.script.Add("_RCON_STATS", structs.Map(client.Rcon.Stats))

	client.runScript(stat.script)
}

func (client *Client) runInvokedStat(stat *Stats) {
	zap.S().Debugf("STATS: Running %s", stat.command)

	client.Rcon.SendCallback(stat.command, stat.interval-1, func(response *webrcon.Response) {
		if needs, modtime := client.checkNeedReload(stat.scriptpath, stat.modTime); needs {
			var err error

			zap.S().Infof("Change detected in %s, reloading", stat.scriptpath)
			stat.script, err = client.getScript(stat.scriptpath)
			if err != nil {
				zap.S().Errorf("Error reloading new script %s: %s", stat.scriptpath, err)
				return
			}

			stat.modTime = modtime
		}

		zap.S().Debugf("Running callback for %s", stat.command)

		_ = stat.script.Add("_SCRIPT_TYPE", "invoked")
		_ = stat.script.Add("_INPUT", response.Message)
		client.runScript(stat.script)
	})
}

// OnMessageMonitoredStat implements the RCON client OnMessage callback, to be
// used for Monitored Stats.
func (client *Client) OnMessageMonitoredStat(message []byte) {
	var r webrcon.Response

	if err := json.Unmarshal(message, &r); err != nil {
		zap.S().Error("Error decoding RCON websocket response in OnMessage callback, this shouldn't should never happen here.")
	}

	for _, v := range client.monitoredStats {
		zap.S().Debugf("MONITORED STATS: Checking if %s matches %s", v.pattern, r.Message)
		re := v.patternCompiled.FindStringSubmatch(r.Message)
		if re != nil {
			if needs, modtime := client.checkNeedReload(v.scriptpath, v.modTime); needs {
				var err error

				zap.S().Infof("Change detected in %s, reloading", v.scriptpath)
				v.script, err = client.getScript(v.scriptpath)
				if err != nil {
					zap.S().Errorf("Error reloading new script %s: %s", v.scriptpath, err)
					continue
				}

				v.modTime = modtime
			}

			converted := make([]interface{}, len(re))
			for i, vv := range re {
				converted[i] = vv
			}

			_ = v.script.Add("_SCRIPT_TYPE", "monitored")
			err := v.script.Add("_MATCHES", converted)
			if err != nil {
				zap.S().Errorf("STATS: Unable to add _MATCHES variable to script: %s", err)
			}
			err = v.script.Add("_RESPONSE", structs.Map(r))
			if err != nil {
				zap.S().Errorf("STATS: Unable to add _RESPONSE variable to script: %s", err)
			}

			client.runScript(v.script)
		}
	}
}

// CollectStats begins running the configured stats.
func (client *Client) CollectStats(done chan struct{}, wg *sync.WaitGroup) {
	var ticks int64

	zap.S().Info("Starting up stats collector.")
	wg.Add(1)

	for {
		select {
		case <-done:
			zap.S().Info("Shutting down stats collector.")
			wg.Done()
			return
		default:
		}

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

		zap.S().Debugf("STATS: Tick Count: %d", ticks)
	}
}
