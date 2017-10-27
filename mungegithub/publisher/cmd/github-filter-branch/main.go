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
	"strings"

	"github.com/golang/glog"
	gogit "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"

	"k8s.io/test-infra/mungegithub/publisher/pkg/cache"
	"k8s.io/test-infra/mungegithub/publisher/pkg/git"
)

func Usage() {
	fmt.Fprintf(os.Stderr, `Filters the current branch by a subdirectory, but in contrast to
git-filter-branch it keeps all mainline merge commits for non-empty feature branches.
Empty feature branch commits are pruned if --prune-empty is given. To each commit message a
'Kubernetes-commit: <sha1>' line is appended.

Each non-remote ref without onto-ref is reset to the rewritten commit. Those refs with onto-ref
are expected to be mainline descendents of the given onto-ref after rewriting. The onto-ref
is then updated to point to the rewritten ref commit. If the onto-ref does not exist yet, it
is created.

Commits reachable from the onto-refs which have a Kubernetes-commit annotation in their commit
message are re-used as parents of newly created commits.

With --pass-through paths are inherited from first parent of each commit, e.g. vendor/ can be
inherited or a README.md. The paths are relative to <subdir> (without dot notation).

Usage: %s --subdirectory-filter <subdir> [--prune-empty] [--pass-through <path>,<path>,...] [<ref>]:[<onto-ref>] ...
`, os.Args[0])
	flag.PrintDefaults()
}

var (
	subdir                 = flag.String("subdirectory-filter", "", "the relative subdirectory without . or .. to filter by")
	pruneEmpty             = flag.Bool("prune-empty", false, "prune empty non-mainline and non-merge commits")
	dryRun                 = flag.Bool("dry-run", false, "do not update any ref")
	pathThroughPathsString = flag.String("pass-through", "", "inherit these tree paths from the first parent in each commit, overriding the tree-state inside the commits")
)

type Onto struct {
	Name string
	Ref  *plumbing.Reference // nil if this is a new branch
}

type Target struct {
	Ref  *plumbing.Reference
	Onto *Onto
}

