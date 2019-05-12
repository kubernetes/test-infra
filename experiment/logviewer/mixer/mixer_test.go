package main

import (
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/kzmrv/logviewer/worker/work"
)

const line1 = `{"kind":"Event","apiVersion":"audit.k8s.io/v1","level":"Metadata","auditID":"0286b87c-b86c-443f-bb1e-807844b35307","stage":"ResponseComplete","requestURI":"/apis/coordination.k8s.io/v1beta1/namespaces/kube-node-lease/leases/gce-scale-cluster-minion-group-4-zr07?timeout=10s","verb":"update","user":{"username":"system:node:gce-scale-cluster-minion-group-4-zr07","groups":["system:nodes","system:authenticated"]},"sourceIPs":["35.227.76.61"],"userAgent":"kubelet/v1.14.0 (linux/amd64) kubernetes/aef1179","objectRef":{"resource":"leases","namespace":"kube-node-lease","name":"gce-scale-cluster-minion-group-4-zr07","uid":"411615ce-0e66-11e9-a584-42010a280002","apiGroup":"coordination.k8s.io","apiVersion":"v1beta1","resourceVersion":"17652814"},"responseStatus":{"metadata":{},"code":200},"requestReceivedTimestamp":"2019-01-02T15:01:16.105964Z","stageTimestamp":"2019-01-02T15:01:16.108038Z","annotations":{"authorization.k8s.io/decision":"allow","authorization.k8s.io/reason":""}}`
const line2 = `{"kind":"Event","apiVersion":"audit.k8s.io/v1","level":"Metadata","auditID":"9d947172-9f21-4449-8460-e53c8c9e87fc","stage":"ResponseComplete","requestURI":"/apis/coordination.k8s.io/v1beta1/namespaces/kube-node-lease/leases/gce-scale-cluster-minion-group-2-v1fj?timeout=10s","verb":"update","user":{"username":"system:node:gce-scale-cluster-minion-group-2-v1fj","groups":["system:nodes","system:authenticated"]},"sourceIPs":["35.229.86.146"],"userAgent":"kubelet/v1.14.0 (linux/amd64) kubernetes/aef1179","objectRef":{"resource":"leases","namespace":"kube-node-lease","name":"gce-scale-cluster-minion-group-2-v1fj","uid":"f188f92d-0e65-11e9-a584-42010a280002","apiGroup":"coordination.k8s.io","apiVersion":"v1beta1","resourceVersion":"17652718"},"responseStatus":{"metadata":{},"code":200},"requestReceivedTimestamp":"2019-01-02T15:01:16.106483Z","stageTimestamp":"2019-01-02T15:01:16.108244Z","annotations":{"authorization.k8s.io/decision":"allow","authorization.k8s.io/reason":""}}`
const line3 = `{"kind":"Event","apiVersion":"audit.k8s.io/v1","level":"Request","auditID":"39aec93e-031b-4002-8c0a-4ddcd92e250b","stage":"ResponseComplete","requestURI":"/api/v1/nodes/gce-scale-cluster-minion-group-2-t86q/status","verb":"patch","user":{"username":"system:node-problem-detector","uid":"uid:node-problem-detector","groups":["system:authenticated"]},"sourceIPs":["35.196.154.110"],"userAgent":"node-problem-detector/v0.5.0-49-gfb81368","objectRef":{"resource":"nodes","name":"gce-scale-cluster-minion-group-2-t86q","apiVersion":"v1","subresource":"status"},"responseStatus":{"metadata":{},"code":200},"requestObject":{"status":{"conditions":[{"type":"FrequentKubeletRestart","status":"False","lastHeartbeatTime":"2019-01-02T15:01:16Z","lastTransitionTime":"2019-01-02T08:15:27Z","reason":"FrequentKubeletRestart"},{"type":"FrequentDockerRestart","status":"False","lastHeartbeatTime":"2019-01-02T15:01:16Z","lastTransitionTime":"2019-01-02T08:15:28Z","reason":"FrequentDockerRestart"},{"type":"FrequentContainerdRestart","status":"False","lastHeartbeatTime":"2019-01-02T15:01:16Z","lastTransitionTime":"2019-01-02T08:15:29Z","reason":"FrequentContainerdRestart"},{"type":"CorruptDockerOverlay2","status":"False","lastHeartbeatTime":"2019-01-02T15:01:16Z","lastTransitionTime":"2019-01-02T08:15:27Z","reason":"CorruptDockerOverlay2"},{"type":"KernelDeadlock","status":"False","lastHeartbeatTime":"2019-01-02T15:01:16Z","lastTransitionTime":"2019-01-02T08:10:26Z","reason":"KernelHasNoDeadlock","message":"kernel has no deadlock"},{"type":"ReadonlyFilesystem","status":"False","lastHeartbeatTime":"2019-01-02T15:01:16Z","lastTransitionTime":"2019-01-02T08:10:26Z","reason":"FilesystemIsNotReadOnly","message":"Filesystem is not read-only"},{"type":"FrequentUnregisterNetDevice","status":"False","lastHeartbeatTime":"2019-01-02T15:01:16Z","lastTransitionTime":"2019-01-02T08:15:27Z","reason":"UnregisterNetDevice"}]}},"requestReceivedTimestamp":"2019-01-02T15:01:16.104561Z","stageTimestamp":"2019-01-02T15:01:16.108460Z","annotations":{"authorization.k8s.io/decision":"allow","authorization.k8s.io/reason":"RBAC: allowed by ClusterRoleBinding \"npd-binding\" of ClusterRole \"system:node-problem-detector\" to User \"system:node-problem-detector\""}}`

