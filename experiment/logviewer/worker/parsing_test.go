/*
Copyright 2019 The Kubernetes Authors.

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
	"io"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	ts "github.com/golang/protobuf/ptypes/timestamp"
	pb "github.com/kzmrv/logviewer/worker/work"
)

func TestParseSingleLine(t *testing.T) {
	line, err := parseLine(line1)
	if err != nil {
		t.Fatal(err)
	}

	expectedTime := time.Unix(1546444876, 105964000)
	if line.time.UTC() != expectedTime.UTC() {
		t.Fatalf("Expected time %s received %s", expectedTime, line.time)
	}

	if *line.log != line1 {
		t.Fatalf("Expected log %s received %s", line1, *line.log)
	}
}

func TestDoSimpleWork(t *testing.T) {
	w := &pb.Work{
		TargetSubstring: "\"auditID\":\"39aec93e-031b-4002-8c0a-4ddcd92e250b\"",
	}
	ProcessDoWorkTest(t, w, 1)
}

func TestDoWorkWithSince(t *testing.T) {
	tSince, _ := time.Parse(time.RFC3339Nano, "2019-01-02T15:00:00.104561Z")
	since, _ := ptypes.TimestampProto(tSince)
	w := &pb.Work{
		TargetSubstring: "",
		Since:           since,
	}
	ProcessDoWorkTest(t, w, 2)
}

func TestDoWorkWithSinceAndUntil(t *testing.T) {
	tSince, _ := time.Parse(time.RFC3339Nano, "2019-01-02T15:00:00.104561Z")
	since, _ := ptypes.TimestampProto(tSince)
	tUntil, _ := time.Parse(time.RFC3339Nano, "2019-01-02T16:00:00.104561Z")
	until, _ := ptypes.TimestampProto(tUntil)
	w := &pb.Work{
		TargetSubstring: "",
		Since:           since,
		Until:           until,
	}
	ProcessDoWorkTest(t, w, 1)
}

func TestDoWorkWithMatchAllFilters(t *testing.T) {
	tSince, _ := time.Parse(time.RFC3339Nano, "2019-01-02T15:00:00.104561Z")
	since, _ := ptypes.TimestampProto(tSince)
	tUntil, _ := time.Parse(time.RFC3339Nano, "2019-01-02T16:00:00.104561Z")
	until, _ := ptypes.TimestampProto(tUntil)
	w := &pb.Work{
		TargetSubstring: "9d947172-9f21-4449-8460-e53c8c9e87fc",
		Since:           since,
		Until:           until,
	}
	ProcessDoWorkTest(t, w, 1)
}

func TestDoWorkOutOfTimeFilter(t *testing.T) {
	tSince, _ := time.Parse(time.RFC3339Nano, "2019-01-02T15:00:00.104561Z")
	since, _ := ptypes.TimestampProto(tSince)
	tUntil, _ := time.Parse(time.RFC3339Nano, "2019-01-02T16:00:00.104561Z")
	until, _ := ptypes.TimestampProto(tUntil)
	w := &pb.Work{
		TargetSubstring: "39aec93e-031b-4002-8c0a-4ddcd92e250b",
		Since:           since,
		Until:           until,
	}
	ProcessDoWorkTest(t, w, 0)
}

func ProcessDoWorkTest(t *testing.T, w *pb.Work, expectedCount int) {
	s := &serverType{logReader: &inMemoryReader{}, send: getResult}
	s.DoWork(w, nil)
	if len(workResult) != expectedCount {
		t.Fatalf("Expected %v line, got %v", expectedCount, len(workResult))
	}
}

var workResult []*pb.LogLine

func getResult(ch chan *lineEntry, server pb.Worker_DoWorkServer) {
	results := make([]*pb.LogLine, 0)
	for {
		line, hasMore := <-ch
		if line.err == io.EOF || !hasMore {
			break
		}

		entry := line.logEntry
		pbLine := &pb.LogLine{
			Entry:     *entry.log,
			Timestamp: &ts.Timestamp{Seconds: entry.time.Unix(), Nanos: int32(entry.time.Nanosecond())}}
		results = append(results, pbLine)
	}
	workResult = results
}

type inMemoryReader struct{}

func (*inMemoryReader) downloadAndDecompress(objectPath string) (io.Reader, error) {
	arr := []string{line1, line2, line3}
	text := strings.Join(arr, "\n") + "\n"
	return strings.NewReader(text), nil
}

const line1 = `{"kind":"Event","apiVersion":"audit.k8s.io/v1","level":"Metadata","auditID":"0286b87c-b86c-443f-bb1e-807844b35307","stage":"ResponseComplete","requestURI":"/apis/coordination.k8s.io/v1beta1/namespaces/kube-node-lease/leases/gce-scale-cluster-minion-group-4-zr07?timeout=10s","verb":"update","user":{"username":"system:node:gce-scale-cluster-minion-group-4-zr07","groups":["system:nodes","system:authenticated"]},"sourceIPs":["35.227.76.61"],"userAgent":"kubelet/v1.14.0 (linux/amd64) kubernetes/aef1179","objectRef":{"resource":"leases","namespace":"kube-node-lease","name":"gce-scale-cluster-minion-group-4-zr07","uid":"411615ce-0e66-11e9-a584-42010a280002","apiGroup":"coordination.k8s.io","apiVersion":"v1beta1","resourceVersion":"17652814"},"responseStatus":{"metadata":{},"code":200},"requestReceivedTimestamp":"2019-01-02T16:01:16.105964Z","stageTimestamp":"2019-01-02T15:01:16.108038Z","annotations":{"authorization.k8s.io/decision":"allow","authorization.k8s.io/reason":""}}\r\n`
const line2 = `{"kind":"Event","apiVersion":"audit.k8s.io/v1","level":"Metadata","auditID":"9d947172-9f21-4449-8460-e53c8c9e87fc","stage":"ResponseComplete","requestURI":"/apis/coordination.k8s.io/v1beta1/namespaces/kube-node-lease/leases/gce-scale-cluster-minion-group-2-v1fj?timeout=10s","verb":"update","user":{"username":"system:node:gce-scale-cluster-minion-group-2-v1fj","groups":["system:nodes","system:authenticated"]},"sourceIPs":["35.229.86.146"],"userAgent":"kubelet/v1.14.0 (linux/amd64) kubernetes/aef1179","objectRef":{"resource":"leases","namespace":"kube-node-lease","name":"gce-scale-cluster-minion-group-2-v1fj","uid":"f188f92d-0e65-11e9-a584-42010a280002","apiGroup":"coordination.k8s.io","apiVersion":"v1beta1","resourceVersion":"17652718"},"responseStatus":{"metadata":{},"code":200},"requestReceivedTimestamp":"2019-01-02T15:01:16.106483Z","stageTimestamp":"2019-01-02T15:01:16.108244Z","annotations":{"authorization.k8s.io/decision":"allow","authorization.k8s.io/reason":""}}`
const line3 = `{"kind":"Event","apiVersion":"audit.k8s.io/v1","level":"Request","auditID":"39aec93e-031b-4002-8c0a-4ddcd92e250b","stage":"ResponseComplete","requestURI":"/api/v1/nodes/gce-scale-cluster-minion-group-2-t86q/status","verb":"patch","user":{"username":"system:node-problem-detector","uid":"uid:node-problem-detector","groups":["system:authenticated"]},"sourceIPs":["35.196.154.110"],"userAgent":"node-problem-detector/v0.5.0-49-gfb81368","objectRef":{"resource":"nodes","name":"gce-scale-cluster-minion-group-2-t86q","apiVersion":"v1","subresource":"status"},"responseStatus":{"metadata":{},"code":200},"requestObject":{"status":{"conditions":[{"type":"FrequentKubeletRestart","status":"False","lastHeartbeatTime":"2019-01-02T15:01:16Z","lastTransitionTime":"2019-01-02T08:15:27Z","reason":"FrequentKubeletRestart"},{"type":"FrequentDockerRestart","status":"False","lastHeartbeatTime":"2019-01-02T15:01:16Z","lastTransitionTime":"2019-01-02T08:15:28Z","reason":"FrequentDockerRestart"},{"type":"FrequentContainerdRestart","status":"False","lastHeartbeatTime":"2019-01-02T15:01:16Z","lastTransitionTime":"2019-01-02T08:15:29Z","reason":"FrequentContainerdRestart"},{"type":"CorruptDockerOverlay2","status":"False","lastHeartbeatTime":"2019-01-02T15:01:16Z","lastTransitionTime":"2019-01-02T08:15:27Z","reason":"CorruptDockerOverlay2"},{"type":"KernelDeadlock","status":"False","lastHeartbeatTime":"2019-01-02T15:01:16Z","lastTransitionTime":"2019-01-02T08:10:26Z","reason":"KernelHasNoDeadlock","message":"kernel has no deadlock"},{"type":"ReadonlyFilesystem","status":"False","lastHeartbeatTime":"2019-01-02T15:01:16Z","lastTransitionTime":"2019-01-02T08:10:26Z","reason":"FilesystemIsNotReadOnly","message":"Filesystem is not read-only"},{"type":"FrequentUnregisterNetDevice","status":"False","lastHeartbeatTime":"2019-01-02T15:01:16Z","lastTransitionTime":"2019-01-02T08:15:27Z","reason":"UnregisterNetDevice"}]}},"requestReceivedTimestamp":"2019-01-02T14:01:16.104561Z","stageTimestamp":"2019-01-02T15:01:16.108460Z","annotations":{"authorization.k8s.io/decision":"allow","authorization.k8s.io/reason":"RBAC: allowed by ClusterRoleBinding \"npd-binding\" of ClusterRole \"system:node-problem-detector\" to User \"system:node-problem-detector\""}}`
