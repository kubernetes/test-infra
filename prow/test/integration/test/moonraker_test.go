/*
Copyright 2023 The Kubernetes Authors.

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

package integration

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/moonraker"
	"k8s.io/test-infra/prow/test/integration/internal/fakegitserver"
)

type fakeConfigAgent struct {
	sync.Mutex
	c *config.Config
}

func (fca *fakeConfigAgent) Config() *config.Config {
	fca.Lock()
	defer fca.Unlock()
	return fca.c
}

// TestMoonrakerBurst tests Moonraker by flooding it with a burst of 100
// requests at once. Each request will have the same base SHA, but a different
// head SHA (pretending to be a unique Pull Request). This way, the requests
// will avoid getting coalesced into the same LRU cache entry. This in turn will
// force Moonraker to fetch the same base SHA in parallel while constructing the
// ProwYAML value. We expect Moonraker to return successfully for all 100
// requests.
//
// The hard part here is constructing the 100 different head SHA values. We need
// to create 100 different PRs, each with its own unique head SHA. We do this
// dynamically with our own Git v2 client (this means we will write into our
// local filesystem). We send a shell script over to fakegitserver (FGS) to
// create the 100 different commit SHAs, but all using the same base SHA (this
// is how FGS lets clients seed test repos). We then clone the repo from FGS to
// our local disk to read the SHA values for these 100 fake PRs. Now that we
// know the repo location, the base SHA, and the head SHAs, we can construct the
// GetProwYAML() queries to Moonraker. We then just check that the return values
// for all 100 requests are the same (because the 100 PRs will not modify the
// inrepoconfig files).
//
// The reason why we do this as an integration test instead of a unit test is
// because we want to go over the network for fetching the SHA object from FGS,
// as opposed to "fetching" locally from disk.
func TestMoonrakerBurst(t *testing.T) {
	const createRepoWithPRs = `
echo hello > README.txt
git add README.txt
git commit -m "commit 1"
git checkout master

echo this-is-from-repo%s > README.txt
git add README.txt
git commit -m "uniquify"

mkdir .prow
cat <<EOF >.prow/presubmits.yaml
presubmits:
  - name: my-presubmit
    always_run: false
    decorate: true
    spec:
      containers:
      - image: localhost:5001/alpine
        command:
        - sh
        args:
        - -c
        - |
          set -eu
          echo "hello from my-presubmit"
          cat README.txt
EOF

git add .prow/presubmits.yaml
git commit -m "add inrepoconfig for my-presubmit"
baseSHA=$(git rev-parse HEAD)

# Create fake PRs. These are "Gerrit" style refs under refs/changes/*.
for num in $(seq 1 100); do
	git checkout -d ${baseSHA}
    # Modify the presubmit name to match the unique num.
	sed -i "s,my-presubmit,my-presubmit (ref refs/changes/00/123/${num})," .prow/presubmits.yaml

	git add .prow/presubmits.yaml

	git commit -m "PR${num}"
	git update-ref "refs/changes/00/123/${num}" HEAD
done

git checkout master
git reset --hard ${baseSHA}
`

	repoSetup := fakegitserver.RepoSetup{
		Name:      "moonraker-burst",
		Script:    createRepoWithPRs,
		Overwrite: true,
	}

	// Set up repos on FGS for just this test case.
	fgsClient := fakegitserver.NewClient("http://localhost/fakegitserver", 5*time.Second)
	err := fgsClient.SetupRepo(repoSetup)
	if err != nil {
		t.Fatalf("FGS repo setup failed: %v", err)
	}

	// Clone the repo out of FGS to our local disk to determine the base SHA of
	// master branch for moonraker-burst, as well as the 100 different
	// PR head SHAs. We want to figure out the base SHA (master branch HEAD SHA)
	// and head SHAs (SHAs of each of the 100 changes we created during
	// repoSetup above) programmatically, so that we don't have to do it
	// manually as part of writing this test (or making changes to it in the
	// future).
	cacheDirBase := "/var/tmp"

	trueVal := true
	var gitClientFactoryOpt = func(o *git.ClientFactoryOpts) {
		o.UseInsecureHTTP = &trueVal
		o.Host = "localhost"
		o.CacheDirBase = &cacheDirBase
	}

	gc, err := git.NewClientFactory(gitClientFactoryOpt)
	if err != nil {
		t.Fatal(err)
	}

	// repoClient points to our local copy of this repo. We will use it to
	// figure out the base SHA and head SHAs. Because we are not sharing objects
	// (ShareObjectsWithSourceRepo is false in repoOpts), the repoClient will be
	// a full mirror clone.
	repoClient, err := gc.ClientForWithRepoOpts("fakegitserver/repo", repoSetup.Name, git.RepoOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer repoClient.Clean()

	baseSHA, err := repoClient.RevParse("HEAD")
	if err != nil {
		t.Fatal(err)
	}
	baseSHA = strings.TrimSuffix(baseSHA, "\n")

	// Determine the SHAs for all changes in refs/changes/* (all 100 of them).
	// We'll then spawn 100 goroutines and make each one fetch one of these 100
	// SHAs. We have to fetch the refs from FGS, because the repoClient we are
	// using is for the secondary clone on disk and does not have a mirror clone
	// (it only has refs from the primary mirror).
	remoteResolver := func() (string, error) {
		return "http://localhost/fakegitserver/repo/moonraker-burst", nil
	}
	repoClient.FetchFromRemote(remoteResolver, "refs/changes/*:refs/changes/*")
	refs := []string{}
	for i := 1; i <= 100; i++ {
		ref := fmt.Sprintf("refs/changes/00/123/%d", i)
		refs = append(refs, ref)
	}
	refsToShas, err := repoClient.RevParseN(refs)
	if err != nil {
		t.Fatal(err)
	}

	// Set up client to talk to Moonraker inside the KIND cluster. The moonraker
	// address here uses localhost, because we're initiating the request from
	// outside the KIND cluster (this file you are reading is executed outside
	// the cluster).
	fca := &fakeConfigAgent{
		c: &config.Config{
			ProwConfig: config.ProwConfig{
				Moonraker: config.Moonraker{
					ClientTimeout: &metav1.Duration{Duration: 5 * time.Second},
				},
			},
		},
	}
	moonrakerClient, err := moonraker.NewClient("http://localhost/moonraker", fca)
	if err != nil {
		t.Fatal(err)
	}

	want := func(ref string) config.Presubmit {
		return config.Presubmit{
			JobBase: config.JobBase{
				Name: fmt.Sprintf("my-presubmit (ref %s)", ref),
				UtilityConfig: config.UtilityConfig{
					Decorate: &trueVal,
				},
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Image:   "localhost:5001/alpine",
							Command: []string{"sh"},
							Args: []string{
								"-c",
								fmt.Sprintf(`set -eu
echo "hello from my-presubmit (ref %s)"
cat README.txt
`, ref),
							},
						},
					},
				},
			},
		}
	}

	// Collect errors from the workers. This is because otherwise we get a
	// "call to (*T).Fatal from a non-test goroutine" error.
	errs := make(chan error, 1)

	var sendGetProwYAMLRequest = func(t *testing.T, ref, headSHA string) {
		got, err := moonrakerClient.GetProwYAML(&prowjobv1.Refs{
			// org is the URL of the "org" for the repo, as seen from inside the
			// KIND cluster (because moonraker is running inside the KIND
			// cluster). This is why we use the "fakegitserver.default" domain.
			Org:     "https://fakegitserver.default/repo",
			Repo:    "moonraker-burst",
			BaseSHA: baseSHA,
			Pulls:   []prowjobv1.Pull{{SHA: headSHA}},
		})
		if err != nil {
			errs <- err
			return
		}

		gotPresubmit := got.Presubmits[0]
		// Clear out parts of the response that we don't care about.
		gotPresubmit.Trigger = ""
		gotPresubmit.RerunCommand = ""
		gotPresubmit.Reporter = config.Reporter{}
		gotPresubmit.Brancher = config.Brancher{}
		gotPresubmit.JobBase.Agent = ""
		gotPresubmit.JobBase.Cluster = ""
		gotPresubmit.JobBase.Namespace = nil
		gotPresubmit.JobBase.ProwJobDefault = nil
		gotPresubmit.UtilityConfig.DecorationConfig = nil

		// Check that the gotPresubmit is what we expect (what we created in the
		// createRepoWithPRs bit at the beginning of this test function above).
		if diff := cmp.Diff(gotPresubmit, want(ref), cmpopts.IgnoreUnexported(
			config.Presubmit{},
			config.Brancher{},
			config.RegexpChangeMatcher{})); diff != "" {
			errs <- fmt.Errorf("unexpected moonraker response: %s", diff)
		} else {
			errs <- nil
		}
	}

	// Now create the burst of 100 requests (each one with its own unique
	// headSHA). It is at this point that moonraker will learn of the
	// moonraker-burst repo we've created in FGS.
	for ref, headSHA := range refsToShas {
		go sendGetProwYAMLRequest(t, ref, headSHA)
	}

	// Check that all 100 requests finished successfully.
	for range refsToShas {
		err := <-errs
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestMoonrakerUpdateBaseBranch checks that the base branch gets updated in the
// primary clone location. We create a repo on FGS with a regular base branch
// and PR HEAD that branches off of master. Then we do a ProwYAML lookup of the PR
// HEAD. After this request, we add a second commit on the master branch in FGS.
// Then we do a ProwYAML lookup of the same PR HEAD, but with an updated master
// branch SHA. Finally, we check that Moonraker has advanced the ref to the master
// branch.
func TestMoonrakerUpdateBaseBranch(t *testing.T) {
	const createRepoWithPR = `
echo hello > README.txt
git add README.txt
git commit -m "commit 1"
git checkout master

mkdir .prow
cat <<EOF >.prow/presubmits.yaml
presubmits:
  - name: my-presubmit
    always_run: false
    decorate: true
    spec:
      containers:
      - image: localhost:5001/alpine
        command:
        - sh
        args:
        - -c
        - |
          set -eu
          echo "hello from my-presubmit"
          cat README.txt
EOF

git add .prow/presubmits.yaml
git commit -m "add inrepoconfig for my-presubmit"
baseSHA=$(git rev-parse HEAD)

# Create another commit which the master branch can advance to. Note that this
# is a dangling commit because master branch does not point to it (yet). We
# can't make master branch point to it immediately because then this commit will
# get fetched alongside "commit 1" in the initial mirror clone in moonraker.
git checkout -d ${baseSHA}
echo world >> README.txt
git add README.txt
git commit -m "commit-2"
# Save the SHA to a stable file path so that we can read this out later in the test.
git rev-parse HEAD > .git/commit-2-SHA

# Create fake PRs. These are "Gerrit" style refs under refs/changes/*.
num=1
git checkout -d ${baseSHA}
# Modify the presubmit name to match the unique num.
sed -i "s,my-presubmit,my-presubmit (ref refs/changes/00/123/${num})," .prow/presubmits.yaml

git add .prow/presubmits.yaml

git commit -m "PR${num}"
git update-ref "refs/changes/00/123/${num}" HEAD

git checkout master
git reset --hard ${baseSHA}
`

	repoSetup := fakegitserver.RepoSetup{
		Name:      "moonraker-update-base-branch",
		Script:    createRepoWithPR,
		Overwrite: true,
	}

	// Set up repos on FGS for just this test case.
	fgsClient := fakegitserver.NewClient("http://localhost/fakegitserver", 5*time.Second)
	err := fgsClient.SetupRepo(repoSetup)
	if err != nil {
		t.Fatalf("FGS repo setup failed: %v", err)
	}

	// Clone the repo out of FGS to our local disk to determine the base SHA of
	// master branch for moonraker-update-base-branch, as well as the
	// PR head SHA. We want to figure out the base SHA (master branch HEAD SHA)
	// and head SHAs programmatically, so that we don't have to do it
	// manually as part of writing this test (or making changes to it in the
	// future).
	cacheDirBase := "/var/tmp"

	trueVal := true
	var gitClientFactoryOpt = func(o *git.ClientFactoryOpts) {
		o.UseInsecureHTTP = &trueVal
		o.Host = "localhost"
		o.CacheDirBase = &cacheDirBase
	}

	gc, err := git.NewClientFactory(gitClientFactoryOpt)
	if err != nil {
		t.Fatal(err)
	}

	// repoClient points to our local copy of this repo. We will use it to
	// figure out the base SHA and head SHAs. Because we are not sharing objects
	// (ShareObjectsWithSourceRepo is false in git.RepoOpts), the repoClient will be
	// a full mirror clone.
	repoClient, err := gc.ClientForWithRepoOpts("fakegitserver/repo", repoSetup.Name, git.RepoOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer repoClient.Clean()

	baseSHA, err := repoClient.RevParse("HEAD")
	if err != nil {
		t.Fatal(err)
	}
	baseSHA = strings.TrimSpace(baseSHA)

	// Determine the SHA for the PR head.
	// We'll then spawn 100 goroutines and make each one fetch one of these 100
	// SHAs. We have to fetch the refs from FGS, because the repoClient we are
	// using is for the secondary clone on disk and does not have a mirror clone
	// (it only has refs from the primary mirror).
	remoteResolver := func() (string, error) {
		return "http://localhost/fakegitserver/repo/moonraker-update-base-branch", nil
	}
	repoClient.FetchFromRemote(remoteResolver, "refs/changes/*:refs/changes/*")
	refs := []string{"refs/changes/00/123/1"}
	refsToShas, err := repoClient.RevParseN(refs)
	if err != nil {
		t.Fatal(err)
	}

	// Set up client to talk to Moonraker inside the KIND cluster. The moonraker
	// address here uses localhost, because we're initiating the request from
	// outside the KIND cluster (this file you are reading is executed outside
	// the cluster).
	fca := &fakeConfigAgent{
		c: &config.Config{
			ProwConfig: config.ProwConfig{
				Moonraker: config.Moonraker{
					ClientTimeout: &metav1.Duration{Duration: 5 * time.Second},
				},
			},
		},
	}
	moonrakerClient, err := moonraker.NewClient("http://localhost/moonraker", fca)
	if err != nil {
		t.Fatal(err)
	}

	want := func(ref string) config.Presubmit {
		return config.Presubmit{
			JobBase: config.JobBase{
				Name: fmt.Sprintf("my-presubmit (ref %s)", ref),
				UtilityConfig: config.UtilityConfig{
					Decorate: &trueVal,
				},
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Image:   "localhost:5001/alpine",
							Command: []string{"sh"},
							Args: []string{
								"-c",
								fmt.Sprintf(`set -eu
echo "hello from my-presubmit (ref %s)"
cat README.txt
`, ref),
							},
						},
					},
				},
			},
		}
	}

	// Collect errors from the workers. This is because otherwise we get a
	// "call to (*T).Fatal from a non-test goroutine" error.
	errs := make(chan error, 1)

	var sendGetProwYAMLRequest = func(t *testing.T, baseRef, baseSHA, prRef, headSHA string) {
		refs := &prowjobv1.Refs{
			// org is the URL of the "org" for the repo, as seen from inside the
			// KIND cluster (because moonraker is running inside the KIND
			// cluster). This is why we use the "fakegitserver.default" domain.
			Org:   "https://fakegitserver.default/repo",
			Repo:  "moonraker-update-base-branch",
			Pulls: []prowjobv1.Pull{{SHA: headSHA}},
		}
		if baseSHA != "" {
			refs.BaseSHA = baseSHA
		}
		if baseRef != "" {
			refs.BaseRef = baseRef
		}
		got, err := moonrakerClient.GetProwYAML(refs)
		if err != nil {
			errs <- err
			return
		}

		gotPresubmit := got.Presubmits[0]
		// Clear out parts of the response that we don't care about.
		gotPresubmit.Trigger = ""
		gotPresubmit.RerunCommand = ""
		gotPresubmit.Reporter = config.Reporter{}
		gotPresubmit.Brancher = config.Brancher{}
		gotPresubmit.JobBase.Agent = ""
		gotPresubmit.JobBase.Cluster = ""
		gotPresubmit.JobBase.Namespace = nil
		gotPresubmit.JobBase.ProwJobDefault = nil
		gotPresubmit.UtilityConfig.DecorationConfig = nil

		// Check that the gotPresubmit is what we expect (what we created in the
		// createRepoWithPR bit at the beginning of this test function above).
		if diff := cmp.Diff(gotPresubmit, want(prRef), cmpopts.IgnoreUnexported(
			config.Presubmit{},
			config.Brancher{},
			config.RegexpChangeMatcher{})); diff != "" {
			errs <- fmt.Errorf("unexpected moonraker response: %s", diff)
		} else {
			errs <- nil
		}
	}

	// Now make moonraker do lookups of the ProwYAML in the PR we created.
	// It is at this point that moonraker will learn of the
	// moonraker-update-base-branch repo we've created in FGS.
	for prRef, headSHA := range refsToShas {
		go sendGetProwYAMLRequest(t, "", baseSHA, prRef, headSHA)
	}

	// Check that all requests finished successfully.
	for range refsToShas {
		err := <-errs
		if err != nil {
			t.Fatal(err)
		}
	}

	// Get commit-2-SHA from FGS. Basically runs "kubectl exec" against the FGS
	// pod, but in Golang.
	clusterContext := getClusterContext()
	t.Logf("Creating client for cluster: %s", clusterContext)

	restConfig, err := NewRestConfig("", clusterContext)
	if err != nil {
		t.Fatalf("could not create restConfig: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		t.Fatalf("could not create Clientset: %v", err)
	}

	fgsPods, err := clientset.CoreV1().Pods("default").List(context.TODO(),
		metav1.ListOptions{LabelSelector: "app=fakegitserver"})
	if err != nil {
		t.Fatalf("could not list FGS pod: %v", err)
	}
	fgsPod := fgsPods.Items[0]
	commit2SHA, _, _ := execRemoteCommand(restConfig, clientset, &fgsPod, []string{"cat", "/git-repo/moonraker-update-base-branch.git/commit-2-SHA"})
	commit2SHA = strings.TrimSpace(commit2SHA)

	// Confirm that moonraker's primary clone of moonraker-update-base-branch
	// has a "master" branch that does *NOT* yet point to commit2SHA.
	moonrakerPods, err := clientset.CoreV1().Pods("default").List(context.TODO(),
		metav1.ListOptions{LabelSelector: "app=moonraker"})
	if err != nil {
		t.Fatalf("could not list Moonraker pod: %v", err)
	}
	moonrakerPod := moonrakerPods.Items[0]
	moonrakerMasterBranchSHA, _, _ := execRemoteCommand(restConfig, clientset, &moonrakerPod,
		[]string{"git",
			"-C",
			"/etc/moonraker-inrepoconfig-pv/https:/fakegitserver.default/repo/moonraker-update-base-branch",
			"rev-parse",
			"master",
		})
	moonrakerMasterBranchSHA = strings.TrimSpace(moonrakerMasterBranchSHA)
	if moonrakerMasterBranchSHA == commit2SHA {
		t.Fatal("programmer error: moonraker master branch is already pointing to commit-2's SHA")
	}

	// Execute another sendGetProwYAMLRequest(), but this time populate the base
	// branch name "master" instead of the empty string as before.
	for prRef, headSHA := range refsToShas {
		go sendGetProwYAMLRequest(t, "master", commit2SHA, prRef, headSHA)
	}
	for range refsToShas {
		err := <-errs
		if err != nil {
			t.Fatal(err)
		}
	}

	// Check that moonraker's primary clone's "master" ref is now pointing to
	// commit2SHA.
	moonrakerMasterBranchSHA, _, _ = execRemoteCommand(restConfig, clientset, &moonrakerPod,
		[]string{"git",
			"-C",
			"/etc/moonraker-inrepoconfig-pv/https:/fakegitserver.default/repo/moonraker-update-base-branch",
			"rev-parse",
			"master",
		})
	moonrakerMasterBranchSHA = strings.TrimSpace(moonrakerMasterBranchSHA)

	if diff := cmp.Diff(moonrakerMasterBranchSHA, commit2SHA); diff != "" {
		t.Fatalf("master branch on moonraker is not pointing to the commit2SHA %s: %s", commit2SHA, diff)
	}
}
