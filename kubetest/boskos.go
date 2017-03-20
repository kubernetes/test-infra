/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func renew(proj string) error {
	resp, err := http.Get(fmt.Sprintf("http://boskos/request?name=%v&duration=5m", proj))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("Status %s, StatusCode %v", resp.Status, resp.StatusCode)
	}
	return nil
}

func returnProj(proj string) error {
	resp, err := http.Get(fmt.Sprintf("http://boskos/request?name=%v", proj))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("Status %s, StatusCode %v", resp.Status, resp.StatusCode)
	}
	return nil
}

func requestProj() (string, error) {
	resp, err := http.Get(fmt.Sprintf("http://boskos/request?type=free&duration=5m"))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 204 {
		log.Print("No available projects, retrying in 30sec.")
		time.Sleep(30 * time.Second)
		return "", nil
	} else if resp.StatusCode == 200 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}

		type GCPProject struct {
			// This need to be consistent with boskos
			Name     string        `json:"name"`
			Owner    string        `json:"owner"`
			Duration time.Duration `json:"duration"`
			Start    time.Time     `json:"start"`
		}
		var p = new(GCPProject)
		err = json.Unmarshal(body, &p)
		if err != nil {
			return "", err
		}
		return p.Name, nil
	} else {
		return "", fmt.Errorf("Status %s, StatusCode %v", resp.Status, resp.StatusCode)
	}
}

func boskos(lease time.Duration) (string, error) {

	var proj string
	for i := 0; i < 3; i++ {
		proj, err := requestProj()
		if err != nil {
			return "", err
		}
		if proj != "" {
			break
		}
		return "", fmt.Errorf("No available projects")
	}

	go func(proj string, lease time.Duration) {
		sigs := make(chan os.Signal, 1)
		leaseChan := time.NewTimer(lease).C
		tickChan := time.NewTicker(time.Minute * 4).C // Give 1 min overhead

		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

		for {
			select {
			case <-tickChan:
				log.Print("Renew lease for another 5min")
				renew(proj)
			case <-leaseChan:
				log.Print("Lease timed up")
				returnProj(proj)
				return
			case <-sigs:
				log.Printf("Terminated : sig %v", sigs)
				returnProj(proj)
				return
			}
		}
	}(proj, lease)

	return proj, nil
}
