# apicoverage - API coverage measuring tool

apicoverage is a tool for measuring API coverage on e2e tests
by comparing the e2e test log and swagger spec of k8s.

## Usage

Run e2e tests with API operation detail(-v=8 option):
```
$ kubetest --test --test_args="--ginkgo.focus=\[Conformance\] -v=8" | tee api.log
```
Then run the tool:
```
$ go run apicoverage.go --restlog=api.log
API,TOTAL,TESTED,UNTESTED,COVERAGE(%)
ALL,958,69,889,7
STABLE,481,53,428,11
Alpha,98,1,97,1
Beta,379,15,364,3
```

