# Test for Network Benchmark Tool

[This script](./test.sh) verifies the network benchmarks in the [kubernetes/perf-tests](https://github.com/kubernetes/perf-tests) package. It clones the specified fork and branch, runs network benchmark tests against a provided image, and cleans up after execution.

## Usage

Set environment variables as needed:
• FORK_TO_TEST (default: kubernetes/perf-tests)
• BRANCH_TO_TEST (default: master)
• IMAGE_TO_TEST (default: ghcr.io/ritwikranjan/nptest:latest)
• KUBECONFIG (default: ~/.kube/config)

```shell
bash test.sh
```
