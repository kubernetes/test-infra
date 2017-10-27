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
	"os/exec"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/renstrom/dedent"
	gogit "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"

	"k8s.io/test-infra/mungegithub/publisher/pkg/cache"
	"k8s.io/test-infra/mungegithub/publisher/pkg/git"
)

func Usage() {
	fmt.Fprintf(os.Stderr, `Syncs tags between the upstream remote branch and the local checkout
of an origin branch. Tags which do not exist in origin, but in upstream
are prepended with the given prefix and then created locally to be pushed
to origin (not done by this tool).

Tags from the upstream remote are fetched as "refs/tags/<upstream-remote>/<tag-name>".

Usage: %s --upstream-remote <remote> --upstream-branch <upstream-branch>
          [--origin-branch <branch>]
          [--prefix <tag-prefix>]
          [--push-script <file-path>]
`, os.Args[0])
	flag.PrintDefaults()
}

const rfc2822 = "Mon Jan 02 15:04:05 -0700 2006"

func main() {
	upstreamRemote := flag.String("upstream-remote", "", "the k8s.io/kubernetes remote")
	upstreamBranch := flag.String("upstream-branch", "", "the k8s.io/kubernetes branch (not qualified, just the name; defaults to equal <branch>)")
	publishBranch := flag.String("branch", "", "a (not qualified) branch name")
	prefix := flag.String("prefix", "kubernetes-", "a string to put in front of upstream tags")
	pushScriptPath := flag.String("push-script", "", "git-push command(s) are appended to this file to push the new tags to the origin remote")
	flag.Usage = Usage
	flag.Parse()

	if *upstreamRemote == "" {
		glog.Fatalf("upstream-remote cannot be empty")
	}

	if *upstreamBranch == "" {
		glog.Fatalf("branch cannot be empty")
	}

	// open repo at "."
	r, err := gogit.PlainOpen(".")
	if err != nil {
		glog.Fatalf("Failed to open repo at .: %v", err)
	}

	h, err := r.Head()
	if err != nil {
		glog.Fatalf("Failed to get HEAD: %v", err)
	}
	localBranch := h.Name().Short()
	if localBranch == "" {
		glog.Fatalf("Failed to get current branch.")
	}

	if *publishBranch == "" {
		*publishBranch = localBranch
	}

	// get first-parent commit list of local branch
	bRevision, err := r.ResolveRevision(plumbing.Revision(fmt.Sprintf("refs/heads/%s", localBranch)))
	if err != nil {
		glog.Fatalf("Failed to open branch %s: %v", localBranch, err)
	}
	bHead, err := cache.CommitObject(r, *bRevision)
	if err != nil {
		glog.Fatalf("Failed to open branch %s head: %v", localBranch, err)
	}
	bFirstParents, err := git.FirstParentList(r, bHead)
	if err != nil {
		glog.Fatalf("Failed to get branch %s first-parent list: %v", localBranch, err)
	}

	// get first-parent commit list of upstream branch
	kUpdateBranch, err := r.ResolveRevision(plumbing.Revision(fmt.Sprintf("refs/remotes/%s/%s", *upstreamRemote, *upstreamBranch)))
	if err != nil {
		glog.Fatalf("Failed to open upstream branch %s: %v", *upstreamBranch, err)
	}
	kHead, err := cache.CommitObject(r, *kUpdateBranch)
	if err != nil {
		glog.Fatalf("Failed to open upstream branch %s head: %v", *upstreamBranch, err)
	}
	kFirstParents, err := git.FirstParentList(r, kHead)
	if err != nil {
		glog.Fatalf("Failed to get upstream branch %s first-parent list: %v", *upstreamBranch, err)
	}

	// delete annotated remote tags locally
	fmt.Printf("Removing all local copies of origin and %s tags.\n", *upstreamRemote)
	if err := removeRemoteTags(r, []string{"origin", *upstreamRemote}); err != nil {
		glog.Fatalf("Failed to iterate through tags: %v", err)
	}

	// fetch tags
	fmt.Printf("Fetching tags from remote %q.\n", "origin")
	err = fetchTags(r, "origin")
	if err != nil {
		glog.Fatalf("Failed to fetch tags for %q: %v", "origin", err)
	}
	fmt.Printf("Fetching tags from remote %q.\n", *upstreamRemote)
	err = fetchTags(r, *upstreamRemote)
	if err != nil {
		glog.Fatalf("Failed to fetch tags for %q: %v", *upstreamRemote, err)
	}

	// get all annotated tags
	fmt.Printf("Resolving all tags.\n")
	bTagCommits, err := remoteTags(r, "origin")
	if err != nil {
		glog.Fatalf("Failed to iterate through tags: %v", err)
	}
	kTagCommits, err := remoteTags(r, *upstreamRemote)
	if err != nil {
		glog.Fatalf("Failed to iterate through tags: %v", err)
	}

	// compute kube commit map
	fmt.Printf("Computing mapping from kube commits to the local branch.\n")
	kubeCommitsToDstCommits, err := git.KubeCommitsToDstCommits(r, bFirstParents, kFirstParents)
	if err != nil {
		glog.Fatalf("Failed to map upstream branch %s to HEAD: %v", *upstreamBranch, err)
	}

	// create or update tags from kTagCommits as local tags with the given prefix
	createdTags := []string{}
	for name, kh := range kTagCommits {
		bName := name
		if *prefix != "" {
			bName = *prefix + name[1:] // remove the v
		}

		// skip if it already exists in origin
		if _, found := bTagCommits[bName]; found {
			fmt.Printf("Ignoring existing tag origin/%s\n", bName)
			continue
		}

		// ignore non-annotated tags
		tag, err := r.TagObject(kh)
		if err != nil {
			continue
		}

		// ignore old tags
		if tag.Tagger.When.Before(time.Date(2017, 9, 1, 0, 0, 0, 0, time.UTC)) {
			//fmt.Printf("Ignoring old tag origin/%s from %v\n", bName, tag.Tagger.When)
			continue
		}

		// map kube commit to local branch
		bh, found := kubeCommitsToDstCommits[tag.Target]
		if !found {
			continue
		}

		// do not override tags (we build master first, i.e. the x.y.z-alpha.0 tag on master will not be created for feature branches)
		if tagExists(r, bName) {
			fmt.Printf("Skipping existing tag %q.\n", bName)
			continue
		}

		// create prefixed annotated tag
		fmt.Printf("Tagging %v as %q.\n", bh, bName)
		err = createAnnotatedTag(bh, bName, tag.Tagger.When, dedent.Dedent(fmt.Sprintf(`
			Kubernetes release %s

			Based on https://github.com/kubernetes/kubernetes/releases/tag/%s
			`, name, name)))
		if err != nil {
			glog.Fatalf("Failed to create tag %q: %v", bName, err)
		}
		createdTags = append(createdTags, bName)
	}

	// write push command for new tags
	if *pushScriptPath != "" && len(createdTags) > 0 {
		pushScript, err := os.OpenFile(*pushScriptPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0755)
		if err != nil {
			glog.Fatalf("Failed to open push-script %q for appending: %v", *pushScriptPath, err)
		}
		defer pushScript.Close()
		_, err = pushScript.WriteString(fmt.Sprintf("git push origin %s\n", "refs/tags/"+strings.Join(createdTags, " refs/tags/")))
		if err != nil {
			glog.Fatalf("Failed to write to push-script %q: %q", *pushScriptPath, err)
		}
	}
}

