package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type WazeParameters struct {
	FromCoordinates       string
	ToCoordinates         string
	Region                Region
	Vehicle               Vehicle
	AvoidToll             bool
	AvoidSubscriptionRoad bool
	AvoidFerry            bool
}

type WazeRequest struct {
	client     *http.Client
	routingURL string
}

type WazeResult struct {
	Duration time.Duration
	Distance int
}

const (
	wazeScheme = "https"
	wazeHost   = "www.waze.com"

	wazeReferer = wazeScheme + "://" + wazeHost + "/"
)

var (
	coordServers = map[Region]string{
		US:  "SearchServer/mozi",
		IL:  "il-SearchServer/mozi",
		ROW: "row-SearchServer/mozi",
	}
	routingServers = map[Region]string{
		US:  "RoutingManager/routingRequest",
		IL:  "il-RoutingManager/routingRequest",
		ROW: "row-RoutingManager/routingRequest",
	}
)

func CreateRequest(wazeParam WazeParameters, client *http.Client) (*WazeRequest, error) {
	param := url.Values{}
	if vehicle := marshalVehicleMap[wazeParam.Vehicle]; vehicle != "" {
		param.Set("vehicleType", vehicle)
	}
	options := []string{"AVOID_TRAILS:t"}
	if wazeParam.AvoidToll {
		options = append(options, "AVOID_TOLL_ROADS:t")
	}
	if wazeParam.AvoidFerry {
		options = append(options, "AVOID_FERRIES:t")
	}
	param.Set("options", strings.Join(options, ","))
	if !wazeParam.AvoidSubscriptionRoad {
		param.Set("subscription", "*")
	}

	param.Set("from", wazeParam.FromCoordinates)
	param.Set("to", wazeParam.ToCoordinates)
	param.Set("at", "0")
	param.Set("returnJSON", "true")
	param.Set("timeout", "60000")
	param.Set("nPaths", "1")

	u := url.URL{
		Scheme:   wazeScheme,
		Host:     wazeHost,
		Path:     routingServers[wazeParam.Region],
		RawQuery: param.Encode(),
	}

	log.Println("Result query", u.String())
	return &WazeRequest{
		client:     client,
		routingURL: u.String(),
	}, nil
}

func decodeWazeRoutingResponse(w *wazeRoutingInnerResponse) WazeResult {
	sumLength := 0
	for _, segment := range w.Results {
		sumLength += segment.Length
	}
	return WazeResult{
		Duration: time.Duration(w.TotalRouteTime) * time.Second,
		Distance: sumLength,
	}
}

func (w *WazeRequest) Call() ([]WazeResult, error) {
	log.Println("Call", w.routingURL)
	req, err := http.NewRequest("GET", w.routingURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", wazeReferer)

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Got HTTP %d %s", resp.StatusCode, resp.Status)
	}

	decodedResponse := wazeRoutingResponse{}
	if err := json.NewDecoder(resp.Body).Decode(&decodedResponse); err != nil {
		return nil, err
	}

	var result []WazeResult
	if decodedResponse.Response != nil {
		result = append(result, decodeWazeRoutingResponse(decodedResponse.Response))
	}
	for _, resp := range decodedResponse.Alternatives {
		result = append(result, decodeWazeRoutingResponse(&resp.Response))
	}

	return result, nil
}

func WazeAddressToQuery(address string, region Region, client *http.Client) (string, error) {
	log.Println("Look for address", address)
	param := url.Values{}
	param.Set("q", address)
	param.Set("lat", "0")
	param.Set("lon", "0")

	u := url.URL{
		Scheme:   wazeScheme,
		Host:     wazeHost,
		Path:     coordServers[region],
		RawQuery: param.Encode(),
	}
	log.Println("Call", u.String())
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Referer", wazeReferer)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Got HTTP %d %s", resp.StatusCode, resp.Status)
	}

	decodedResponse := []wazeCoordResponse{}
	if err := json.NewDecoder(resp.Body).Decode(&decodedResponse); err != nil {
		return "", err
	}
	for i := range decodedResponse {
		item := &decodedResponse[i]
		if item.Name != "" {
			return fmt.Sprintf("x:%f y:%f", item.Location.Lon, item.Location.Lat), nil
		}
	}

	return "", fmt.Errorf("Address not found: %s", address)
}

////////////////////////////////////////////////////////////////////////////////
// Region
////////////////////////////////////////////////////////////////////////////////

type Region int

const (
	ROW Region = iota
	US
	IL
)

var marshalRegionMap = map[Region]string{
	US:  "US",
	IL:  "IL",
	ROW: "ROW",
}

var unmarshalRegionMap = map[string]Region{
	"US":  US,
	"IL":  IL,
	"ROW": ROW,
}

func (s Region) String() string {
	return marshalRegionMap[s]
}

func (s Region) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *Region) UnmarshalJSON(b []byte) error {
	var j string
	err := json.Unmarshal(b, &j)
	if err != nil {
		return err
	}
	if val, found := unmarshalRegionMap[strings.ToUpper(j)]; found {
		*s = val
		return nil
	}
	return errors.New("Cannot unmarshal " + j + " as region")
}

////////////////////////////////////////////////////////////////////////////////
// Vehicle
////////////////////////////////////////////////////////////////////////////////

type Vehicle int

const (
	Regular Vehicle = iota
	Taxi
	Motorcycle
)

var marshalVehicleMap = map[Vehicle]string{
	Regular:    "",
	Taxi:       "TAXI",
	Motorcycle: "MOTORCYCLE",
}

var unmarshalVehicleMap = map[string]Vehicle{
	"":           Regular,
	"TAXI":       Taxi,
	"MOTORCYCLE": Motorcycle,
}

func (s Vehicle) String() string {
	return marshalVehicleMap[s]
}

func (s Vehicle) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *Vehicle) UnmarshalJSON(b []byte) error {
	var j string
	err := json.Unmarshal(b, &j)
	if err != nil {
		return err
	}
	if val, found := unmarshalVehicleMap[strings.ToUpper(j)]; found {
		*s = val
		return nil
	}
	return errors.New("Cannot unmarshal " + j + " as vehicle")
}

////////////////////////////////////////////////////////////////////////////////
// wazeRoutingResponse
////////////////////////////////////////////////////////////////////////////////

type wazeRoutingResponse struct {
	Response     *wazeRoutingInnerResponse `json:"response"`
	Alternatives []wazeRoutingAlternative  `json:"alternatives"`
}

type wazeRoutingAlternative struct {
	Response wazeRoutingInnerResponse `json:"response"`
}

type wazeRoutingInnerResponse struct {
	Results        []wazeRoutingResult `json:"results"`
	TotalRouteTime int                 `json:"totalRouteTime"`
}

type wazeRoutingResult struct {
	Length int `json:"length"`
}

////////////////////////////////////////////////////////////////////////////////
// wazeCoordResponse
////////////////////////////////////////////////////////////////////////////////

type wazeCoordResponse struct {
	Name     string            `json:"name"`
	Location wazeCoordLocation `json:"location"`
}

type wazeCoordLocation struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}
