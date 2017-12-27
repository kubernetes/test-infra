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
	"net/http"
	"os"
	"time"
)

const (
	pollDuration = 10 * time.Minute
	errorDelay   = 10 * time.Second
	maxBuilds    = 100
)

var (
	addr   = flag.String("address", ":8080", "The address to serve web data on")
	www    = flag.Bool("www", false, "If true, start a web-server to server performance data")
	wwwDir = flag.String("dir", "www", "If non-empty, add a file server for this directory at the root of the web server")
	builds = flag.Int("builds", maxBuilds, "Total builds number")
)

func main() {
	fmt.Print("Starting perfdash...\n")
	flag.Parse()

	if *builds > maxBuilds || *builds < 0 {
		fmt.Printf("Invalid builds number: %d", *builds)
		*builds = maxBuilds
	}

	// TODO(random-liu): Add a top layer downloader to download build log from different buckets when we support
	// more buckets in the future.
	downloader := NewGoogleGCSDownloader(*builds)
	result := make(TestToBuildData)
	var err error

	if !*www {
		result, err = downloader.getData()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching data: %v\n", err)
			os.Exit(1)
		}
		prettyResult, err := json.MarshalIndent(result, "", " ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error formating data: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Result: %v", string(prettyResult))
		return
	}

	go func() {
		for {
			fmt.Printf("Fetching new data...\n")
			result, err = downloader.getData()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error fetching data: %v\n", err)
				time.Sleep(errorDelay)
				continue
			}
			time.Sleep(pollDuration)
		}
	}()

	http.Handle("/api", &result)
	http.Handle("/", http.FileServer(http.Dir(*wwwDir)))
	http.ListenAndServe(*addr, nil)
}
