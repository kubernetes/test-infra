/*
Copyright 2017 The Kubernetes Authors.

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
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/golang/glog"
	gogit "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"

	"k8s.io/test-infra/mungegithub/publisher/pkg/cache"
	"k8s.io/test-infra/mungegithub/publisher/pkg/git"
)

func Usage() {
	fmt.Fprintf(os.Stderr, `Print a lookup table by printing each mainline k8s.io/kubernetes
commit hash with its corresponding commit hash in the current branch
(which is the result of a "git filter-branch --sub-directory"). It is
expected that the commit messages on the current branch contain a
"Kubernetes-commit: <upstream commit>" line for the directly corresonding
commit. Note, that a number of k8s.io/kubernetes mainline commits might
be collapsed during filtering:

    HEAD <upstream-branch>
     |          |
     H'<--------H
     z          |
     y         ,G
     F'<------*-F
     |        ,-E
     x       / ,D
     |      / / |
     C'<----**--C
     w          |
     v          |
                B
                A

The sorted output looks like this:

    <sha of E> <sha of C'>
    <sha of C> <sha of C'>
    <sha of G> <sha of F'>
    <sha of D> <sha of C'>
    <sha of F> <sha of F'>
    <sha of H> <sha of H'>
    ...

Usage: %s --upstream-branch <upstream-branch> [-l]
`, os.Args[0])
	flag.PrintDefaults()
}

func main() {
	upstreamBranch := flag.String("upstream-branch", "", "the k8s.io/kubernetes branch (fully qualified e.g. refs/remotes/origin/master) used as the filter-branch basis")
	showMessage := flag.Bool("l", false, "list the commit message after the two hashes")

	flag.Usage = Usage
	flag.Parse()

	if *upstreamBranch == "" {
		glog.Fatalf("upstream-branch cannot be empty")
	}

	// open repo at "."
	r, err := gogit.PlainOpen(".")
	if err != nil {
		glog.Fatalf("Failed to open repo at .: %v", err)
	}

	// get HEAD
	dstRef, err := r.Head()
	if err != nil {
		glog.Fatalf("Failed to open HEAD: %v", err)
	}
	dstHead, err := cache.CommitObject(r, dstRef.Hash())
	if err != nil {
		glog.Fatalf("Failed to resolve HEAD: %v", err)
	}

	// get first-parent commit list of upstream branch
	kUpstreamBranch, err := r.ResolveRevision(plumbing.Revision(*upstreamBranch))
	if err != nil {
		glog.Fatalf("Failed to open upstream branch %s: %v", *upstreamBranch, err)
	}
	kHead, err := cache.CommitObject(r, *kUpstreamBranch)
	if err != nil {
		glog.Fatalf("Failed to open upstream branch %s head: %v", *upstreamBranch, err)
	}
	kFirstParents, err := git.FirstParentList(r, kHead)
	if err != nil {
		glog.Fatalf("Failed to get upstream branch %s first-parent list: %v", *upstreamBranch, err)
	}

	// get first-parent commit list of HEAD
	dstFirstParents, err := git.FirstParentList(r, dstHead)
	if err != nil {
		glog.Fatalf("Failed to get first-parent commit list for %s: %v", dstHead.Hash, err)
	}

	kubeCommitsToDstCommits, err := git.KubeCommitsToDstCommits(r, dstFirstParents, kFirstParents)
	if err != nil {
		glog.Fatalf("Failed to map upstream branch %s to HEAD: %v", *upstreamBranch, err)
	}

	// print out a look-up table
	// <kube sha> <dst sha>
	var lines []string
	for kh, dh := range kubeCommitsToDstCommits {
		if *showMessage {
			c, err := cache.CommitObject(r, kh)
			if err != nil {
				// if this happen something above in the algorithm is broken
				glog.Fatalf("Failed to find k8s.io/kubernetes commit %s", kh)
			}
			lines = append(lines, fmt.Sprintf("%s %s %s", kh, dh, strings.SplitN(c.Message, "\n", 2)[0]))
		} else {
			lines = append(lines, fmt.Sprintf("%s %s", kh, dh))
		}
	}
	sort.Strings(lines) // sort to allow binary search
	for _, l := range lines {
		fmt.Println(l)
	}
}
