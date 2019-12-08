# prometheus-waze-exporter
Simple Prometheus exporter to monitor the travel distance and duration thanks to Waze API

## Purpose

It has been made to monitor when is the best time to commute to avoid traffic jam.

## Install

Having a working [Golang](https://golang.org/) environment:

```bash
go get github.com/trazfr/prometheus-waze-exporter
go install github.com/trazfr/prometheus-waze-exporter
```

## Use

It performs a query to Waze to compute 2 main metrics:

 - `waze_travel_distance_meters`
 - `waze_travel_time_seconds`

It needs a configuration file to define which travel should be monitored.

To run it, just `prometheus-waze-exporter config.json`

### Example of configuration file

config.json:
```json
{
    "from": {
        "name": "paris",
        "address": "55 Rue du Faubourg Saint-Honoré, Paris, France"
    },
    "to": {
        "name": "versailles",
        "address": "Place d'Armes, Versailles, France"
    },
    "listen": ":9091",
    "region": "row",
    "vehicle": "motorcycle",
    "avoid_toll": true,
    "avoid_subscription_road": true,
    "avoid_ferry": true,
    "bidirectional": true,
    "sleep": 500
}
```

- `region` may be:
  - `us` for the United States
  - `il` for Israel
  - `row`, this is the default value

- `vehicle` may be:
  - empty (`""`), it is a regular car. This is the default value if not defined
  - `taxi`
  - `motorcycle`

- `avoid_toll`, `avoid_subscription_road` and `avoid_ferry` are booleans. Their default value is `false`.

- `bidirectional` is a boolean. Its default value is `true`

- `sleep` is an integer. It represents the number of milliseconds to wait between two calls to Waze API. Its default value is 500ms.

- `listen` is `:9091` if unset, so you may configure in your scrape config if Prometheus is running on the same server:
```
- job_name: prometheus-waze-exporter
  scrape_timeout: 1m
  scrape_interval: 5m
  static_configs:
  - targets: ['127.0.0.1:9091']
```
