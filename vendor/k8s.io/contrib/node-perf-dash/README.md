# Kubernetes Node Performance Dashboard

Node Performance Dashboard (node-perf-dash) is a web UI to collect and analyze performance test results of Kubernetes nodes. It collects data from Kubernetes node e2e performance tests, which can be stored either in local FS or Google GCS, then visualizes the data in 4 dashboards:

* **Builds**:monitoring performance change over different builds
* **Comparison**: compare performance change with different test parameters (e.g. pod number, creation speed, machine type)
* **Time series**: time series data including operation tracing probes inside kubernetes and resource-usage change over time
* **Tracing**: plot the latency percentile between any two tracing probes over different build

Node-Perf-Dash is running and available at http://node-perf-dash.k8s.io/

## Getting Started

Build node-perf-dash:

```bash
make node-perf-dash
```

Collect data from Google GCS:

```bash
node-perf-dash --address=0.0.0.0:808 --builds=20 --tracing=true --datasource=google-gcs
```

Collect data from local test data:

```bash
node-perf-dash --address=0.0.0.0:808 --builds=20 --tracing=true --datasource=local --local-data-dir=$MY_TEST_RESULT_PATH
```

The test result must have the following directory structure:
```bash
$MY_TEST_RESULT_PATH/
  latest-build.txt
  build_nr_1/
      build-log.txt
      artifacts/
          test_machine_host_name1/
              kubelet.log
          test_machine_host_name2
          ...
  build_nr_N
  ...
```

## Dashboards

#### Builds

You display the desired data by selecting

* **Job**: select the test project (e.g. _kubelet-benchmark-gce-e2e-ci_, _continuous-node-e2e-docker-benchmark_)
* **test**: display data for a test by selecting the short test name, or selecting test options one by one
* **image/machine**: select from the available images and machine type (capacity in format _cpu:1core,memory:3.5G_)
* **build**: periodic benchmark tests are running with incremental build number, node-perf-dash collects latest test data using total build count specified by _--builds_, you can change the range of builds in dashboar (see https://github.com/kubernetes/kubernetes/blob/master/test/e2e_node/jenkins/benchmark/benchmark-config.yam)

Resource usage (CPU/memory of kubelet/runtime) will be displayed. Pod startup latency and creation throughput will be displayed for density test. (see https://github.com/kubernetes/kubernetes/blob/master/test/e2e_node/density_test.go)

#### Comparison

To compare node performance among different tests, click _COMPARE IT_ button in the right upper corner on the build page. The test will be added to the comparison list in the comparison page. Click _LOAD_ to see the comparison in bar charts (data are averaged over the selected build range).

#### Time Series

Analyzing time series data are useful to drill into node performance issues. The page contains the operation tracing data both from test and kubelet operations. It also shows the resource usage of kubelet and runtime changing with time during the test.

The tracing inside kubelet is done by parsing the log of kubelet. It contains important information such as when kubelet SyncLoop detects pod configuration change, when a pod is running, and when kubelet status manager reports pod status change to the API server. In future we plan to use _Event_ as a fixed format of tracing instead of using random logs. See https://github.com/kubernetes/kubernetes/pull/31583 for more details.

### Tracing

Interested in knowing the latency distribution between any two operations? You can select two operations shown in the time series page (probes) and see the latency percentiles. (it does not match operations for the same pod, instead simply assumes all operations happen in order)