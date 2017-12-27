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

package helpers

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"time"

	dockermessage "github.com/docker/docker/pkg/jsonmessage"
	docker "github.com/docker/engine-api/client"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/engine-api/types/network"
	"github.com/docker/engine-api/types/strslice"
	"github.com/juju/ratelimit"
	"golang.org/x/net/context"
)

const (
	// TestImage is the image used in the test
	TestImage = "ubuntu:latest"
	// TestCommand is the command run in the container
	TestCommand = "/bin/bash"
)

func newContainerName() string {
	return "benchmark_container_" + strconv.FormatInt(time.Now().UnixNano(), 10) + strconv.Itoa(rand.Int())
}

func getContext() context.Context {
	return context.Background()
}

// DockerHelper provides the helper functions to simplify docker benchmark.
type DockerHelper struct {
	client   *docker.Client
	errStats *ErrorStats
}

// NewDockerHelper creates and returns a new DockerHelper
func NewDockerHelper(client *docker.Client) *DockerHelper {
	return &DockerHelper{
		client:   client,
		errStats: newErrorStats(),
	}
}

// PullTestImage pulls the test image, panics when error occurs during pulling
func (d *DockerHelper) PullTestImage() {
	resp, err := d.client.ImagePull(getContext(), TestImage, types.ImagePullOptions{})
	if err != nil {
		panic(fmt.Sprintf("Error pulling image: %v", err))
		return
	}
	defer resp.Close()
	// TODO(random-liu): Use the image pulling progress information.
	decoder := json.NewDecoder(resp)
	for {
		var msg dockermessage.JSONMessage
		err := decoder.Decode(&msg)
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(fmt.Sprintf("Error decoding image pulling progress: %v", err))
		}
		if msg.Error != nil {
			panic(fmt.Sprintf("Error during image pulling: %v", err))
		}
	}
}

// GetContainerIDs returns all the container ids in the system, panics when error occurs
func (d *DockerHelper) GetContainerIDs() []string {
	containerIDs := []string{}
	containers, err := d.client.ContainerList(getContext(), types.ContainerListOptions{All: true})
	if err != nil {
		panic(fmt.Sprintf("Error list containers: %v", err))
	}
	for _, container := range containers {
		containerIDs = append(containerIDs, container.ID)
	}
	return containerIDs
}

// GetContainerNum returns container number in the system, panics when error occurs
func (d *DockerHelper) GetContainerNum(all bool) int {
	containers, err := d.client.ContainerList(getContext(), types.ContainerListOptions{All: all})
	if err != nil {
		panic(fmt.Sprintf("Error list containers: %v", err))
	}
	return len(containers)
}

// CreateContainers creates num of containers, returns a slice of container ids
func (d *DockerHelper) CreateContainers(num int) []string {
	ids := []string{}
	for i := 0; i < num; i++ {
		name := newContainerName()
		cfg := &container.Config{
			AttachStderr: false,
			AttachStdin:  false,
			AttachStdout: false,
			Tty:          true,
			Cmd:          strslice.StrSlice([]string{TestCommand}),
			Image:        TestImage,
		}
		container, err := d.client.ContainerCreate(getContext(), cfg, &container.HostConfig{}, &network.NetworkingConfig{}, name)
		ids = append(ids, container.ID)
		d.errStats.add("create containers", err)
	}
	return ids
}

// StartContainers starts all the containers in ids slice
func (d *DockerHelper) StartContainers(ids []string) {
	for _, id := range ids {
		d.errStats.add("start containers", d.client.ContainerStart(getContext(), id))
	}
}

// StopContainers stops all the containers in ids slice
func (d *DockerHelper) StopContainers(ids []string) {
	for _, id := range ids {
		d.errStats.add("stop containers", d.client.ContainerStop(getContext(), id, 10))
	}
}

// RemoveContainers removes all the containers in ids slice
func (d *DockerHelper) RemoveContainers(ids []string) {
	for _, id := range ids {
		d.errStats.add("remove containers", d.client.ContainerRemove(getContext(), id, types.ContainerRemoveOptions{}))
	}
}

// CreateDeadContainers creates num of containers but not starts them, returns a slice of container ids
func (d *DockerHelper) CreateDeadContainers(num int) []string {
	return d.CreateContainers(num)
}

// CreateAliveContainers creates num of containers and also starts them, returns a slice of container ids
func (d *DockerHelper) CreateAliveContainers(num int) []string {
	ids := d.CreateContainers(num)
	d.StartContainers(ids)
	return ids
}

// DoListContainerBenchmark does periodically ListContainers with specific interval, returns latencies of
// all the calls in nanoseconds
func (d *DockerHelper) DoListContainerBenchmark(interval, testPeriod time.Duration, listAll bool) []int {
	startTime := time.Now()
	latencies := []int{}
	for {
		start := time.Now()
		_, err := d.client.ContainerList(getContext(), types.ContainerListOptions{All: listAll})
		d.errStats.add("list containers", err)
		latencies = append(latencies, int(time.Since(start).Nanoseconds()))
		if time.Now().Sub(startTime) >= testPeriod {
			break
		}
		if interval != 0 {
			time.Sleep(interval)
		}
	}
	return latencies
}

