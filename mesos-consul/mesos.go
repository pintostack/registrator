package mesosconsul

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	// JSON structures for mesos state.json
	"github.com/CiscoCloud/mesos-consul/state"
)

// Mesos service
type Mesos struct {
	MasterUrl string // mesos master's URL
	State     state.State

	httpClient *http.Client
}

// Create new mesos instance
func NewMesos(url string) *Mesos {
	m := new(Mesos)
	m.MasterUrl = url
	m.httpClient = new(http.Client)
	return m
}

// Refresh mesos state
func (m *Mesos) Refresh() error {
	log.Print("mesos: updating state...")
	st, err := m.loadState(m.MasterUrl)
	if err != nil {
		log.Printf("mesos: failed to load state: %s", err)
		return fmt.Errorf("failed to load state: %s", err)
	}

	log.Printf("mesos: got state updated: %#v", st)
	m.State = st
	return nil // OK
}

// Load mesos state from state.json
func (m *Mesos) loadState(host string) (st state.State, err error) {
	// prepare GET request
	url := host + "/master/state.json"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return st, fmt.Errorf("failed to create GET /master/state.json: %s", err)
	}
	req.Header.Set("Content-Type", "application/json")

	log.Printf("mesos: getting state from: %q", url)
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return st, fmt.Errorf("failed to GET /master/state.json: %s", err)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return st, fmt.Errorf("failed to read /master/state.json response: %s", err)
	}

	log.Printf("mesos: /master/state.json response:\n%s", string(body))
	err = json.Unmarshal(body, &st)
	if err != nil {
		return st, fmt.Errorf("failed to parse /master/state.json response: %s", err)
	}

	return st, nil // OK
}
