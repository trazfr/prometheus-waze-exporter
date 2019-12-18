package main

import (
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
	}, []string{"region", "sleep", "vehicle", "avoid_toll", "avoid_subscription_road", "avoid_ferry"})
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
		log.Println("Error", w.timeTravelTime.Desc().String(), err)
	} else if len(result) > 0 {
		w.timeTravelDistance.Set(float64(result[0].Distance))
		w.timeTravelTime.Set(math.Round(result[0].Duration.Seconds()))
	}
	w.timeTravelDistance.Collect(ch)
	w.timeTravelTime.Collect(ch)
	return duration, err
}

func createWazeCoordinates(addresses map[string]string, region Region, client *http.Client) map[string]string {
	result := map[string]string{}
	for name, address := range addresses {
		coordinates, err := WazeAddressToQuery(address, region, client)
		log.Println("Address", address, "has been found at", coordinates)
		if err != nil {
			log.Fatalln("Failed to retrieve the address", address, err)
		}
		result[name] = coordinates
	}
	return result
}

func getContext(filename string, client *http.Client) context {
	jsonConfig := NewConfig(filename)

	context := context{
		sleepTime:     time.Millisecond * time.Duration(jsonConfig.Sleep),
		listen:        jsonConfig.Listen,
		wazeTimeSpent: promWazeTimeSpent,
		wazeCallsOk:   promWazeCalls.WithLabelValues("ok"),
		wazeCallsKo:   promWazeCalls.WithLabelValues("ko"),
		wazeParameters: promWazeParams.WithLabelValues(
			jsonConfig.Region.String(),
			strconv.FormatInt(jsonConfig.Sleep, 10),
			jsonConfig.Vehicle.String(),
			strconv.FormatBool(jsonConfig.AvoidToll),
			strconv.FormatBool(jsonConfig.AvoidSubscriptionRoad),
			strconv.FormatBool(jsonConfig.AvoidFerry),
		),
	}

	log.Println("Look for", len(jsonConfig.Addresses), "addresses")
	coordinates := createWazeCoordinates(jsonConfig.Addresses, jsonConfig.Region, client)

	log.Println("Create", len(jsonConfig.Paths), "paths")
	for _, path := range jsonConfig.Paths {
		fromCoordinates, fromFound := coordinates[path.From]
		if !fromFound {
			log.Fatalln("Address not found:", path.From)
		}
		toCoordinates, toFound := coordinates[path.To]
		if !toFound {
			log.Fatalln("Address not found:", path.To)
		}

		wazeMetric := &wazeMetric{
			wazeParameters: WazeParameters{
				FromCoordinates:       fromCoordinates,
				ToCoordinates:         toCoordinates,
				Region:                jsonConfig.Region,
				Vehicle:               jsonConfig.Vehicle,
				AvoidToll:             jsonConfig.AvoidToll,
				AvoidSubscriptionRoad: jsonConfig.AvoidSubscriptionRoad,
				AvoidFerry:            jsonConfig.AvoidFerry,
			},
			timeTravelTime:     promWazeTravelTime.WithLabelValues(path.From, path.To),
			timeTravelDistance: promWazeTravelDistance.WithLabelValues(path.From, path.To),
		}
		var err error
		wazeMetric.wazeRequest, err = CreateRequest(wazeMetric.wazeParameters, client)
		if err != nil {
			log.Fatalln(err)
		}
		context.wazeMetrics = append(context.wazeMetrics, wazeMetric)
	}

	context.wazeParameters.Inc()
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