// DoInspectContainerBenchmark does periodically InspectContainer with specific interval, returns latencies
// of all the calls in nanoseconds
func (d *DockerHelper) DoInspectContainerBenchmark(interval, testPeriod time.Duration, containerIDs []string) []int {
	startTime := time.Now()
	latencies := []int{}
	rand.Seed(time.Now().Unix())
	for {
		containerID := containerIDs[rand.Int()%len(containerIDs)]
		start := time.Now()
		_, err := d.client.ContainerInspect(getContext(), containerID)
		d.errStats.add("inspect container", err)
		latencies = append(latencies, int(time.Since(start).Nanoseconds()))
		if time.Now().Sub(startTime) >= testPeriod {
			break
		}
		if interval != 0 {
			time.Sleep(interval)
		}
	}
	return latencies
}

// DoParallelListContainerBenchmark starts routineNumber of goroutines and let them do DoListContainerBenchmark,
// returns latencies of all the calls in nanoseconds
func (d *DockerHelper) DoParallelListContainerBenchmark(interval, testPeriod time.Duration, routineNumber int, all bool) []int {
	wg := &sync.WaitGroup{}
	wg.Add(routineNumber)
	latenciesTable := make([][]int, routineNumber)
	for i := 0; i < routineNumber; i++ {
		go func(index int) {
			latenciesTable[index] = d.DoListContainerBenchmark(interval, testPeriod, all)
			wg.Done()
		}(i)
	}
	wg.Wait()
	allLatencies := []int{}
	for _, latencies := range latenciesTable {
		allLatencies = append(allLatencies, latencies...)
	}
	return allLatencies
}

// DoParallelInspectContainerBenchmark starts routineNumber of goroutines and let them do DoInspectContainerBenchmark,
// returns latencies of all the calls in nanoseconds
func (d *DockerHelper) DoParallelInspectContainerBenchmark(interval, testPeriod time.Duration, routineNumber int, containerIDs []string) []int {
	wg := &sync.WaitGroup{}
	wg.Add(routineNumber)
	latenciesTable := make([][]int, routineNumber)
	for i := 0; i < routineNumber; i++ {
		go func(index int) {
			latenciesTable[index] = d.DoInspectContainerBenchmark(interval, testPeriod, containerIDs)
			wg.Done()
		}(i)
	}
	wg.Wait()
	allLatencies := []int{}
	for _, latencies := range latenciesTable {
		allLatencies = append(allLatencies, latencies...)
	}
	return allLatencies
}

// DoParallelContainerStartBenchmark starts routineNumber of goroutines and let them start containers, returns latencies
// of all the starting calls in nanoseconds. There is a global rate limit on starting calls per second.
func (d *DockerHelper) DoParallelContainerStartBenchmark(qps float64, testPeriod time.Duration, routineNumber int) []int {
	wg := &sync.WaitGroup{}
	wg.Add(routineNumber)
	ratelimit := ratelimit.NewBucketWithRate(qps, int64(routineNumber))
	latenciesTable := make([][]int, routineNumber)
	for i := 0; i < routineNumber; i++ {
		go func(index int) {
			startTime := time.Now()
			latencies := []int{}
			for {
				ratelimit.Wait(1)
				start := time.Now()
				ids := d.CreateContainers(1)
				d.StartContainers(ids)
				latencies = append(latencies, int(time.Since(start).Nanoseconds()))
				if time.Now().Sub(startTime) >= testPeriod {
					break
				}
			}
			latenciesTable[index] = latencies
			wg.Done()
		}(i)
	}
	wg.Wait()
	allLatencies := []int{}
	for _, latencies := range latenciesTable {
		allLatencies = append(allLatencies, latencies...)
	}
	return allLatencies
}

// DoParallelContainerStopBenchmark starts routineNumber of goroutines and let them stop containers, returns latencies
// of all the stopping calls in nanoseconds. There is a global rate limit on stopping calls per second.
func (d *DockerHelper) DoParallelContainerStopBenchmark(qps float64, routineNumber int) []int {
	wg := &sync.WaitGroup{}
	ids := d.GetContainerIDs()
	idTable := make([][]string, routineNumber)
	for i := 0; i < len(ids); i++ {
		idTable[i%routineNumber] = append(idTable[i%routineNumber], ids[i])
	}
	wg.Add(routineNumber)
	ratelimit := ratelimit.NewBucketWithRate(qps, int64(routineNumber))
	latenciesTable := make([][]int, routineNumber)
	for i := 0; i < routineNumber; i++ {
		go func(index int) {
			latencies := []int{}
			for _, id := range idTable[index] {
				ratelimit.Wait(1)
				start := time.Now()
				d.StopContainers([]string{id})
				d.RemoveContainers([]string{id})
				latencies = append(latencies, int(time.Since(start).Nanoseconds()))
			}
			latenciesTable[index] = latencies
			wg.Done()
		}(i)
	}
	wg.Wait()
	allLatencies := []int{}
	for _, latencies := range latenciesTable {
		allLatencies = append(allLatencies, latencies...)
	}
	return allLatencies
}

// LogError logs all the docker operation errors to stderr and clear current errors
// TODO(random-liu): Print configuration and errors to stdout, and benchmark result to files
func (d *DockerHelper) LogError() {
	if !d.errStats.hasError() {
		fmt.Fprintf(os.Stderr, "[Error] %s %s\n", time.Now(), d.errStats.stats())
	}
	d.errStats = newErrorStats()
}
