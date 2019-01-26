# Crier

Crier reports your prowjobs on their status changes.

## Usage / How to enable existing available reporters

### [Gerrit reporter](/prow/gerrit/reporter)

You can enable gerrit reporter in crier by specifying `--gerrit` flag.

Similar to the [gerrit adapter](/prow/cmd/gerrit), you'll need to specify `--gerrit-projects` for
your gerrit projects, and also `--cookiefile` for the gerrit auth token (leave it unset for anonymous).

Gerrit reporter will send a gerrit code review, when all [gerrit adapter](/prow/cmd/gerrit)
scheduled prowjob finishes on a revision, aka, on `SuccessState`, `FailureState`, `AbortedState` or `ErrorState`.
It will also attach a report url so people can find logs of the job.

### [Pubsub reporter](/prow/pubsub/reporter)

You can enable pubsub reporter in crier by specifying `--pubsub` flag.

You need to specify following labels in order for pubsub reporter to report your prowjob:

| Label                          | Description                                                                                                                               |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------|
| `"prow.k8s.io/pubsub-project"` | Your gcp project where pubsub channel lives                                                                                               |
| `"prow.k8s.io/pubsub-topic"`   | The [topic](https://cloud.google.com/pubsub/docs/publisher) of your pubsub message                                                        |
| `"prow.k8s.io/pubsub-runID"`   | A user assigned job id. It's tied to the prowjob, serves as a name tag and help user to differentiate results in multiple pubsub messages |

Pubsub reporter will report whenever prowjob has a state transition.

You can check the reported result by [list the pubsub topic](https://cloud.google.com/sdk/gcloud/reference/pubsub/topics/list). 

<!-- TODO(krzyzacy): move github reporter over -->

## Implementation details

Crier supports multiple reporters, each reporter will become a crier controller. Controllers
will get prowjob change notifications from a [shared informer](https://github.com/kubernetes/client-go/blob/master/tools/cache/shared_informer.go), and you can specify `--num-workers` to change parallelism.

If you are interested in how client-go works under the hood, the details are explained 
[in this doc](https://github.com/kubernetes/sample-controller/blob/master/docs/controller-client-go.md)


## Adding a new reporter

Each crier controller takes in a reporter.

Each reporter will implement the following interface:
```go
type reportClient interface {
	Report(pj *v1.ProwJob) error
	GetName() string
	ShouldReport(pj *v1.ProwJob) bool
}
```

`GetName` will return the name of your reporter, the name will be used as a key when we store previous
reported state for each prowjob.

`ShouldReport` will return if a prowjob should be handled by current reporter.

`Report` is the actual report logic happens. Return `nil` means report is successful, and the reported
state will be saved in the prowjob. Return an actual error if report fails, crier will re-add the prowjob
key to the shared cache and retry up to 5 times.

You can add a reporter that implements the above interface, and add a flag to turn it on/off in crier.

