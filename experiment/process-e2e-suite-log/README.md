# Target 
To process the input file and extract only a given selection of visited endpoints.

## "Input file":
it is a "master log," that is the collection of the endpoints visited while executing a testing run.  
Here is the [input][input source]: 

This file has an expected and known format: 
- line 1: "separatorLine".
- line 2: timestamp.
- line 3: sig-description for the testcase.
- line 4: description for the testcase.
- line 5 : path of the filename where this specific testcase is located.
- line (repeateable...N): endpoint visited, expressed using non-rest-verbs (get, list, create, update, delete, deletecollection, patch, watch).
- line N : "separatorLine".

Example:
```
----------------
2018-06-05T03:34:03.723486
[sig-network] Services 
  should check NodePort out-of-range
  /go/src/k8s.io/kubernetes/_output/dockerized/go/src/k8s.io/kubernetes/test/e2e/network/service.go:1077
      26: list /apis/admissionregistration.k8s.io/v1alpha1/initializerconfigurations
       5: get /api/v1/namespaces/e2e-tests-services-2xplh
       5: get /apis/extensions/v1beta1/namespaces/kube-system/daemonsets/fluentd-gcp-v3.0.0
       4: get /api/v1/namespaces/e2e-tests-services-2xplh/serviceaccounts/default
       4: watch /api/v1/namespaces/e2e-tests-services-2xplh/serviceaccounts
       3: create /api/v1/namespaces/e2e-tests-services-2xplh/secrets
----------------
```

## Process behind the scenes
- _endpoints_ is a map where we keep the selection of endpoints to match against. This is a map holding the http verb and uri (key) and the regular expression to look after (value).
- the input file is split into subfiles (testcases). The subfiles are left so we can later go and verify/explore the specific testcase and not browse through the master input file.
- For each subfile (testcase) a row is generated, By applying the collection of _endpoints_.  

### Ok, tell me how
- 2 channels are used:
  - _fileNameChannel_ : a channel where the main process will put each filename (testcase) that has to be processed.
  - _rowChannel_ : a channel where the _workers_ will leave the rows to write in the result file.
- workers: Each worker will take a fileName from the _fileNameChannel_, process it, and build a row. The row will be put in _rowChannel_ so that there is only one writer over the resultFile.
- workingGroups: their target is to signal the fact that elements need to be processed (add) or had already been processed (done). Since the processing is done in goroutines, we have to tell the main process to wait until everyone has finished their tasks. Once the workingGroup completeness signal is sent (everything sent was processed), the correspondent channel is closed (sending that signal) and the goroutine ends.

## Output file
A csv file where:
- headers
  - made from the collection of endpoints.
  - testDescription.
- rows
  - Each row is the result of the invocation of each endpoint regex against the current file under process.

# Usage

## Build
```
go build
```

This will leave an executable file named "process-e2e-suite-log".

## Execution
```
$ ./process-e2e-suite-log -input-file-path=master.txt -result-file-name=report.csv
```

### Mandatory flags
- input-file-path: where to find the path of the master log.
- result-file-name: name of the output file.

### Optional values
- workers: 2 by default.
- prefix-for-sub-file: "res" by default.
- skip-splitting: false by default..
- separator: "," by default

[input source]: https:github.com/cncf/apisnoop/issues/17#issuecomment-394866106
