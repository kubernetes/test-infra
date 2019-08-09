# Testgrid Summarizer
The Testgrid Summarizer creates test result summaries for the Testgrid
dashboards and tables. It continuously writes summaries owned by the
Update Master.

## Workflow
The `Update Master` will parse all the groups and request the `Update Servers` to aggregate test results. When the update cycle is done, the `Update Master` will run post-update jobs, which will trigger the `Summarizer` to create the summary object.

## Highlevel Design
The `Summarizer` implements the key functions to convert aggregated results into the appropriate summary format, and continuously writes/reads summaries owned by the `Update Master` to/from storage. This will be a standalone service for all flavor of Testgrid clusters to use for test result translation. The summarizer server will implement the gRPC server component.

```golang
type Summarizer struct {}
func (s *Summarizer) TableToSummary(...) (...) {}
func (s *Summarizer) SummaryToClientObject(...) (...) {}
func (s *Summarizer) GetStatusForDashboard(...) (...) {}

type Storage struct{}
func (s *Storage) SaveSummaryObject() {}
func (s *Storage) RestoreSummaryObject() {}
func (s *Storage) NormalizeObjectName() {}

type SummarizerServer struct {}
func (s *SummarizerServer) GetSummary(...) (...) {}
func (s *SummarizerServer) GetDashboardStatus(...) (...) {}
```

## Implementation Breakdown
The Summarizer component will be implemented in two stages:

1. The translation of test results to summary objects. This will implement most of the Summarizer object and the SummarizerServer. When stage 1 is ready, we should have a standalone server running and serving on-demand test result translation from a remote gRPC client. This section is implemented by [PR #13132](https://github.com/kubernetes/test-infra/pull/13132)
1. The storage of summary. This will implement the Storage object and integrate it with the Summarizer. When stage 2 is ready, we should be able to store data to a permanent storage location to avoid recomputing some summary data, which will improve the overall system efficiency.

## Developer Guide
To run all the tests for the summarizer component.
```
test-infra/testgrid/cmd/summarizer $ bazel test ...
```

To run the server.
```
test-infra/testgrid/cmd/summarizer $ bazel run :summarizer
```

To run the example client.
```
test-infra/testgrid/cmd/summarizer/example $ bazel run :example
```