func remoteTags(r *gogit.Repository, remote string) (map[string]plumbing.Hash, error) {
	refs, err := r.Storer.IterReferences()
	if err != nil {
		glog.Fatalf("Failed to get tags: %v", err)
	}
	defer refs.Close()
	tagCommits := map[string]plumbing.Hash{}
	err = refs.ForEach(func(ref *plumbing.Reference) error {
		if !ref.IsTag() {
			return nil
		}
		n := ref.Name().String()
		if prefix := "refs/tags/" + remote + "/"; strings.HasPrefix(n, prefix) {
			tagCommits[n[len(prefix):]] = ref.Hash()
		}
		return nil
	})
	return tagCommits, err
}

func removeRemoteTags(r *gogit.Repository, remotes []string) error {
	refs, err := r.Storer.IterReferences()
	if err != nil {
		glog.Fatalf("Failed to get tags: %v", err)
	}
	defer refs.Close()
	return refs.ForEach(func(ref *plumbing.Reference) error {
		if !ref.IsTag() {
			return nil
		}
		n := ref.Name().String()
		for _, remote := range remotes {
			if strings.HasPrefix(n, "refs/tags/"+remote+"/") {
				r.Storer.RemoveReference(ref.Name())
				break
			}
		}
		return nil
	})
}

func createAnnotatedTag(h plumbing.Hash, name string, date time.Time, message string) error {
	cmd := exec.Command("git", "tag", "-a", "-m", message, name, h.String())
	cmd.Env = append(cmd.Env, fmt.Sprintf("GIT_COMMITTER_DATE=%s", date.Format(rfc2822)))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func tagExists(r *gogit.Repository, tag string) bool {
	cmd := exec.Command("git", "show-ref", fmt.Sprintf("refs/tags/%s", tag))
	return cmd.Run() == nil

	// the following does not work with go-git, for unknown reasons:
	//_, err := r.ResolveRevision(plumbing.Revision(fmt.Sprintf("refs/tags/%s", tag)))
	//return err == nil
}

func fetchTags(r *gogit.Repository, remote string) error {
	cmd := exec.Command("git", "fetch", "-q", "--no-tags", remote, fmt.Sprintf("+refs/tags/*:refs/tags/%s/*", remote))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()

	// the following with go-git does not work (yet) due to missing support for * in refspecs:
	/*
		err := r.Fetch(&gogit.FetchOptions{
			RemoteName: remote,
			RefSpecs:   []config.RefSpec{"+refs/heads/*:refs/remotes/origin/*"},
			Progress:   sideband.Progress(os.Stderr),
			Tags:       gogit.TagFollowing,
		})
		if err == gogit.NoErrAlreadyUpToDate {
			return nil
		}
		return err
	*/
}
