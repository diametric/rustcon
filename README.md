# rustcon

rustcon is a middleware application for [Rust servers](https://rust.facepunch.com/). It aims to provide two core
components to assist in generating graphs and providing an informational middleware layer for other applications.

# Table of contents

* [Stats collection with InfluxDB](#stats-collection-with-influxdb)
* [Redis based middleware](#redis-based-middleware)
* [Quickstart](#quickstart)

## Stats collection with InfluxDB

rustcon supports collecting stats into an [InfluxDB v1.8+](https://www.influxdata.com/) database. Stat collection is
split into three types:

* **internal**: Internal stats are run at a predefined interval, and do not send any RCON commands or receive any RCON
  data.
* **invoked**: Invoked stats are run at a predefined interval, and a RCON command is supplied to run, returning the
  input.
* **monitored**: Monitored stats listen for strings to match a predefined regular expression, and then run the stat
  based on those matches.

### Scripting language

All stat logic is defined by creating a [Tengo script](https://github.com/d5/tengo). The tengo script is run, passing in
inputs depending on the type of stat, and then defines a special `_MEASUREMENTS` array
containing [InfluxDB line protocol](https://docs.influxdata.com/influxdb/v1.8/write_protocols/line_protocol_tutorial/)
data to insert. More details about how to write scripts are documented in
the [example.tengo](https://github.com/diametric/rustcon/blob/master/scripts/example.tengo) script. Tengo scripts can be
updated without restarting the application, new changes will be detected and reloaded.

**Example script:**

```tengo
text := import("text")

// Split out the "30 FPS" string to get the value.
parts := text.fields(_INPUT)

_BUCKET := "sixty_days"
_MEASUREMENTS := [format("serverfps,servertag=%s fps=%s", _TAG, parts[0])]
```

**Configuration for the above script:**

```json
"invoked": [
    {
        "command": "server.fps",
        "script": "scripts/fps.tengo",
        "interval": 1
    }
]
```

This will run `server.fps` every second, parse the resulting data and insert it into the retention policy and
measurement `sixty_days.serverfps`

Note: Scripts are not required to define a `_MEASUREMENTS` array, and can be configured to simply cache data, run
commands, write files, etc. Their primary focus is parsing data into usable statistics, but they can be created for a
wider variety of tasks.

### Retention policies/buckets

Defining the InfluxDB retention policy/bucket is left up to each individual script. If you define a variable
named `_BUCKET` containing the retention policy you want the data written to. If no `_BUCKET` is defined, `autogen` is
used.

## Redis based middleware

The other function of rustcon is to provide a messaging queue system and data cache middleware. You can define RCON
commands to run at specific intervals, and have their data stored in a [Redis](https://redis.io) key. You can also
define multiple queues of raw RCON data to be consumed by other applications.

For example, you can define a queue named *middleware:livebot* and create a discord bot to begin reading that RCON data
to announce play joins/leaves, chat data, etc. You could then create a queue named *middleware:elklogger* to log all
RCON data into an [ELK stack](https://www.elastic.co/what-is/elk-stack).

All redis keys defined in the configuration will substitude the special variable `{tag}` with the tag supplied by
the `-tag` command line argument.

## Redis based RCON callback requests

If you enable the redis middleware, you can also use rustcon to send callback requests to RCON through redis.

By pushing a JSON structure to the defined `callback_queue_key` list, rustcon will run that command and set the results
in a key defined by the ID passed in the JSON structure. Example:

`LPUSH [callback_queue_key] {"id": 5, "command": "playerlist"}`

The contents of the RCON command `playerlist` will be set into the key `[callback_queue_key]:results:5`

# Quickstart

1. Edit the configuration file with your InfluxDB and Redis credentials. If you have only one or the other, and don't
   desire that modules functionality, you can disable it with the `enable_influx_stats` and `enable_redis_queue`
   configuration options. Keep in mind at least one of these must be true for rustcon to work.
2. Create an text file containing your servers RCON password on a single line. Remember this location.
3. Run rustcon with the appropriate hostname and port parameters as well as the path to your RCON password file, for
   example:

Linux:

```sh
$ ./rustcon -hostname rustserver-ip.com -port 28016 -passfile /path/to/your/rcon/passwordfile.txt
```

Windows:

```sh
C:\rustcon> rustcon.exe -hostname rustserver-ip.com -port 28016 -passfile /path/to/your/rcon/passwordfile.txt
```

4. Load up your [favorite graphing software](https://grafana.com/) and enjoy your stats!