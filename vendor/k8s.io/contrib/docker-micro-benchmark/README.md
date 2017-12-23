Docker Micro Benchmark
======================
Docker micro benchmark is a tool aimed at benchmarking docker operations
which are critical to Kubelet performance, such as `docker ps`, `docker inspect` etc.

Description
------------
Docker micro benchmark benchmarks the following docker operations:
  * **`docker ps [-a]`**: Kubelet does periodical `docker ps -a` to detect container
    state changes, so its performance is crucial to Kubelet.
  * **`docker inspect`**: Kubelet does `docker inspect` to get detailed information of
    specific container when it finds out a container state is changed. So inspect
    is also relatively frequent.
  * **`docker create` & `docker start`**: The performance of `docker create` and
    `docker start` are important for Kubelet when creating pods, especially for batch
    creation.
  * **`docker stop` & `docker remove`**: The same with above.

Docker micro benchmark supports 4 kinds of benchmarks:
  * Benchmark `docker ps`, `docker ps -a` and `docker inspect` with different number
    of dead and alive containers.
  * Benchmark `docker ps`, `docker ps -a` and `docker inspect` with different operation
    intervals.
  * Benchmark `docker ps -a` and `docker inspect` with different number of goroutines.
  * Benchmark `docker create & docker start` and `docker stop & docker remove` with
    different operation rate limits.

Instructions
------------
#### Dependencies
* golang
* sysstat
* gnuplot

#### Build
`godep go build k8s.io/contrib/docker-micro-benchmark`

#### Usage
`benchmark.sh` is the script starting the benchmark.

`Usage : benchmark.sh -[o|c|i|r]`:
  * **-o**: Run `docker create/start/stop/remove` benchmark.
  * **-c**: Run `docker list/inspect` with different number of containers.
    *Notice that containers created in this benchmark won't be removed even after
    benchmark is over.* That's intended because creating containers is really
    slow, it's better to reuse these containers in the following benchmarks. If
    you want to remove all the containers, just run `shell/remove_all_containers.sh`.
  * **-i**: Run `docker list/inspect` with different operation intervals.
  * **-r**: Run `docker list/inspect` with different number of goroutines.

You can run `benchmark.sh` with multiple options, the script will run benchmark
corresponding to each of the options one by one. *Notice that it's better to put
`-c` in front of `-i` and `-r`, so that they can reuse the containers created in
`-c` benchmark.*

#### Result
After the benchmark finishes, all results will be generated in `result/`
directory. Different benchmark results locate in different sub-directories.

There are two forms of benchmark result:
* **\*.dat**: Table formatted text result.
* **\*.png**: Graph result auto-generated from the text result. Notice that there
    is no graph for `sar` result. If you want to analyse the data in `sar` result,
    you can use another tool [kSar](https://sourceforge.net/projects/ksar/).

Result
------
Here are links of some previous benchmark results:
* https://github.com/kubernetes/kubernetes/issues/16110#issuecomment-180510177
* https://docs.google.com/document/d/1d5xaYW3oVnzTgjlcRSnj5aJQusRkrQotXF2zBoTnNkw/edit

TODOs
-----
* Support event stream benchmark. The old event stream benchmark is removed in this version.
* Add error statisitcs and sanity check in current benchmarks to analyze docker reliability.

