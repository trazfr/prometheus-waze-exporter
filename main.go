package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type wazeMetric struct {
	wazeParameters     WazeParameters
	wazeRequest        *WazeRequest
	timeTravelTime     prometheus.Gauge
	timeTravelDistance prometheus.Gauge
}

type context struct {
	bidir          bool
	sleepTime      time.Duration
	listen         string
	wazeMetrics    []*wazeMetric
	wazeTimeSpent  prometheus.Counter
	wazeCallsOk    prometheus.Counter
	wazeCallsKo    prometheus.Counter
	wazeParameters prometheus.Counter
}

const (
	namespace = "waze"
)

var (
	promWazeTravelTime = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "travel_time_seconds",
		Help:      "travel time in seconds",
	}, []string{"from", "to"})
	promWazeTravelDistance = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "travel_distance_meters",
		Help:      "travel distance in meters",
	}, []string{"from", "to"})
	promWazeCalls = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "api_calls",
		Help:      "number of calls to the Waze API",
	}, []string{"status"})
	promWazeParams = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "parameters",
		Help:      "Waze parameters",
	}, []string{"from", "to", "region", "sleep", "vehicle", "avoid_toll", "avoid_subscription_road", "avoid_ferry", "bidirectional"})
	promWazeTimeSpent = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "time_seconds",
		Help:      "total time spent to to process Waze API",
	})
)

func (c *context) Describe(ch chan<- *prometheus.Desc) {
	for _, metric := range c.wazeMetrics {
		metric.describe(ch)
	}
	c.wazeCallsOk.Describe(ch)
	c.wazeCallsKo.Describe(ch)
	c.wazeTimeSpent.Describe(ch)
	c.wazeParameters.Describe(ch)
}

func (c *context) Collect(ch chan<- prometheus.Metric) {
	sleep := false
	for _, metric := range c.wazeMetrics {
		if sleep {
			time.Sleep(c.sleepTime)
		}
		duration, err := metric.collect(ch)
		if err == nil {
			c.wazeCallsOk.Inc()
		} else {
			c.wazeCallsKo.Inc()
		}
		c.wazeTimeSpent.Add(duration.Seconds())
		sleep = true
	}
	c.wazeCallsOk.Collect(ch)
	c.wazeCallsKo.Collect(ch)
	c.wazeTimeSpent.Collect(ch)
	c.wazeParameters.Collect(ch)
}

func (w *wazeMetric) describe(ch chan<- *prometheus.Desc) {
	w.timeTravelDistance.Describe(ch)
	w.timeTravelTime.Describe(ch)
}

func (w *wazeMetric) collect(ch chan<- prometheus.Metric) (time.Duration, error) {
	begin := time.Now()
	result, err := w.wazeRequest.Call()
	duration := time.Now().Sub(begin)
	if err != nil {
		// dont change the values
		log.Println(w.wazeParameters.From.Name, w.wazeParameters.To.Name, err)
	} else if len(result) > 0 {
		w.timeTravelDistance.Set(float64(result[0].Distance))
		w.timeTravelTime.Set(math.Round(result[0].Duration.Seconds()))
	}
	w.timeTravelDistance.Collect(ch)
	w.timeTravelTime.Collect(ch)
	return duration, err
}

func createWazeMetrics(wazeParameters WazeParameters, client *http.Client, switchDirection bool) *wazeMetric {
	var err error
	r := &wazeMetric{
		wazeParameters: wazeParameters,
	}
	if switchDirection {
		r.wazeParameters.From, r.wazeParameters.To = wazeParameters.To, wazeParameters.From
	}

	log.Println("Create metrics from", r.wazeParameters.From.Name, "to", r.wazeParameters.To.Name)
	r.timeTravelDistance = promWazeTravelDistance.WithLabelValues(r.wazeParameters.From.Name, r.wazeParameters.To.Name)
	r.timeTravelTime = promWazeTravelTime.WithLabelValues(r.wazeParameters.From.Name, r.wazeParameters.To.Name)
	r.wazeRequest, err = CreateRequest(r.wazeParameters, client)

	if err != nil {
		log.Fatalln(err)
	}
	return r
}

func getContext(filename string, client *http.Client) context {
	fd, err := os.Open(filename)
	if err != nil {
		log.Fatalln(err)
	}
	jsonConfig := struct {
		WazeParameters
		Bidirectional bool   `json:"bidirectional"`
		Listen        string `json:"listen"`
		Sleep         int64  `json:"sleep"`
	}{
		Bidirectional: true,
		Listen:        ":9091",
		Sleep:         500,
	}
	if err := json.NewDecoder(fd).Decode(&jsonConfig); err != nil {
		log.Fatalln(err)
	}

	context := context{
		bidir:         jsonConfig.Bidirectional,
		sleepTime:     time.Millisecond * time.Duration(jsonConfig.Sleep),
		listen:        jsonConfig.Listen,
		wazeMetrics:   []*wazeMetric{createWazeMetrics(jsonConfig.WazeParameters, client, false)},
		wazeTimeSpent: promWazeTimeSpent,
		wazeCallsOk:   promWazeCalls.WithLabelValues("ok"),
		wazeCallsKo:   promWazeCalls.WithLabelValues("ko"),
		wazeParameters: promWazeParams.WithLabelValues(jsonConfig.From.Name,
			jsonConfig.To.Name,
			jsonConfig.Region.String(),
			strconv.FormatInt(jsonConfig.Sleep, 10),
			jsonConfig.Vehicle.String(),
			strconv.FormatBool(jsonConfig.AvoidToll),
			strconv.FormatBool(jsonConfig.AvoidSubscriptionRoad),
			strconv.FormatBool(jsonConfig.AvoidFerry),
			strconv.FormatBool(jsonConfig.Bidirectional),
		),
	}
	context.wazeParameters.Inc()
	if jsonConfig.Bidirectional {
		context.wazeMetrics = append(context.wazeMetrics, createWazeMetrics(jsonConfig.WazeParameters, client, true))
	}
	return context
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "Usage", os.Args[0], "<config_file>")
		os.Exit(1)
	}

	client := &http.Client{
		Timeout: time.Second * 10,
	}
	context := getContext(os.Args[1], client)

	prometheus.MustRegister(&context)
	http.Handle("/metrics", promhttp.Handler())
	log.Println(http.ListenAndServe(context.listen, nil))
}
