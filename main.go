package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
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
	bidir       bool
	listen      string
	wazeMetrics []*wazeMetric
}

const (
	namespace = "waze"
)

var (
	collectorVecTravelTime = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "travel_time_seconds",
		Help:      "travel time in seconds",
	}, []string{"from", "to"})
	collectorVecTravelDistance = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "travel_distance_meters",
		Help:      "travel distance in meters",
	}, []string{"from", "to"})
)

func (c *context) Describe(ch chan<- *prometheus.Desc) {
	for _, metric := range c.wazeMetrics {
		metric.describe(ch)
	}
}

func (c *context) Collect(ch chan<- prometheus.Metric) {
	sleep := false
	for _, metric := range c.wazeMetrics {
		if sleep {
			time.Sleep(time.Millisecond * 500)
		}
		metric.collect(ch)
		sleep = true
	}
}

func (w *wazeMetric) describe(ch chan<- *prometheus.Desc) {
	w.timeTravelDistance.Describe(ch)
	w.timeTravelTime.Describe(ch)
}

func (w *wazeMetric) collect(ch chan<- prometheus.Metric) {
	result, err := w.wazeRequest.Call()
	if err != nil {
		log.Println(w.wazeParameters.From.Name, w.wazeParameters.To.Name, err)
	} else if len(result) > 0 {
		w.timeTravelDistance.Set(float64(result[0].Distance))
		w.timeTravelTime.Set(math.Round(result[0].Duration.Seconds()))
		w.timeTravelDistance.Collect(ch)
		w.timeTravelTime.Collect(ch)
	}
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
	r.timeTravelDistance = collectorVecTravelDistance.WithLabelValues(r.wazeParameters.From.Name, r.wazeParameters.To.Name)
	r.timeTravelTime = collectorVecTravelTime.WithLabelValues(r.wazeParameters.From.Name, r.wazeParameters.To.Name)
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
	}{
		Bidirectional: true,
		Listen:        ":9091",
	}
	if err := json.NewDecoder(fd).Decode(&jsonConfig); err != nil {
		log.Fatalln(err)
	}

	context := context{
		listen:      jsonConfig.Listen,
		bidir:       jsonConfig.Bidirectional,
		wazeMetrics: []*wazeMetric{createWazeMetrics(jsonConfig.WazeParameters, client, false)},
	}
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
