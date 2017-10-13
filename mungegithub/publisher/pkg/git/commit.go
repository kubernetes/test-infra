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

package git

import (
	"fmt"
	"strings"

	"github.com/golang/glog"
	gogit "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"

	"k8s.io/test-infra/mungegithub/publisher/pkg/cache"
)

// collectKubernetesCommits does a depth first search to collect all commit with Kubernetes-commit annotation. It
// stores the mapping from kube-commit to corresponding filtered-commit in kube2filtered.
//
// Note: it does not do any pruning reversal logic, i.e. pruned commit won't be stored in kube2filtered.
func CollectKubernetesCommits(r *gogit.Repository, kube2filtered map[plumbing.Hash]plumbing.Hash, fhash plumbing.Hash) error {
	fc, err := cache.CommitObject(r, fhash)
	if err != nil {
		return fmt.Errorf("failed to find %v: %v", fhash, err)
	}
	khash := KubeHash(fc)
	if khash == plumbing.ZeroHash {
		return nil
	}
	if oldFhash, found := kube2filtered[khash]; found {
		if oldFhash.String() != fhash.String() {
			glog.Warningf("Conflicting commits for Kubernetes commit %v found: %v, %v", khash, oldFhash, fhash)
			// error out here?
		}
		return nil
	}

	kube2filtered[khash] = fhash

	// depth first recursion
	for _, fphash := range fc.ParentHashes {
		err := CollectKubernetesCommits(r, kube2filtered, fphash)
		if err != nil {
			return err
		}
	}

	return nil
}

// StripSignature strips a signature like
//
//----
// gpgsig -----BEGIN PGP SIGNATURE-----
// Version: GnuPG v1
//
// iQEcBAABAgAGBQJXYRjRAAoJEGEJLoW3InGJ3IwIAIY4SA6GxY3BjL60YyvsJPh/
// HRCJwH+w7wt3Yc/9/bW2F+gF72kdHOOs2jfv+OZhq0q4OAN6fvVSczISY/82LpS7
// DVdMQj2/YcHDT4xrDNBnXnviDO9G7am/9OE77kEbXrp7QPxvhjkicHNwy2rEflAA
// zn075rtEERDHr8nRYiDh8eVrefSO7D+bdQ7gv+7GsYMsd2auJWi1dHOSfTr9HIF4
// HJhWXT9d2f8W+diRYXGh4X0wYiGg6na/soXc+vdtDYBzIxanRqjg8jCAeo1eOTk1
// EdTwhcTZlI0x5pvJ3H0+4hA2jtldVtmPM4OTB0cTrEWBad7XV6YgiyuII73Ve3I=
// =jKHM
// -----END PGP SIGNATURE-----
//
// signed commit
//
// signed commit message body
// ----
//
func StripSignature(s string) string {
	// we get a signature string, not the header. Maybe go-git is broken?
	if !strings.HasPrefix(s, " ") && !strings.HasPrefix(s, "gpgsig -----BEGIN PGP SIGNATURE-----") {
		return s
	}

	lines := strings.Split(s, "\n")
	for i := range lines {
		if strings.Contains(lines[i], "-----END PGP SIGNATURE-----") {
			return strings.Join(lines[i+2:], "\n")
		}
	}

	return s
}