var logLines []*work.LogLine

func setupTests() {
	lg1 := toLine("2019-01-02T15:01:16.105964Z", line1)
	lg2 := toLine("2019-01-02T15:01:16.106483Z", line2)
	lg3 := toLine("2019-01-02T15:01:16.104561Z", line3)
	lg4 := toLine("2019-01-02T15:01:16.106964Z", line1)
	lg5 := toLine("2019-01-02T15:01:16.105083Z", line2)
	lg6 := toLine("2019-01-02T15:01:16.104961Z", line3)
	logLines = []*work.LogLine{lg1, lg2, lg3, lg4, lg5, lg6}
}

func toLine(timestamp string, entry string) *work.LogLine {
	t, _ := time.Parse(time.RFC3339Nano, timestamp)
	pt, _ := ptypes.TimestampProto(t)
	return &work.LogLine{
		Entry:     entry,
		Timestamp: pt,
	}
}

func TestProcessWorkResultSortingSingleResponse(t *testing.T) {
	setupTests()
	workRes := &callResult{
		workResult: &work.WorkResult{
			LogLines: logLines,
		},
	}

	ch := toChannel([]*callResult{workRes})
	channels := []chan *callResult{ch}
	logs := processWorkResults(channels)
	expectedOrder := []int{2, 5, 4, 0, 1, 3}
	for i, lg := range logs {
		if lg.Timestamp != logLines[expectedOrder[i]].Timestamp {
			t.Fatalf("Sorting failed with element %v", i)
		}
	}
}

func TestProcessWorkResultSortingMultiResponse(t *testing.T) {
	setupTests()
	workRes := &callResult{
		workResult: &work.WorkResult{
			LogLines: logLines[:3],
		},
	}
	workRes2 := &callResult{
		workResult: &work.WorkResult{
			LogLines: logLines[3:6],
		},
	}

	ch := toChannel([]*callResult{workRes})
	ch2 := toChannel([]*callResult{workRes2})
	channels := []chan *callResult{ch, ch2}
	logs := processWorkResults(channels)
	expectedOrder := []int{2, 5, 4, 0, 1, 3}
	for i, lg := range logs {
		if lg.Timestamp != logLines[expectedOrder[i]].Timestamp {
			t.Fatalf("Sorting failed with element %v", i)
		}
	}
}

func toChannel(slice []*callResult) chan *callResult {
	channel := make(chan *callResult, 10)
	for _, item := range slice {
		channel <- item
	}
	close(channel)
	return channel
}
