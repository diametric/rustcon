**rustcon** is a Rust RCON middleware application designed to alleviate the need
for numerous clients connecting to RCON, and provide messaging queue mechanism
for RCON data.  In addition to this, it optionally provides a mechanism to query
RCON periodically for stats data and to maintain cached copies of various output.

For example, you can have it run `serverinfo` every X seconds, and maintain that
output in a specified redis key to be available to websites, bots, or wahtever.

You can also build statistic collection scripts using [Tengo](https://github.com/d5/tengo/blob/master/docs/tutorial.md) to parse arbitrarily configured RCON commands, or even pattern
match against any output.

Example from default config:
```json
"stats": {
    "invoked": [
        {
            "command": "server.fps",
            "script": "scripts/fps.tengo",
            "interval": 1
        }
    ]
}```

scripts/fps.tengo:
```tengo
text := import("text")

// Split out the "30 FPS" string to get the value.
parts := text.fields(_INPUT)

_MEASUREMENTS := [format("serverfps,servertag=%s fps=%s", _TAG, parts[0])]
```

In this example, we run `server.fps` every second, and call fps.tengo to parse
the output and provide [InfluxDB Line Protocol](https://docs.influxdata.com/influxdb/v1.8/write_protocols/line_protocol_tutorial/) formatted data back to the engine to
insert.

This method allows you to configure any arbitrary kind of stat collection without
the need to modify the underlying engine.
