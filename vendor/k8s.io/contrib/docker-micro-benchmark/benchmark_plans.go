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

	"k8s.io/contrib/docker-micro-benchmark/helpers"
)

func benchmarkContainerStart(d *helpers.DockerHelper) {
	cfg := containerOpConfig
	period := cfg["period"].(time.Duration)
	routine := cfg["routine"].(int)
	func() {
		defer d.LogError()
		helpers.LogTitle("container_op")
		helpers.LogEVar(map[string]interface{}{
			"period":  period,
			"routine": routine,
		})
		helpers.LogLabels("qps", "cps")
		for _, q := range cfg["qps"].([]float64) {
			start := time.Now()
			latencies := d.DoParallelContainerStartBenchmark(q, period, routine)
			cps := float64(len(latencies)) / time.Now().Sub(start).Seconds()
			helpers.LogResult(latencies, helpers.Ftoas(q, cps)...)

			start = time.Now()
			latencies = d.DoParallelContainerStopBenchmark(q, routine)
			cps = float64(len(latencies)) / time.Now().Sub(start).Seconds()
			helpers.LogResult(latencies, helpers.Ftoas(q, cps)...)
		}
	}()
}

func benchmarkVariesContainerNumber(d *helpers.DockerHelper) {
	cfg := variesContainerNumConfig
	deadContainers := cfg["dead"].([]int)
	aliveContainers := cfg["alive"].([]int)
	period := cfg["period"].(time.Duration)
	interval := cfg["interval"].(time.Duration)
	dead := deadContainers[0]
	alive := aliveContainers[0]
	ids := append(d.CreateDeadContainers(dead), d.CreateAliveContainers(alive)...)
	func() {
		defer d.LogError()
		helpers.LogTitle("varies_container")
		helpers.LogEVar(map[string]interface{}{
			"period":   period,
			"interval": interval,
		})
		helpers.LogLabels("#dead", "#alive", "#total")
		for i, num := range append(deadContainers, aliveContainers...) {
			if i < len(deadContainers) {
				// Create more dead containers
				ids = append(ids, d.CreateDeadContainers(num-dead)...)
				dead = num
			} else {
				// Create more alive containers
				ids = append(ids, d.CreateAliveContainers(num-alive)...)
				alive = num
			}
			total := dead + alive
			latencies := d.DoListContainerBenchmark(interval, period, true)
			helpers.LogResult(latencies, helpers.Itoas(dead, alive, total)...)
			latencies = d.DoListContainerBenchmark(interval, period, false)
			helpers.LogResult(latencies, helpers.Itoas(dead, alive, total)...)
			latencies = d.DoInspectContainerBenchmark(interval, period, ids)
			helpers.LogResult(latencies, helpers.Itoas(dead, alive, total)...)
		}
	}()
}

func benchmarkVariesInterval(d *helpers.DockerHelper) {
	alive := d.GetContainerNum(false)
	dead := d.GetContainerNum(true) - alive
	containerIDs := d.GetContainerIDs()
	cfg := variesIntervalConfig
	listIntervals := cfg["list interval"].([]time.Duration)
	listPeriod := cfg["list period"].(time.Duration)
	func() {
		defer d.LogError()
		helpers.LogTitle("list_all")
		helpers.LogEVar(map[string]interface{}{
			"#alive": alive,
			"#dead":  dead,
			"all":    true,
			"period": listPeriod,
		})
		helpers.LogLabels("interval")
		for _, curInterval := range listIntervals {
			latencies := d.DoListContainerBenchmark(curInterval, listPeriod, true)
			helpers.LogResult(latencies, helpers.Itoas(int(curInterval/time.Millisecond))...)
		}
	}()

	func() {
		defer d.LogError()
		helpers.LogTitle("list_alive")
		helpers.LogEVar(map[string]interface{}{
			"#alive": alive,
			"#dead":  dead,
			"all":    false,
			"period": listPeriod,
		})
		helpers.LogLabels("interval")
		for _, curInterval := range listIntervals {
			latencies := d.DoListContainerBenchmark(curInterval, listPeriod, false)
			helpers.LogResult(latencies, helpers.Itoas(int(curInterval/time.Millisecond))...)
		}
	}()

	func() {
		defer d.LogError()
		inspectIntervals := cfg["inspect interval"].([]time.Duration)
		inspectPeriod := cfg["inspect period"].(time.Duration)
		helpers.LogTitle("inspect")
		helpers.LogEVar(map[string]interface{}{
			"#alive": alive,
			"#dead":  dead,
			"period": inspectPeriod,
		})
		helpers.LogLabels("interval")
		for _, curInterval := range inspectIntervals {
			latencies := d.DoInspectContainerBenchmark(curInterval, inspectPeriod, containerIDs)
			helpers.LogResult(latencies, helpers.Itoas(int(curInterval/time.Millisecond))...)
		}
	}()
}

func benchmarkVariesRoutineNumber(d *helpers.DockerHelper) {
	alive := d.GetContainerNum(false)
	dead := d.GetContainerNum(true) - alive
	containerIDs := d.GetContainerIDs()
	cfg := variesRoutineNumConfig
	routines := cfg["routines"].([]int)
	period := cfg["period"].(time.Duration)

	func() {
		defer d.LogError()
		listInterval := cfg["list interval"].(time.Duration)
		helpers.LogTitle("list_all")
		helpers.LogEVar(map[string]interface{}{
			"#alive":   alive,
			"#dead":    dead,
			"all":      true,
			"interval": listInterval,
			"period":   period,
		})
		helpers.LogLabels("#routines")
		for _, curRoutineNumber := range routines {
			latencies := d.DoParallelListContainerBenchmark(listInterval, period, curRoutineNumber, true)
			helpers.LogResult(latencies, helpers.Itoas(curRoutineNumber)...)
		}
	}()

	func() {
		defer d.LogError()
		inspectInterval := cfg["inspect interval"].(time.Duration)
		helpers.LogTitle("inspect")
		helpers.LogEVar(map[string]interface{}{
			"#alive":   alive,
			"#dead":    dead,
			"interval": inspectInterval,
			"period":   period,
		})
		helpers.LogLabels("#routines")
		for _, curRoutineNumber := range routines {
			latencies := d.DoParallelInspectContainerBenchmark(inspectInterval, period, curRoutineNumber, containerIDs)
			helpers.LogResult(latencies, helpers.Itoas(curRoutineNumber)...)
		}
	}()
}
