/*
Copyright 2015 The Kubernetes Authors.

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

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"

	"strings"

	"k8s.io/contrib/test-utils/utils"
	"k8s.io/test-infra/mungegithub/options"
)

type testHandler struct {
	handler func(http.ResponseWriter, *http.Request)
}

func (t *testHandler) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	t.handler(res, req)
}

func marshalOrDie(obj interface{}, t *testing.T) []byte {
	data, err := json.Marshal(obj)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	return data
}

func genMockGCSListResponse(files ...string) []byte {
	respTemplate := "{\"items\":[%s]}"
	itemTemplate := "{\"name\":\"%s\"}"
	items := []string{}
	for _, file := range files {
		items = append(items, fmt.Sprintf(itemTemplate, file))
	}
	return []byte(fmt.Sprintf(respTemplate, strings.Join(items, ",")))
}

func TestCheckGCSBuilds(t *testing.T) {
	latestBuildNumberFoo := 42
	latestBuildNumberBar := 44
	latestBuildNumberBaz := 99
	tests := []struct {
		paths             map[string][]byte
		expectedLastBuild int
		expectedStatus    map[string]BuildInfo
	}{
		{
			paths: map[string][]byte{
				"/bucket/logs/foo/latest-build.txt": []byte(strconv.Itoa(latestBuildNumberFoo)),
				fmt.Sprintf("/bucket/logs/foo/%v/finished.json", latestBuildNumberFoo): marshalOrDie(utils.FinishedFile{
					Result:    "SUCCESS",
					Timestamp: 1234,
				}, t),
				"/bucket/logs/bar/latest-build.txt": []byte(strconv.Itoa(latestBuildNumberBar)),
				fmt.Sprintf("/bucket/logs/bar/%v/finished.json", latestBuildNumberBar): marshalOrDie(utils.FinishedFile{
					Result:    "SUCCESS",
					Timestamp: 1234,
				}, t),
				"/bucket/logs/baz/latest-build.txt": []byte(strconv.Itoa(latestBuildNumberBaz)),
				fmt.Sprintf("/bucket/logs/baz/%v/finished.json", latestBuildNumberBaz): marshalOrDie(utils.FinishedFile{
					Result:    "UNSTABLE",
					Timestamp: 1234,
				}, t),
				"/storage/v1/b/bucket/o": genMockGCSListResponse(),
			},
			expectedStatus: map[string]BuildInfo{
				"foo": {Status: "[nonblocking] Stable", ID: "42"},
				"bar": {Status: "[nonblocking] Stable", ID: "44"},
				"baz": {Status: "[nonblocking] Not Stable", ID: "99"},
			},
		},
		{
			paths: map[string][]byte{
				"/bucket/logs/foo/latest-build.txt": []byte(strconv.Itoa(latestBuildNumberFoo)),
				fmt.Sprintf("/bucket/logs/foo/%v/finished.json", latestBuildNumberFoo): marshalOrDie(utils.FinishedFile{
					Result:    "SUCCESS",
					Timestamp: 1234,
				}, t),
				"/bucket/logs/bar/latest-build.txt": []byte(strconv.Itoa(latestBuildNumberBar)),
				fmt.Sprintf("/bucket/logs/bar/%v/finished.json", latestBuildNumberBar): marshalOrDie(utils.FinishedFile{
					Result:    "UNSTABLE",
					Timestamp: 1234,
				}, t),
				"/bucket/logs/baz/latest-build.txt": []byte(strconv.Itoa(latestBuildNumberBaz)),
				fmt.Sprintf("/bucket/logs/baz/%v/finished.json", latestBuildNumberBaz): marshalOrDie(utils.FinishedFile{
					Result:    "SUCCESS",
					Timestamp: 1234,
				}, t),
				"/storage/v1/b/bucket/o": genMockGCSListResponse(),
			},
			expectedStatus: map[string]BuildInfo{
				"foo": {Status: "[nonblocking] Stable", ID: "42"},
				"bar": {Status: "[nonblocking] Not Stable", ID: "44"},
				"baz": {Status: "[nonblocking] Stable", ID: "99"},
			},
		},
		{
			paths: map[string][]byte{
				"/bucket/logs/foo/latest-build.txt": []byte(strconv.Itoa(latestBuildNumberFoo)),
				fmt.Sprintf("/bucket/logs/foo/%v/finished.json", latestBuildNumberFoo): marshalOrDie(utils.FinishedFile{
					Result:    "SUCCESS",
					Timestamp: 1234,
				}, t),
				"/bucket/logs/bar/latest-build.txt": []byte(strconv.Itoa(latestBuildNumberBar)),
				fmt.Sprintf("/bucket/logs/bar/%v/finished.json", latestBuildNumberBar): marshalOrDie(utils.FinishedFile{
					Result:    "UNSTABLE",
					Timestamp: 1234,
				}, t),
				fmt.Sprintf("/bucket/logs/bar/%v/artifacts/junit_01.xml", latestBuildNumberBar-1): getJUnit(5, 0),
				fmt.Sprintf("/bucket/logs/bar/%v/artifacts/junit_02.xml", latestBuildNumberBar-1): getRealJUnitFailure(),
				fmt.Sprintf("/bucket/logs/bar/%v/artifacts/junit_03.xml", latestBuildNumberBar-1): getJUnit(5, 0),
				fmt.Sprintf("/bucket/logs/bar/%v/artifacts/junit_01.xml", latestBuildNumberBar):   getJUnit(5, 0),
				fmt.Sprintf("/bucket/logs/bar/%v/artifacts/junit_02.xml", latestBuildNumberBar):   getRealJUnitFailure(),
				fmt.Sprintf("/bucket/logs/bar/%v/artifacts/junit_03.xml", latestBuildNumberBar):   getJUnit(5, 0),
				fmt.Sprintf("/bucket/logs/bar/%v/finished.json", latestBuildNumberBar-1): marshalOrDie(utils.FinishedFile{
					Result:    "UNSTABLE",
					Timestamp: 999,
				}, t),
				"/storage/v1/b/bucket/o": genMockGCSListResponse(
					fmt.Sprintf("/bucket/logs/bar/%v/artifacts/junit_01.xml", latestBuildNumberBar-1),
					fmt.Sprintf("/bucket/logs/bar/%v/artifacts/junit_02.xml", latestBuildNumberBar-1),
					fmt.Sprintf("/bucket/logs/bar/%v/artifacts/junit_03.xml", latestBuildNumberBar-1),
					fmt.Sprintf("/bucket/logs/bar/%v/artifacts/junit_01.xml", latestBuildNumberBar),
					fmt.Sprintf("/bucket/logs/bar/%v/artifacts/junit_02.xml", latestBuildNumberBar),
					fmt.Sprintf("/bucket/logs/bar/%v/artifacts/junit_03.xml", latestBuildNumberBar),
				),
			},
			expectedStatus: map[string]BuildInfo{
				"foo": {Status: "[nonblocking] Stable", ID: "42"},
				"bar": {Status: "[nonblocking] Not Stable", ID: "44"},
				"baz": {Status: "[nonblocking] Not Stable", ID: "-1"},
			},
		},

		{
			paths: map[string][]byte{
				"/bucket/logs/foo/latest-build.txt": []byte(strconv.Itoa(latestBuildNumberFoo)),
				fmt.Sprintf("/bucket/logs/foo/%v/finished.json", latestBuildNumberFoo): marshalOrDie(utils.FinishedFile{
					Result:    "SUCCESS",
					Timestamp: 1234,
				}, t),
				"/bucket/logs/bar/latest-build.txt": []byte(strconv.Itoa(latestBuildNumberBar)),
				fmt.Sprintf("/bucket/logs/bar/%v/finished.json", latestBuildNumberBar): marshalOrDie(utils.FinishedFile{
					Result:    "FAILURE",
					Timestamp: 1234,
				}, t),
				"/storage/v1/b/bucket/o": genMockGCSListResponse(),
			},
			expectedStatus: map[string]BuildInfo{
				"foo": {Status: "[nonblocking] Stable", ID: "42"},
				"bar": {Status: "[nonblocking] Not Stable", ID: "44"},
				"baz": {Status: "[nonblocking] Not Stable", ID: "-1"},
			},
		},
		{
			paths: map[string][]byte{
				"/bucket/logs/foo/latest-build.txt": []byte(strconv.Itoa(latestBuildNumberFoo)),
				fmt.Sprintf("/bucket/logs/foo/%v/finished.json", latestBuildNumberFoo): marshalOrDie(utils.FinishedFile{
					Result:    "FAILURE",
					Timestamp: 1234,
				}, t),
				"/bucket/logs/bar/latest-build.txt": []byte(strconv.Itoa(latestBuildNumberBar)),
				fmt.Sprintf("/bucket/logs/bar/%v/finished.json", latestBuildNumberBar): marshalOrDie(utils.FinishedFile{
					Result:    "UNSTABLE",
					Timestamp: 1234,
				}, t),
				"/storage/v1/b/bucket/o": genMockGCSListResponse(),
			},
			expectedStatus: map[string]BuildInfo{
				"foo": {Status: "[nonblocking] Not Stable", ID: "42"},
				"bar": {Status: "[nonblocking] Not Stable", ID: "44"},
				"baz": {Status: "[nonblocking] Not Stable", ID: "-1"},
			},
		},
	}
	for index, test := range tests {
		server := httptest.NewServer(&testHandler{
			handler: func(res http.ResponseWriter, req *http.Request) {
				data, found := test.paths[req.URL.Path]
				if !found {
					res.WriteHeader(http.StatusNotFound)
					fmt.Fprintf(res, "Unknown path: %s", req.URL.Path)
					return
				}
				res.WriteHeader(http.StatusOK)
				res.Write(data)
			},
		})
		jobs := []string{"foo", "bar", "baz"}
		e2e := &RealE2ETester{
			Opts:                 options.New(),
			NonBlockingJobNames:  &jobs,
			BuildStatus:          map[string]BuildInfo{},
			GoogleGCSBucketUtils: utils.NewTestUtils("bucket", "logs", server.URL),
		}
		e2e.Init(nil)

		e2e.LoadNonBlockingStatus()
		if !reflect.DeepEqual(test.expectedStatus, e2e.BuildStatus) {
			t.Errorf("%v: expected: %v, saw: %v", index, test.expectedStatus, e2e.BuildStatus)
		}
	}
}

func getJUnit(testsNo int, failuresNo int) []byte {
	return []byte(fmt.Sprintf("%v\n<testsuite tests=\"%v\" failures=\"%v\" time=\"1234\">\n</testsuite>",
		ExpectedXMLHeader, testsNo, failuresNo))
}

func getOtherRealJUnitFailure() []byte {
	return []byte(`<testsuite tests="7" failures="1" time="275.882258919">
<testcase name="[k8s.io] ResourceQuota should create a ResourceQuota and capture the life of a loadBalancer service." classname="Kubernetes e2e suite" time="17.759834805"/>
<testcase name="[k8s.io] ResourceQuota should create a ResourceQuota and capture the life of a secret." classname="Kubernetes e2e suite" time="21.201547548"/>
<testcase name="OTHER [k8s.io] Kubectl client [k8s.io] Kubectl patch should add annotations for pods in rc [Conformance]" classname="Kubernetes e2e suite" time="126.756441938">
<failure type="Failure">
/go/src/k8s.io/kubernetes/_output/dockerized/go/src/k8s.io/kubernetes/test/e2e/kubectl.go:972 May 18 13:02:24.715: No pods matched the filter.
</failure>
</testcase>
<testcase name="[k8s.io] hostPath should give a volume the correct mode [Conformance]" classname="Kubernetes e2e suite" time="9.246191421"/>
<testcase name="[k8s.io] Volumes [Feature:Volumes] [k8s.io] Ceph RBD should be mountable" classname="Kubernetes e2e suite" time="0">
<skipped/>
</testcase>
<testcase name="[k8s.io] Deployment deployment should label adopted RSs and pods" classname="Kubernetes e2e suite" time="16.557498555"/>
<testcase name="[k8s.io] ConfigMap should be consumable from pods in volume as non-root with FSGroup [Feature:FSGroup]" classname="Kubernetes e2e suite" time="0">
<skipped/>
</testcase>
<testcase name="[k8s.io] V1Job should scale a job down" classname="Kubernetes e2e suite" time="77.122626914"/>
<testcase name="[k8s.io] EmptyDir volumes volume on default medium should have the correct mode [Conformance]" classname="Kubernetes e2e suite" time="7.169679079"/>
<testcase name="[k8s.io] Reboot [Disruptive] [Feature:Reboot] each node by ordering unclean reboot and ensure they function upon restart" classname="Kubernetes e2e suite" time="0">
<skipped/>
</testcase>
</testsuite>`)
}

func getRealJUnitFailure() []byte {
	return []byte(`<testsuite tests="7" failures="1" time="275.882258919">
<testcase name="[k8s.io] ResourceQuota should create a ResourceQuota and capture the life of a loadBalancer service." classname="Kubernetes e2e suite" time="17.759834805"/>
<testcase name="[k8s.io] ResourceQuota should create a ResourceQuota and capture the life of a secret." classname="Kubernetes e2e suite" time="21.201547548"/>
<testcase name="[k8s.io] Kubectl client [k8s.io] Kubectl patch should add annotations for pods in rc [Conformance]" classname="Kubernetes e2e suite" time="126.756441938">
<failure type="Failure">
/go/src/k8s.io/kubernetes/_output/dockerized/go/src/k8s.io/kubernetes/test/e2e/kubectl.go:972 May 18 13:02:24.715: No pods matched the filter.
</failure>
</testcase>
<testcase name="[k8s.io] hostPath should give a volume the correct mode [Conformance]" classname="Kubernetes e2e suite" time="9.246191421"/>
<testcase name="[k8s.io] Volumes [Feature:Volumes] [k8s.io] Ceph RBD should be mountable" classname="Kubernetes e2e suite" time="0">
<skipped/>
</testcase>
<testcase name="[k8s.io] Deployment deployment should label adopted RSs and pods" classname="Kubernetes e2e suite" time="16.557498555"/>
<testcase name="[k8s.io] ConfigMap should be consumable from pods in volume as non-root with FSGroup [Feature:FSGroup]" classname="Kubernetes e2e suite" time="0">
<skipped/>
</testcase>
<testcase name="[k8s.io] V1Job should scale a job down" classname="Kubernetes e2e suite" time="77.122626914"/>
<testcase name="[k8s.io] EmptyDir volumes volume on default medium should have the correct mode [Conformance]" classname="Kubernetes e2e suite" time="7.169679079"/>
<testcase name="[k8s.io] Reboot [Disruptive] [Feature:Reboot] each node by ordering unclean reboot and ensure they function upon restart" classname="Kubernetes e2e suite" time="0">
<skipped/>
</testcase>
</testsuite>`)
}

func getRealJUnitFailureWithTestSuitesTag() []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
	<testsuite tests="52" failures="2" time="374.434" name="k8s.io/kubernetes/test/integration">
		<properties>
			<property name="go.version" value="go1.6.2"></property>
		</properties>
		<testcase classname="integration" name="TestMasterProcessMetrics" time="0.070"></testcase>
		<testcase classname="integration" name="TestApiserverMetrics" time="0.070"></testcase>
		<testcase classname="integration" name="TestMasterExportsSymbols" time="0.000"></testcase>
		<testcase classname="integration" name="TestPersistentVolumeRecycler" time="20.460"></testcase>
		<testcase classname="integration" name="TestPersistentVolumeMultiPVs" time="10.240">
			<failure message="Failed" type="">persistent_volumes_test.go:254: volumes created&#xA;persistent_volumes_test.go:260: claim created&#xA;persistent_volumes_test.go:264: volume bound&#xA;persistent_volumes_test.go:266: claim bound&#xA;persistent_volumes_test.go:284: Bind mismatch! Expected pvc-2 capacity 50000000000 but got fake-pvc-72 capacity 5000000000</failure>
		</testcase>
		<testcase classname="integration" name="TestPersistentVolumeMultiPVsPVCs" time="3.370">
			<failure message="Failed" type="">persistent_volumes_test.go:379: PVC &#34;pvc-0&#34; is not bound</failure>
		</testcase>
		<testcase classname="integration" name="TestPersistentVolumeMultiPVsDiffAccessModes" time="10.110"></testcase>
	</testsuite>
</testsuites>
`)
}

func TestJUnitFailureParse(t *testing.T) {
	//parse junit xml result with <testsuite> as top tag
	junitFailReader := bytes.NewReader(getRealJUnitFailure())
	got, err := getJUnitFailures(junitFailReader)
	if err != nil {
		t.Fatalf("Parse error? %v", err)
	}
	if e, a := map[string]string{
		"[k8s.io] Kubectl client [k8s.io] Kubectl patch should add annotations for pods in rc [Conformance] {Kubernetes e2e suite}": `
/go/src/k8s.io/kubernetes/_output/dockerized/go/src/k8s.io/kubernetes/test/e2e/kubectl.go:972 May 18 13:02:24.715: No pods matched the filter.
`,
	}, got; !reflect.DeepEqual(e, a) {
		t.Errorf("Expected %v, got %v", e, a)
	}

	//parse junit xml result with <testsuites> as top tag
	junitFailReader = bytes.NewReader(getRealJUnitFailureWithTestSuitesTag())
	got, err = getJUnitFailures(junitFailReader)
	if err != nil {
		t.Fatalf("Parse error? %v", err)
	}
	if e, a := map[string]string{
		"TestPersistentVolumeMultiPVs {integration}":     "persistent_volumes_test.go:254: volumes created\npersistent_volumes_test.go:260: claim created\npersistent_volumes_test.go:264: volume bound\npersistent_volumes_test.go:266: claim bound\npersistent_volumes_test.go:284: Bind mismatch! Expected pvc-2 capacity 50000000000 but got fake-pvc-72 capacity 5000000000",
		"TestPersistentVolumeMultiPVsPVCs {integration}": `persistent_volumes_test.go:379: PVC "pvc-0" is not bound`,
	}, got; !reflect.DeepEqual(e, a) {
		t.Errorf("Expected %v, got %v", e, a)
	}
}
