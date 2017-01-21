/*
Copyright 2016 The Kubernetes Authors.

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

package mungers

import (
	"strings"
	"testing"

	"time"

	"github.com/google/go-github/github"
	github_testing "k8s.io/contrib/mungegithub/github/testing"
	cache "k8s.io/contrib/mungegithub/mungers/flakesync"
	"k8s.io/contrib/mungegithub/mungers/sync"
	"k8s.io/contrib/test-utils/utils"
)

func makeTestFlakeManager() *FlakeManager {
	bucketUtils := utils.NewUtils("bucket", "logs")
	return &FlakeManager{
		sq:                   nil,
		config:               nil,
		googleGCSBucketUtils: bucketUtils,
	}
}

func expect(t *testing.T, actual, expected string) {
	if actual != expected {
		t.Errorf("expected `%s` to be `%s`", actual, expected)
	}
}

func expectContains(t *testing.T, haystack, needle, desc string) {
	if !strings.Contains(haystack, needle) {
		t.Errorf("%s: `%v` not in `%v`", desc, needle, haystack)
	}
}

func checkCommon(t *testing.T, source sync.IssueSource) {
	expect(t, source.ID(), "/bucket/logs/e2e-gce/123/")
	expectContains(t, source.Body(false), source.ID(),
		"Body() does not contain ID()")
	expectContains(t, "https://storage.googleapis.com/"+
		"bucket/logs/e2e-gce/123/",
		source.ID(),
		"ID() is not compatible with older IDs")
	expectContains(t, source.Body(false),
		"https://k8s-gubernator.appspot.com/build"+source.ID(),
		"Body() does not contain gubernator link")
}
func TestIndividualFlakeSource(t *testing.T) {
	fm := makeTestFlakeManager()
	flake := cache.Flake{
		Job:    "e2e-gce",
		Number: 123,
		Test:   "[k8s.io] Latency",
		Reason: "Took too long!",
	}
	source := individualFlakeSource{flake, fm}
	expect(t, source.Title(), "[k8s.io] Latency")
	checkCommon(t, &source)
}

func TestIndividualFlakeSourceAddTo(t *testing.T) {
	fm := makeTestFlakeManager()
	flakeA := cache.Flake{
		Job:    "e2e-gce",
		Number: 123,
		Test:   "[k8s.io] Latency",
		Reason: "exit status 1",
	}
	flakeB := flakeA
	flakeB.Number = 124
	sourceA := individualFlakeSource{flakeA, fm}
	sourceB := individualFlakeSource{flakeB, fm}

	body := sourceA.Body(false)
	combined := sourceB.AddTo(body)
	expectContains(t, combined, sourceA.ID(), "Body doesn't contain A id")
	expectContains(t, combined, sourceB.ID(), "Body doesn't contain B id")
	expectContains(t, combined, flakeA.Reason, "Body doesn't contain failure reason")

	// TODO: handle "happened on a presubmit run"
	combinedAgain := sourceB.AddTo(combined)
	if combinedAgain != combined {
		t.Fatalf("AddTo is not idempotent: `%s` != `%s`", combined, combinedAgain)
	}
}

func TestBrokenJobSource(t *testing.T) {
	fm := makeTestFlakeManager()
	result := cache.Result{
		Job:    "e2e-gce",
		Number: 123,
	}
	source := brokenJobSource{&result, fm}
	expect(t, source.Title(), "e2e-gce: broken test run")
	checkCommon(t, &source)
}

func flakecomment(id int, createdAt time.Time) *github.IssueComment {
	return github_testing.Comment(id, "k8s-bot", createdAt, "Failed: something failed")
}

func TestAutoPrioritize(t *testing.T) {
	var p0Comments []*github.IssueComment

	//simulates 50 test flakes/comments in the last 50 hours
	for i := 0; i < 50; i++ {
		p0Comments = append(p0Comments, flakecomment(1, time.Now().Add(-time.Duration(i)*time.Hour)))
	}

	testcases := []struct {
		comments       []*github.IssueComment
		issueCreatedAt time.Time
		expectPriority int
	}{
		{
			//0 flakes in the last 7 days
			comments:       []*github.IssueComment{},
			issueCreatedAt: time.Now(),
			expectPriority: 3,
		},
		{
			//1 flakes in the last 7 days
			comments: []*github.IssueComment{
				flakecomment(1, time.Now()),
			},
			issueCreatedAt: time.Now().Add(-1 * 29 * 24 * time.Hour),
			expectPriority: 3,
		},
		{
			//3 flakes in the last 7 days
			comments: []*github.IssueComment{
				flakecomment(1, time.Now()),
				flakecomment(1, time.Now().Add(-1*3*24*time.Hour)),
				flakecomment(1, time.Now().Add(-1*6*24*time.Hour)),
			},
			issueCreatedAt: time.Now().Add(-1 * 30 * 24 * time.Hour),
			expectPriority: 2,
		},
		{
			//10 flakes in the last 10 hrs
			comments: []*github.IssueComment{
				flakecomment(1, time.Now()),
				flakecomment(1, time.Now().Add(-1*time.Hour)),
				flakecomment(1, time.Now().Add(-2*time.Hour)),
				flakecomment(1, time.Now().Add(-3*time.Hour)),
				flakecomment(1, time.Now().Add(-4*time.Hour)),
				flakecomment(1, time.Now().Add(-5*time.Hour)),
				flakecomment(1, time.Now().Add(-6*time.Hour)),
				flakecomment(1, time.Now().Add(-7*time.Hour)),
				flakecomment(1, time.Now().Add(-8*time.Hour)),
				flakecomment(1, time.Now().Add(-9*time.Hour)),
			},
			issueCreatedAt: time.Now().Add(-1 * 29 * 24 * time.Hour),
			expectPriority: 1,
		},
		{
			//4 flakes, but only 2 in a week
			comments: []*github.IssueComment{
				flakecomment(1, time.Now()),
				flakecomment(1, time.Now().Add(-3*24*time.Hour)),
				flakecomment(1, time.Now().Add(-15*24*time.Hour)),
				flakecomment(1, time.Now().Add(-20*24*time.Hour)),
			},
			issueCreatedAt: time.Now().Add(-1 * 29 * 24 * time.Hour),
			expectPriority: 3,
		},
		{
			//50 flakes in a week
			comments:       p0Comments,
			issueCreatedAt: time.Now().Add(-1 * 6 * 24 * time.Hour),
			expectPriority: 0,
		},
	}
	for _, tc := range testcases {
		p := autoPrioritize(tc.comments, &tc.issueCreatedAt)
		if p.Priority() != tc.expectPriority {
			t.Errorf("Expected priority: %d, But got: %d",
				len(tc.comments), tc.expectPriority, p.Priority())
		}
	}
}

func TestPullRE(t *testing.T) {
	table := []struct {
		path   string
		expect string
	}{
		{"/kubernetes-jenkins/pr-logs/pull/27898/kubernetes-pull-build-test-e2e-gce/47123/", "27898"},
		{"kubernetes-jenkins/logs/kubernetes-e2e-gke-test/13095/", ""},
	}
	for _, tt := range table {
		got := ""
		if parts := pullRE.FindStringSubmatch(tt.path); len(parts) > 1 {
			got = parts[1]
		}
		if got != tt.expect {
			t.Errorf("Expected %v, got %v", tt.expect, got)
		}
	}
}

func TestFlakeReasonsAreEquivalent(t *testing.T) {
	testcases := []struct {
		expected bool
		a, b     string
	}{
		{true, "exit status 1", "exit status 1"},
		{false, "exit status 1", "exit status 2"},
		{true, // https://github.com/kubernetes/kubernetes/issues/33756
			`/go/src/k8s.io/kubernetes/_output/dockerized/go/src/k8s.io/kubernetes/test/e2e/kubectl.go:414
Dec 11 17:04:07.433: Unexpected kubectl exec output. Wanted "running in container", got ""
/go/src/k8s.io/kubernetes/_output/dockerized/go/src/k8s.io/kubernetes/test/e2e/kubectl.go:388`,
			`/go/src/k8s.io/kubernetes/_output/dockerized/go/src/k8s.io/kubernetes/test/e2e/kubectl.go:414
Dec 12 00:26:55.408: Unexpected kubectl exec output. Wanted "running in container", got ""
/go/src/k8s.io/kubernetes/_output/dockerized/go/src/k8s.io/kubernetes/test/e2e/kubectl.go:388`,
		},
		{true,
			`/go/src/k8s.io/kubernetes/_output/dockerized/go/src/k8s.io/kubernetes/test/e2e/kubectl.go:521
Expected error:
    <exec.CodeExitError>: {
        Err: {
            s: "error running &{/workspace/kubernetes_skew/cluster/kubectl.sh [/workspace/kubernetes_skew/cluster/kubectl.sh --server=https://104.198.226.155 --kubeconfig=/workspace/.kube/config get service redis-master --namespace=e2e-tests-kubectl-otsd4 -o jsonpath={.spec.ports[0].nodePort}] []  <nil> Error executing template: nodePort is not found. Printing more information for debugging the template:\n\ttemplate was:\n\t\t{.spec.ports[0].nodePort}\n\tobject given to jsonpath engine was:\n\t\tmap[string]interface {}{\"kind\":\"Service\", \"apiVersion\":\"v1\", \"metadata\":map[string]interface {}{\"resourceVersion\":\"1168\", \"creationTimestamp\":\"2016-12-02T23:26:39Z\", \"labels\":map[string]interface {}{\"app\":\"redis\", \"role\":\"master\"}, \"name\":\"redis-master\", \"namespace\":\"e2e-tests-kubectl-otsd4\", \"selfLink\":\"/api/v1/namespaces/e2e-tests-kubectl-otsd4/services/redis-master\", \"uid\":\"c6836226-b8e6-11e6-a09b-42010af00002\"}, \"spec\":map[string]interface {}{\"ports\":[]interface {}{map[string]interface {}{\"protocol\":\"TCP\", \"port\":6379, \"targetPort\":\"redis-server\"}}, \"selector\":map[string]interface {}{\"app\":\"redis\", \"role\":\"master\"}, \"clusterIP\":\"10.0.217.213\", \"type\":\"ClusterIP\", \"sessionAffinity\":\"None\"}, \"status\":map[string]interface {}{\"loadBalancer\":map[string]interface {}{}}}\n\n error: error executing jsonpath \"{.spec.ports[0].nodePort}\": nodePort is not found\n [] <nil> 0xc820933ec0 exit status 1 <nil> true [0xc8201742a8 0xc8201742c0 0xc8201744c8] [0xc8201742a8 0xc8201742c0 0xc8201744c8] [0xc8201742b8 0xc8201744c0] [0xaf8950 0xaf8950] 0xc820e2a1e0}:\nCommand stdout:\nError executing template: nodePort is not found. Printing more information for debugging the template:\n\ttemplate was:\n\t\t{.spec.ports[0].nodePort}\n\tobject given to jsonpath engine was:\n\t\tmap[string]interface {}{\"kind\":\"Service\", \"apiVersion\":\"v1\", \"metadata\":map[string]interface {}{\"resourceVersion\":\"1168\", \"creationTimestamp\":\"2016-12-02T23:26:39Z\", \"labels\":map[string]interface {}{\"app\":\"redis\", \"role\":\"master\"}, \"name\":\"redis-master\", \"namespace\":\"e2e-tests-kubectl-otsd4\", \"selfLink\":\"/api/v1/namespaces/e2e-tests-kubectl-otsd4/services/redis-master\", \"uid\":\"c6836226-b8e6-11e6-a09b-42010af00002\"}, \"spec\":map[string]interface {}{\"ports\":[]interface {}{map[string]interface {}{\"protocol\":\"TCP\", \"port\":6379, \"targetPort\":\"redis-server\"}}, \"selector\":map[string]interface {}{\"app\":\"redis\", \"role\":\"master\"}, \"clusterIP\":\"10.0.217.213\", \"type\":\"ClusterIP\", \"sessionAffinity\":\"None\"}, \"status\":map[string]interface {}{\"loadBalancer\":map[string]interface {}{}}}\n\n\nstderr:\nerror: error executing jsonpath \"{.spec.ports[0].nodePort}\": nodePort is not found\n\nerror:\nexit status 1\n",
        },
        Code: 1,
    }
    error running &{/workspace/kubernetes_skew/cluster/kubectl.sh [/workspace/kubernetes_skew/cluster/kubectl.sh --server=https://104.198.226.155 --kubeconfig=/workspace/.kube/config get service redis-master --namespace=e2e-tests-kubectl-otsd4 -o jsonpath={.spec.ports[0].nodePort}] []  <nil> Error executing template: nodePort is not found. Printing more information for debugging the template:
    	template was:
    		{.spec.ports[0].nodePort}
    	object given to jsonpath engine was:
    		map[string]interface {}{"kind":"Service", "apiVersion":"v1", "metadata":map[string]interface {}{"resourceVersion":"1168", "creationTimestamp":"2016-12-02T23:26:39Z", "labels":map[string]interface {}{"app":"redis", "role":"master"}, "name":"redis-master", "namespace":"e2e-tests-kubectl-otsd4", "selfLink":"/api/v1/namespaces/e2e-tests-kubectl-otsd4/services/redis-master", "uid":"c6836226-b8e6-11e6-a09b-42010af00002"}, "spec":map[string]interface {}{"ports":[]interface {}{map[string]interface {}{"protocol":"TCP", "port":6379, "targetPort":"redis-server"}}, "selector":map[string]interface {}{"app":"redis", "role":"master"}, "clusterIP":"10.0.217.213", "type":"ClusterIP", "sessionAffinity":"None"}, "status":map[string]interface {}{"loadBalancer":map[string]interface {}{}}}
    
     error: error executing jsonpath "{.spec.ports[0].nodePort}": nodePort is not found
     [] <nil> 0xc820933ec0 exit status 1 <nil> true [0xc8201742a8 0xc8201742c0 0xc8201744c8] [0xc8201742a8 0xc8201742c0 0xc8201744c8] [0xc8201742b8 0xc8201744c0] [0xaf8950 0xaf8950] 0xc820e2a1e0}:
    Command stdout:
    Error executing template: nodePort is not found. Printing more information for debugging the template:
    	template was:
    		{.spec.ports[0].nodePort}
    	object given to jsonpath engine was:
    		map[string]interface {}{"kind":"Service", "apiVersion":"v1", "metadata":map[string]interface {}{"resourceVersion":"1168", "creationTimestamp":"2016-12-02T23:26:39Z", "labels":map[string]interface {}{"app":"redis", "role":"master"}, "name":"redis-master", "namespace":"e2e-tests-kubectl-otsd4", "selfLink":"/api/v1/namespaces/e2e-tests-kubectl-otsd4/services/redis-master", "uid":"c6836226-b8e6-11e6-a09b-42010af00002"}, "spec":map[string]interface {}{"ports":[]interface {}{map[string]interface {}{"protocol":"TCP", "port":6379, "targetPort":"redis-server"}}, "selector":map[string]interface {}{"app":"redis", "role":"master"}, "clusterIP":"10.0.217.213", "type":"ClusterIP", "sessionAffinity":"None"}, "status":map[string]interface {}{"loadBalancer":map[string]interface {}{}}}
    
    
    stderr:
    error: error executing jsonpath "{.spec.ports[0].nodePort}": nodePort is not found
    
    error:
    exit status 1
    
not to have occurred
/go/src/k8s.io/kubernetes/_output/dockerized/go/src/k8s.io/kubernetes/test/e2e/framework/util.go:2207`,
			`/go/src/k8s.io/kubernetes/_output/dockerized/go/src/k8s.io/kubernetes/test/e2e/kubectl.go:521
Expected error:
    <exec.CodeExitError>: {
        Err: {
            s: "error running &{/workspace/kubernetes_skew/cluster/kubectl.sh [/workspace/kubernetes_skew/cluster/kubectl.sh --server=https://104.154.242.87 --kubeconfig=/workspace/.kube/config get service redis-master --namespace=e2e-tests-kubectl-qasmj -o jsonpath={.spec.ports[0].nodePort}] []  <nil> Error executing template: nodePort is not found. Printing more information for debugging the template:\n\ttemplate was:\n\t\t{.spec.ports[0].nodePort}\n\tobject given to jsonpath engine was:\n\t\tmap[string]interface {}{\"metadata\":map[string]interface {}{\"resourceVersion\":\"567\", \"creationTimestamp\":\"2016-12-02T23:31:31Z\", \"labels\":map[string]interface {}{\"app\":\"redis\", \"role\":\"master\"}, \"name\":\"redis-master\", \"namespace\":\"e2e-tests-kubectl-qasmj\", \"selfLink\":\"/api/v1/namespaces/e2e-tests-kubectl-qasmj/services/redis-master\", \"uid\":\"74993e39-b8e7-11e6-bda3-42010af0002a\"}, \"spec\":map[string]interface {}{\"ports\":[]interface {}{map[string]interface {}{\"protocol\":\"TCP\", \"port\":6379, \"targetPort\":\"redis-server\"}}, \"selector\":map[string]interface {}{\"role\":\"master\", \"app\":\"redis\"}, \"clusterIP\":\"10.19.251.75\", \"type\":\"ClusterIP\", \"sessionAffinity\":\"None\"}, \"status\":map[string]interface {}{\"loadBalancer\":map[string]interface {}{}}, \"kind\":\"Service\", \"apiVersion\":\"v1\"}\n\n error: error executing jsonpath \"{.spec.ports[0].nodePort}\": nodePort is not found\n [] <nil> 0xc820332540 exit status 1 <nil> true [0xc82016eec8 0xc82016eee0 0xc82016eef8] [0xc82016eec8 0xc82016eee0 0xc82016eef8] [0xc82016eed8 0xc82016eef0] [0xaf8950 0xaf8950] 0xc8205bd5c0}:\nCommand stdout:\nError executing template: nodePort is not found. Printing more information for debugging the template:\n\ttemplate was:\n\t\t{.spec.ports[0].nodePort}\n\tobject given to jsonpath engine was:\n\t\tmap[string]interface {}{\"metadata\":map[string]interface {}{\"resourceVersion\":\"567\", \"creationTimestamp\":\"2016-12-02T23:31:31Z\", \"labels\":map[string]interface {}{\"app\":\"redis\", \"role\":\"master\"}, \"name\":\"redis-master\", \"namespace\":\"e2e-tests-kubectl-qasmj\", \"selfLink\":\"/api/v1/namespaces/e2e-tests-kubectl-qasmj/services/redis-master\", \"uid\":\"74993e39-b8e7-11e6-bda3-42010af0002a\"}, \"spec\":map[string]interface {}{\"ports\":[]interface {}{map[string]interface {}{\"protocol\":\"TCP\", \"port\":6379, \"targetPort\":\"redis-server\"}}, \"selector\":map[string]interface {}{\"role\":\"master\", \"app\":\"redis\"}, \"clusterIP\":\"10.19.251.75\", \"type\":\"ClusterIP\", \"sessionAffinity\":\"None\"}, \"status\":map[string]interface {}{\"loadBalancer\":map[string]interface {}{}}, \"kind\":\"Service\", \"apiVersion\":\"v1\"}\n\n\nstderr:\nerror: error executing jsonpath \"{.spec.ports[0].nodePort}\": nodePort is not found\n\nerror:\nexit status 1\n",
        },
        Code: 1,
    }
    error running &{/workspace/kubernetes_skew/cluster/kubectl.sh [/workspace/kubernetes_skew/cluster/kubectl.sh --server=https://104.154.242.87 --kubeconfig=/workspace/.kube/config get service redis-master --namespace=e2e-tests-kubectl-qasmj -o jsonpath={.spec.ports[0].nodePort}] []  <nil> Error executing template: nodePort is not found. Printing more information for debugging the template:
    	template was:
    		{.spec.ports[0].nodePort}
    	object given to jsonpath engine was:
    		map[string]interface {}{"metadata":map[string]interface {}{"resourceVersion":"567", "creationTimestamp":"2016-12-02T23:31:31Z", "labels":map[string]interface {}{"app":"redis", "role":"master"}, "name":"redis-master", "namespace":"e2e-tests-kubectl-qasmj", "selfLink":"/api/v1/namespaces/e2e-tests-kubectl-qasmj/services/redis-master", "uid":"74993e39-b8e7-11e6-bda3-42010af0002a"}, "spec":map[string]interface {}{"ports":[]interface {}{map[string]interface {}{"protocol":"TCP", "port":6379, "targetPort":"redis-server"}}, "selector":map[string]interface {}{"role":"master", "app":"redis"}, "clusterIP":"10.19.251.75", "type":"ClusterIP", "sessionAffinity":"None"}, "status":map[string]interface {}{"loadBalancer":map[string]interface {}{}}, "kind":"Service", "apiVersion":"v1"}
    
     error: error executing jsonpath "{.spec.ports[0].nodePort}": nodePort is not found
     [] <nil> 0xc820332540 exit status 1 <nil> true [0xc82016eec8 0xc82016eee0 0xc82016eef8] [0xc82016eec8 0xc82016eee0 0xc82016eef8] [0xc82016eed8 0xc82016eef0] [0xaf8950 0xaf8950] 0xc8205bd5c0}:
    Command stdout:
    Error executing template: nodePort is not found. Printing more information for debugging the template:
    	template was:
    		{.spec.ports[0].nodePort}
    	object given to jsonpath engine was:
    		map[string]interface {}{"metadata":map[string]interface {}{"resourceVersion":"567", "creationTimestamp":"2016-12-02T23:31:31Z", "labels":map[string]interface {}{"app":"redis", "role":"master"}, "name":"redis-master", "namespace":"e2e-tests-kubectl-qasmj", "selfLink":"/api/v1/namespaces/e2e-tests-kubectl-qasmj/services/redis-master", "uid":"74993e39-b8e7-11e6-bda3-42010af0002a"}, "spec":map[string]interface {}{"ports":[]interface {}{map[string]interface {}{"protocol":"TCP", "port":6379, "targetPort":"redis-server"}}, "selector":map[string]interface {}{"role":"master", "app":"redis"}, "clusterIP":"10.19.251.75", "type":"ClusterIP", "sessionAffinity":"None"}, "status":map[string]interface {}{"loadBalancer":map[string]interface {}{}}, "kind":"Service", "apiVersion":"v1"}
    
    
    stderr:
    error: error executing jsonpath "{.spec.ports[0].nodePort}": nodePort is not found
    
    error:
    exit status 1
    
not to have occurred
/go/src/k8s.io/kubernetes/_output/dockerized/go/src/k8s.io/kubernetes/test/e2e/framework/util.go:2207`,
		},
		{
			false,
			`Error: 1 leaked resources
[ target-pools ]
+a5be3cc98c48511e6aa8b42010af0001  us-central1`,
			`Error: 1 leaked resources
[ target-pools ]
+a15e1aadcc48511e6bbbe42010af0003  us-central1`,
		},
		{
			true,
			`/go/src/k8s.io/kubernetes/_output/dockerized/go/src/k8s.io/kubernetes/test/e2e/reboot.go:118
Sep 29 08:42:26.724: Test failed; at least one node failed to reboot in the time given.
/go/src/k8s.io/kubernetes/_output/dockerized/go/src/k8s.io/kubernetes/test/e2e/reboot.go:158`,
			`/go/src/k8s.io/kubernetes/_output/dockerized/go/src/k8s.io/kubernetes/test/e2e/reboot.go:118
Sep 29 22:41:21.062: Test failed; at least one node failed to reboot in the time given.
/go/src/k8s.io/kubernetes/_output/dockerized/go/src/k8s.io/kubernetes/test/e2e/reboot.go:158`,
		},
	}
	for i, test := range testcases {
		result := flakeReasonsAreEquivalent(test.a, test.b)
		if result != test.expected {
			t.Errorf("test #%d: flakeReasonsAreEquivalent(`%s`, `%s`) unexpectedly %v", i, test.a, test.b, result)
		}
	}

}
