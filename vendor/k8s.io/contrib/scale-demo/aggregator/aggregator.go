/*
Copyright 2015 The Kubernetes Authors.

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
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"k8s.io/kubernetes/pkg/api"
	client "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/client/unversioned/clientcmd"
	"k8s.io/kubernetes/pkg/fields"
	"k8s.io/kubernetes/pkg/labels"

	vegeta "github.com/tsenart/vegeta/lib"
)

var (
	addr     = flag.String("address", "localhost:8080", "The address to serve on")
	selector = flag.String("selector", "", "The label selector for pods")
	useIP    = flag.Bool("use-ip", false, "Use IP for aggregation")
	sleep    = flag.Duration("sleep", 5*time.Second, "The sleep period between aggregations")

	serveData = []byte{}
	lock      = sync.Mutex{}
)

func getData() []byte {
	lock.Lock()
	defer lock.Unlock()
	return serveData
}

func setData(data []byte) {
	lock.Lock()
	defer lock.Unlock()
	serveData = data
}

func serveHTTP(res http.ResponseWriter, req *http.Request) {
	res.Header().Set("Access-Control-Allow-Origin", "*")
	res.WriteHeader(http.StatusOK)
	res.Write(getData())
}

func main() {
	flag.Parse()

	http.HandleFunc("/", serveHTTP)
	go http.ListenAndServe(*addr, nil)

	for {
		start := time.Now()
		loadData()
		latency := time.Now().Sub(start)
		if latency < *sleep {
			time.Sleep(*sleep - latency)
		}
		fmt.Printf("%v\n", time.Now().Sub(start))
	}
}

func getField(obj map[string]interface{}, fields ...string) (interface{}, bool) {
	nextObj, found := obj[fields[0]]
	if !found {
		return nil, false
	}
	if len(fields) > 1 {
		return getField(nextObj.(map[string]interface{}), fields[1:]...)
	}
	return nextObj, true
}
func loadData() {
	config := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(&clientcmd.ClientConfigLoadingRules{}, &clientcmd.ConfigOverrides{})
	clientConfig, err := config.ClientConfig()
	if err != nil {
		fmt.Printf("Error creating client config: %v", err)
		return
	}
	c, err := client.New(clientConfig)
	if err != nil {
		fmt.Printf("Error creating client: %v", err)
		return
	}
	var labelSelector labels.Selector
	if *selector != "" {
		labelSelector, err = labels.Parse(*selector)
		if err != nil {
			fmt.Printf("Parse label selector err: %v", err)
			return
		}
	} else {
		labelSelector = labels.Everything()
	}
	pods, err := c.Pods(api.NamespaceDefault).List(api.ListOptions{
		LabelSelector: labelSelector,
		FieldSelector: fields.Everything(),
	})
	if err != nil {
		fmt.Printf("Error getting pods: %v", err)
		return
	}
	loadbots := []*api.Pod{}
	for ix := range pods.Items {
		pod := &pods.Items[ix]
		if pod.Status.PodIP == "" {
			continue
		}
		loadbots = append(loadbots, pod)
	}
	parts := []vegeta.Metrics{}
	lock := sync.Mutex{}
	wg := sync.WaitGroup{}
	wg.Add(len(loadbots))
	for ix := range loadbots {
		go func(ix int) {
			defer wg.Done()
			pod := loadbots[ix]
			var data []byte
			if *useIP {
				url := "http://" + pod.Status.PodIP + ":8080/"
				resp, err := http.Get(url)
				if err != nil {
					fmt.Printf("Error getting: %v\n", err)
					return
				}
				defer resp.Body.Close()
				if data, err = ioutil.ReadAll(resp.Body); err != nil {
					fmt.Printf("Error reading: %v\n", err)
					return
				}
			} else {
				var err error
				data, err = c.RESTClient.Get().AbsPath("/api/v1/proxy/namespaces/default/pods/" + pod.Name + ":8080/").DoRaw()
				if err != nil {
					fmt.Printf("Error proxying to pod: %v\n", err)
					return
				}
			}
			var metrics vegeta.Metrics
			if err := json.Unmarshal(data, &metrics); err != nil {
				fmt.Printf("Error decoding: %v\n", err)
				return
			}
			lock.Lock()
			defer lock.Unlock()
			parts = append(parts, metrics)
		}(ix)
	}
	wg.Wait()
	data, err := json.Marshal(parts)
	if err != nil {
		fmt.Printf("Error marshaling: %v", err)
	}
	setData(data)
	fmt.Printf("Updated.\n")
}
