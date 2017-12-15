/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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

package features

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"k8s.io/kubernetes/pkg/util/sets"
	"k8s.io/kubernetes/pkg/util/yaml"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

const (
	ownerFilename = "OWNERS" // file which contains owners and assignees
	// RepoFeatureName is how mungers should indicate this is required
	RepoFeatureName = "gitrepos"
)

type assignmentConfig struct {
	Assignees []string `json:assignees yaml:assignees`
	//Owners []string `json:owners`
}

// RepoInfo provides information about users in OWNERS files in a git repo
type RepoInfo struct {
	enabled       bool
	kubernetesDir string
	assignees     map[string]sets.String
	//owners     map[string]sets.String
}

func init() {
	RegisterFeature(&RepoInfo{})
}

// Name is just going to return the name mungers use to request this feature
func (o *RepoInfo) Name() string {
	return RepoFeatureName
}

func (o *RepoInfo) walkFunc(path string, info os.FileInfo, err error) error {
	if err != nil {
		glog.Errorf("%v", err)
		return nil
	}
	filename := filepath.Base(path)
	if info.Mode().IsDir() {
		switch filename {
		case ".git":
			return filepath.SkipDir
		case "_output":
			return filepath.SkipDir
		}
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	if filename != ownerFilename {
		return nil
	}

	file, err := os.Open(path)
	if err != nil {
		glog.Errorf("%v", err)
		return nil
	}
	defer file.Close()

	c := &assignmentConfig{}
	if err := yaml.NewYAMLToJSONDecoder(file).Decode(c); err != nil {
		glog.Errorf("%v", err)
		return nil
	}

	path, err = filepath.Rel(o.kubernetesDir, path)
	if err != nil {
		glog.Errorf("Unable to find relative path between %q and %q: %v", o.kubernetesDir, path, err)
		return err
	}
	path = filepath.Dir(path)
	if len(c.Assignees) > 0 {
		o.assignees[path] = sets.NewString(c.Assignees...)
	}
	//if len(c.Owners) > 0 {
	//o.owners[path] = sets.NewString(c.Owners...)
	//}
	return nil
}

func (o *RepoInfo) updateRepoUsers() error {
	out, err := o.GitCommand([]string{"pull"})
	if err != nil {
		glog.Errorf("Unable to run git pull:\n%s\n%v", string(out), err)
		return err
	}

	out, err = o.GitCommand([]string{"rev-parse", "HEAD"})
	if err != nil {
		glog.Errorf("Unable get sha of HEAD:\n%s\n%v", string(out), err)
		return err
	}
	sha := out

	o.assignees = map[string]sets.String{}
	//o.owners = map[string]sets.String{}
	err = filepath.Walk(o.kubernetesDir, o.walkFunc)
	if err != nil {
		glog.Errorf("Got error %v", err)
	}
	glog.Infof("Loaded config from %s:%s", o.kubernetesDir, sha)
	glog.V(5).Infof("assignees: %v", o.assignees)
	//glog.V(5).Infof("owners: %v", o.owners)
	return nil
}

// Initialize will initialize the munger
func (o *RepoInfo) Initialize() error {
	o.enabled = true

	if len(o.kubernetesDir) == 0 {
		glog.Fatalf("--kubernetes-dir is required with selected munger(s)")
	}

	finfo, err := os.Stat(o.kubernetesDir)
	if err != nil {
		return fmt.Errorf("Unable to stat --kubernetes-dir: %v", err)
	}
	if !finfo.IsDir() {
		return fmt.Errorf("--kubernetes-dir is not a git directory")
	}
	return o.updateRepoUsers()
}

// EachLoop is called at the start of every munge loop
func (o *RepoInfo) EachLoop() error {
	if !o.enabled {
		return nil
	}

	_, err := o.GitCommand([]string{"remote", "update"})
	if err != nil {
		glog.Errorf("Unable to git remote update: %v", err)
	}

	return o.updateRepoUsers()
}

// AddFlags will add any request flags to the cobra `cmd`
func (o *RepoInfo) AddFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&o.kubernetesDir, "kubernetes-dir", "./gitrepos/kubernetes", "Path to git checkout of kubernetes tree")
}

// GitCommand will execute the git command with the `args`
func (o *RepoInfo) GitCommand(args []string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = o.kubernetesDir
	return cmd.CombinedOutput()
}

func peopleForPath(path string, people map[string]sets.String, leafOnly bool) sets.String {
	d := filepath.Dir(path)
	out := sets.NewString()
	for {
		s, ok := people[d]
		if ok {
			out = out.Union(s)
			if leafOnly {
				break
			}
		}
		if d == "" {
			break
		}
		d, _ = filepath.Split(d)
		d = strings.TrimSuffix(d, "/")
	}
	return out
}

// LeafAssignees returns a set of users who are the closest assginees to the
// requested file. If pkg/OWNERS has user1 and pkg/util/OWNERS has user2 this
// will only return user2 for the path pkg/util/sets/file.go
func (o *RepoInfo) LeafAssignees(path string) sets.String {
	return peopleForPath(path, o.assignees, true)
}

// Assignees returns a set of users who are the closest assginees to the
// requested file. If pkg/OWNERS has user1 and pkg/util/OWNERS has user2 this
// will return both user1 and user2 for the path pkg/util/sets/file.go
func (o *RepoInfo) Assignees(path string) sets.String {
	return peopleForPath(path, o.assignees, false)
}

//func (o *RepoInfo) LeafOwners(path string) sets.String {
//return people(path, o.owners, true)
//}

//func (o *RepoInfo) Owners(path string) sets.String {
//return people(path, o.owners, false)
//}
