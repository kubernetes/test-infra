# Prow-Controller-Manager

`prow-controller-manager` manages the job execution and lifecycle for jobs running in k8s.

It currently acts as a replacement for [Plank].

It is intended to eventually replace other components, such as [Sinker] and [Crier].
See the tracking issue [#17024](https://github.com/kubernetes/test-infra/issues/17024) for details.

### Advantages

- Eventbased rather than cronbased, hence reacting much faster to changes in prowjobs or pods
- Per-Prowjob retrying, meaning genuinely broken prowjobs will not be retried forever and transient errors will be retried much quicker
- Uses a cache for the build cluster rather than doing a LIST every 30 seconds, reducing the load on the build clusters api server

### Exclusion with other components

This is mutually exclusive with only [Plank].
Only one of them may have more than zero replicas at the same time.

### Usage
```bash
$ bazel run //prow/cmd/prow-controller-manager -- --help
```

### Configuration

TODO: Point to configuration after merge

[Plank]: /prow/cmd/plank
[Sinker]: /prow/cmd/sinker
[Crier]: /prow/cmd/crier