func main() {
	flag.Lookup("logtostderr").Value.Set("true")
	flag.Usage = Usage
	flag.Parse()

	if *subdir == "" {
		glog.Fatalf("sub-directory cannot be empty")
	}

	pathThroughPaths := []string{}
	if *pathThroughPathsString != "" {
		for _, p := range strings.Split(*pathThroughPathsString, ",") {
			pathThroughPaths = append(pathThroughPaths, strings.Trim(p, "/"))
		}
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
	localBranch := h.Name().String()
	if localBranch == "" {
		glog.Fatalf("Failed to get current branch.")
	}

	// get refs
	args := flag.Args()
	if len(args) == 0 {
		args = []string{localBranch}
	}
	targets := []Target{}
	for _, arg := range args {
		target := Target{}

		comps := strings.SplitN(arg, ":", 2)
		if len(comps[0]) != 0 {
			ref, err := r.Reference(plumbing.ReferenceName(normalizeRef(comps[0])), true)
			if err != nil {
				glog.Fatalf("Failed resolve ref %s: %v", comps[0], err)
			}
			target.Ref = ref
		}

		if len(comps) > 1 && len(comps[1]) != 0 {
			name := normalizeRef(comps[1])
			target.Onto = &Onto{
				Name: name,
			}
			onto, err := r.Reference(plumbing.ReferenceName(name), true)
			if err == nil {
				target.Onto.Ref = onto
			} else {
				glog.Infof("Assuming %v is a new branch we will create after filtering.", comps[1])
			}
		}

		targets = append(targets, target)
	}

	// get mainlines
	kMainlineCommits := map[plumbing.Hash]bool{}
	for _, target := range targets {
		if target.Ref == nil {
			continue
		}
		kc, err := cache.CommitObject(r, target.Ref.Hash())
		if err != nil {
			glog.Fatalf("Failed to open ref %s head: %v", target.Ref.String(), err)
		}
		mainline, err := git.FirstParentList(r, kc)
		if err != nil {
			glog.Fatalf("Failed to get mainline of ref %s: %v", target.Ref.String(), err)
		}
		for _, kmc := range mainline {
			kMainlineCommits[kmc.Hash] = true
		}
	}

	// create map from kube commit to old filtered commits under one of the target onto-refs
	kube2filtered := map[plumbing.Hash]plumbing.Hash{}
	for _, target := range targets {
		if target.Onto == nil || target.Onto.Ref == nil {
			continue
		}
		err := git.CollectKubernetesCommits(r, kube2filtered, target.Onto.Ref.Hash())
		if err != nil {
			glog.Fatalf("Failed to collect all existing commits in ref %v: %v", target.Onto.Name, err)
		}
	}

	// do the filtering
	fNewRefHeads := map[string]plumbing.Hash{}
	for _, target := range targets {
		if target.Ref == nil {
			continue
		}

		if target.Onto == nil {
			glog.Infof("Filtering ref %s", target.Ref.String())
		} else {
			glog.Infof("Filtering ref %s onto %s", target.Ref.String(), target.Onto.Name)
		}
		ctx := FilterContext{
			repository:       r,
			kube2filtered:    kube2filtered,
			kMainlineCommits: kMainlineCommits,
			filterPath:       *subdir,
			passThroughPaths: pathThroughPaths,
		}
		fc, err := filter(&ctx, target.Ref.Hash())
		if err != nil {
			glog.Fatalf("Failed to filter ref %s: %v", target.Ref.String(), err)
		}
		kube2filtered[target.Ref.Hash()] = fc.Hash

		// verify that we found the onto-ref
		if target.Onto != nil && target.Onto.Ref != nil {
			fMainline, err := git.FirstParentList(r, fc)
			if err != nil {
				glog.Fatalf("Failed to get mainline of filtered ref %s: %v", target.Ref.Hash(), err)
			}
			found := false
			newMainlineCommits := 0
			for _, fmlc := range fMainline {
				if fmlc.Hash == target.Onto.Ref.Hash() {
					found = true
					break
				}
				newMainlineCommits++
			}
			if !found {
				glog.Fatalf("Failed to filter %v onto %v, didn't find %v's tip %v in %v tree",
					target.Ref.Name, target.Onto.Name, target.Onto.Name, target.Onto.Ref.Hash(), fc.Hash)
			}

			// print what we did
			glog.Infof("Found %d new mainline commits", newMainlineCommits)
			for _, fmlc := range fMainline[:newMainlineCommits] {
				glog.V(2).Infof("- %v %v", fmlc.Hash, strings.SplitN(fmlc.Message, "\n", 2)[0])
			}
		}

		if target.Onto == nil {
			fNewRefHeads[target.Ref.String()] = fc.Hash
		} else {
			fNewRefHeads[target.Onto.Name] = fc.Hash
		}
	}

	// update the branches. But do this after all filtering has succeeded.
	for _, target := range targets {
		if target.Ref == nil {
			continue
		}
		ref := target.Ref.String()
		if target.Onto != nil {
			ref = target.Onto.Name
		}
		hash, found := fNewRefHeads[ref]
		if !found {
			glog.Fatalf("Odd, no new hash found for ref %v", ref) // shouldn't happen
		}

		if strings.HasPrefix(ref, "refs/remotes/") {
			glog.Infof("Cowardly refusing to update remote ref %s to %v", ref, hash)
			continue
		}
		if *dryRun {
			glog.Infof("Pretending to update %v to %v", ref, hash)
			continue
		}

		glog.Infof("Updating %v to %v", ref, hash)
		if err := r.Storer.SetReference(plumbing.NewHashReference(plumbing.ReferenceName(ref), hash)); err != nil {
			glog.Fatalf("Failed to update ref %s to %v: %v", ref, hash, err)
		}
	}

	// checkout the local branch again. Avoid this if it is not necessary to avoid possible checkout conflicts.
	wt, err := r.Worktree()
	if err != nil {
		glog.Fatalf("Failed to get worktree: %v", err)
	}
	for _, target := range targets {
		ref := target.Ref.String()
		if target.Onto != nil {
			if target.Onto.Ref == nil {
				continue // new branch
			}
			ref = target.Onto.Ref.String()
		}
		if ref != localBranch {
			continue
		}
		err = wt.Checkout(&gogit.CheckoutOptions{
			Branch: plumbing.ReferenceName(localBranch),
		})
		if err != nil {
			glog.Fatalf("Failed to get checkout %q: %v", h.Name().Short(), err)
		}
	}
}

type FilterContext struct {
	repository       *gogit.Repository
	kube2filtered    map[plumbing.Hash]plumbing.Hash
	kMainlineCommits map[plumbing.Hash]bool
	passThroughPaths []string
	filterPath       string
}

func filter(ctx *FilterContext, khash plumbing.Hash) (*object.Commit, error) {
	if fhash, found := ctx.kube2filtered[khash]; found {
		if fhash == plumbing.ZeroHash {
			return nil, nil
		}
		return cache.CommitObject(ctx.repository, fhash)
	}

	c, err := cache.CommitObject(ctx.repository, khash)
	if err != nil {
		return nil, fmt.Errorf("failed to get hash %v: %v", khash, err)
	}

	// find subdir
	t, err := c.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to get tree %v of commit %v: %v", c.TreeHash, khash, err)
	}
	ft, err := t.Tree(ctx.filterPath)
	if err == object.ErrDirectoryNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get subtree %q of commit %v: %v", ctx.filterPath, khash, err)
	}

	// create filtered commit
	fc := object.Commit{
		Author:       c.Author,
		Committer:    c.Committer,
		Message:      fmt.Sprintf("%s\n\nKubernetes-commit: %v\n", strings.TrimRight(git.StripSignature(c.Message), "\n"), khash),
		ParentHashes: []plumbing.Hash{},
		TreeHash:     ft.Hash,
	}

	// prune empty commit? empty = first parent has the same tree. We do this
	// comparison on the unprocessed parent tree to avoid that our transormations
	// have any influence and lead to ghost commits.
	if *pruneEmpty && len(c.ParentHashes) > 0 {
		pc, err := cache.CommitObject(ctx.repository, c.ParentHashes[0])
		if err != nil {
			return nil, fmt.Errorf("failed to get commit %v: %v", c.ParentHashes[0], err)
		}
		pt, err := pc.Tree()
		if err != nil {
			return nil, fmt.Errorf("failed to get tree %v of commit %v: %v", pc.TreeHash, pc.Hash, err)
		}
		fpt, err := pt.Tree(ctx.filterPath)
		if err == nil && ft.Hash.String() == fpt.Hash.String() {
			fpc, err := filter(ctx, pc.Hash)
			if err != nil {
				return nil, err
			}
			ctx.kube2filtered[pc.Hash] = fpc.Hash
			return fpc, nil
		}
	}

	// map parents
	fpcs := []*object.Commit{}
	for _, phash := range c.ParentHashes {
		fpc, err := filter(ctx, phash)
		if err != nil {
			return nil, err
		}
		if fpc == nil {
			continue
		}
		fpcs = append(fpcs, fpc)
		ctx.kube2filtered[phash] = fpc.Hash
		fc.ParentHashes = append(fc.ParentHashes, fpc.Hash)
	}

	// passing through paths from first parent
	if len(fc.ParentHashes) > 0 {
		for _, p := range ctx.passThroughPaths {
			pt, err := ctx.repository.TreeObject(fpcs[0].TreeHash)
			if err != nil {
				return nil, fmt.Errorf("failed to get tree of %v: %v", fpcs[0].Hash, err)
			}
			entry, _ := pt.FindEntry(p)
			glog.V(6).Infof("Passing-through %v in filtered kubernetes commit %v to tree entry %v", p, khash, entry)
			newFt, err := git.SetTreeEntryAt(ctx.repository, ft, "", strings.Split(p, "/"), entry)
			if err != nil {
				return nil, fmt.Errorf("failed to pass through %v in tree %v: %v", p, ft.Hash, err)
			}
			ft = newFt
		}
		fc.TreeHash = ft.Hash
	}

	// create commit object
	obj := ctx.repository.Storer.NewEncodedObject()
	if err := fc.Encode(obj); err != nil {
		return nil, err
	}
	fc.Hash, err = ctx.repository.Storer.SetEncodedObject(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to store filtered commit for %v: %v", khash, err)
	}

	return &fc, nil
}

func normalizeRef(s string) string {
	if strings.HasPrefix(s, "refs/") {
		return s
	}
	if strings.ContainsRune(s, '/') {
		return "refs/remotes/" + s
	}
	return "refs/heads/" + s
}
