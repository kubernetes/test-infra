/*
Copyright 2016 The Kubernetes Authors.

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
	"time"
)

// Docker configuration
var (
	endpoint   = "unix:///var/run/docker.sock"
	apiVersion = "v1.21"
)

// Period configuration
const (
	shortTestPeriod = 10 * time.Second
	longTestPeriod  = 50 * time.Second
)

// For container start benchmark
var containerOpConfig = map[string]interface{}{
	"qps": []float64{
		1.0,
		2.0,
		4.0,
		8.0,
		16.0,
		32.0,
		64.0,
	},
	"routine": 100,
	"period":  longTestPeriod,
}

// For varies interval benchmark
var variesIntervalConfig = map[string]interface{}{
	"list interval": []time.Duration{
		0 * time.Second,
		50 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond,
		500 * time.Millisecond,
		1 * time.Second,
		2 * time.Second,
		5 * time.Second,
	},
	"inspect interval": []time.Duration{
		0 * time.Second,
		1 * time.Millisecond,   // 1000 inspect/second = 100 pods * 10 containers
		2 * time.Millisecond,   // 500 inspect/second = 100 pods * 5 containers = 50 pods * 10 containers
		5 * time.Millisecond,   // 200 inspect/second = 100 pods * 2 containers = 20 pods * 10 containers
		10 * time.Millisecond,  // 100 inspect/second = 100 pods * 1 containers = 10 pods * 10 containers
		50 * time.Millisecond,  // 20 inspect/second = 20 pods * 1 containers = 10 pods * 2 containers
		100 * time.Millisecond, // 10 insepct/second = 10 pods * 1 containers = 5 pods * 2 containers
	},
	"list period":    longTestPeriod,
	"inspect period": shortTestPeriod,
}

// For varies container number benchmark
var variesContainerNumConfig = map[string]interface{}{
	// aliveContainers * 3
	"dead": []int{
		60,
		120,
		300,
		600,
	},
	"alive": []int{
		20,  // 10 * 2
		40,  // 20 * 2
		100, // 50 * 2
		200, // 100 * 2
	},
	"interval": 200 * time.Millisecond,
	"period":   shortTestPeriod,
}

// For varies routine number benchmark
var variesRoutineNumConfig = map[string]interface{}{
	"list interval":    1 * time.Second,
	"inspect interval": 500 * time.Millisecond, // 2 containers/pod
	"routines": []int{
		1,
		5,
		10,
		20,
		50,
		100,
	},
	"period": shortTestPeriod,
}
