/*
Copyright 2018 The Kubernetes Authors.

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
	"context"
	"encoding/json"
	// "encoding/xml"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"time"

	"k8s.io/test-infra/testgrid/state"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

type Build struct {
	Bucket  *storage.BucketHandle
	Context context.Context
	Prefix  string
}

type Started struct {
	Timestamp   int64             `json:"timestamp"` // python time.time()
	RepoVersion string            `json:"repo-version"`
	Node        string            `json:"node"`
	Pull        string            `json:"pull"`
	Repos       map[string]string `json:"repos"` // {repo: branch_or_pull} map
}

type Finished struct {
	Timestamp  int      `json:"timestamp"` // python time.time()
	Passed     bool     `json:"passed"`
	JobVersion string   `json:"job-version"`
	Metadata   Metadata `json:"metadata"`
}

type Metadata struct {
	Repo        string `json:"repo"` // First repo
	RepoCommit  string `json:"repo-commit"`
	InfraCommit string `json:"infra-commit"`
}

type BuildResult struct {
	Started  int
	Finished int
	Passed   bool
	Results  map[string]state.Row_Result
}

func ReadBuild(build Build) (*BuildResult, error) {
	br := BuildResult{}
	s := build.Bucket.Object(build.Prefix + "started.json")
	sr, err := s.NewReader(build.Context)
	if err != nil {
		return nil, fmt.Errorf("build has not started")
	}
	var started Started
	if err = json.NewDecoder(sr).Decode(&started); err != nil {
		return nil, fmt.Errorf("could not decode started.json: %v", err)
	}
	br.Started = started.Timestamp

	f := build.Bucket.Object(build.Prefix + "finished.json")
	fr, err := f.NewReader(build.Context)
	if err == storage.ErrObjectNotExist {
		return &br, nil
	}

	var finished Finished
	if err = json.NewDecoder(fr).Decode(&finished); err != nil {
		return nil, fmt.Errorf("could not decode finished.json: %v", err)
	}

	br.Finished = finished.Timestamp
	br.Passed = finished.Passed

	re := regexp.MustCompile(`.+/junit_.+\.xml$`)

	ai := build.Bucket.Objects(build.Context, &storage.Query{Prefix: build.Prefix + "artifacts/"})
	var artifacts []string
	for {
		a, err := ai.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list artifacts: %v", err)
		}

		if !re.MatchString(a.Name) {
			continue
		}
		artifacts = append(artifacts, a.Name)
	}
	return &br, nil
}

func ListBuilds(client *storage.Client, ctx context.Context, path string, builds chan Build) error {
	u, err := url.Parse(path)
	if err != nil {
		return fmt.Errorf("could not parse %s: %v", path, err)
	}
	if u.Scheme != "gs" {
		return fmt.Errorf("only gs:// paths supported: %s", path)
	}
	if len(u.Host) == 0 {
		return fmt.Errorf("empty host: %s", path)
	}
	if len(u.Path) < 2 {
		return fmt.Errorf("empty path: %s", path)
	}
	b := u.Host
	p := u.Path[1:]
	if p[len(p)-1] != '/' {
		p += "/"
	}
	bkt := client.Bucket(b)
	it := bkt.Objects(ctx, &storage.Query{
		Delimiter: "/",
		Prefix:    p,
	})
	fmt.Println("Looking in ", u, b, p)
	for {
		objAttrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to list objects: %v", err)
		}
		if len(objAttrs.Prefix) == 0 {
			continue
		}

		fmt.Println("Found name:", objAttrs.Name, "prefix:", objAttrs.Prefix, objAttrs.Updated)
		builds <- Build{
			Bucket:  bkt,
			Context: ctx,
			Prefix:  objAttrs.Prefix,
		}
	}
	return nil
}

func ReadBuilds(builds chan Build) {
	log.Println("Reading builds...")
	for b := range builds {
		br, err := ReadBuild(b)
		if err != nil {
			log.Printf("FAIL %s: %v", b.Prefix, err)
			continue
		}
		log.Printf("found! %v", br)
	}
	log.Println("Finished reading builds.")
}

func main() {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("Failed to create storage client: %v", err)
	}
	bkt := client.Bucket("kubernetes-jenkins")
	attrs, err := bkt.Attrs(ctx)
	if err != nil {
		log.Fatalf("Failed to access bucket: %v", err)
	}
	fmt.Printf("bucket %s, attrs %v", bkt, attrs)
	g := state.Grid{}
	g.Columns = append(g.Columns, &state.Column{Build: "first", Started: 1})
	fmt.Println(g)
	builds := make(chan Build)
	go ReadBuilds(builds)
	if err = ListBuilds(client, ctx, "gs://kubernetes-jenkins/pr-logs/pull-ingress-gce-e2e", builds); err != nil {
		log.Fatalf("Failed to list builds: %v", err)
	}
	close(builds)
	time.Sleep(10 * time.Second)
}
