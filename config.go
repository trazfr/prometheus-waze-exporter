package main

import (
	"encoding/json"
	"log"
	"os"
)

type Path struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type Config struct {
	Addresses             map[string]string `json:"addresses"`
	Paths                 []Path            `json:"paths"`
	Listen                string            `json:"listen"`
	Region                Region            `json:"region"`
	Vehicle               Vehicle           `json:"vehicle"`
	AvoidToll             bool              `json:"avoid_toll"`
	AvoidSubscriptionRoad bool              `json:"avoid_subscription_road"`
	AvoidFerry            bool              `json:"avoid_ferry"`
	Sleep                 int64             `json:"sleep"`
}

func NewConfig(filename string) *Config {
	fd, err := os.Open(filename)
	if err != nil {
		log.Fatalln(err)
	}
	defer fd.Close()

	config := &Config{
		Listen: ":9091",
		Sleep:  500,
	}
	if err := json.NewDecoder(fd).Decode(config); err != nil {
		log.Fatalln(err)
	}

	return config
}
