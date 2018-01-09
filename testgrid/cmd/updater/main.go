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
	"encoding/xml"
	"fmt"
	"io/ioutil"
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
	Timestamp  int64    `json:"timestamp"` // python time.time()
	Passed     bool     `json:"passed"`
	JobVersion string   `json:"job-version"`
	Metadata   Metadata `json:"metadata"`
}

type Metadata struct {
	Repo        string `json:"repo"` // First repo
	RepoCommit  string `json:"repo-commit"`
	InfraCommit string `json:"infra-commit"`
}

type JunitSuite struct {
	XMLName  xml.Name      `xml:"testsuite"`
	Time     float64       `xml:"time,attr"`
	Failures int           `xml:"failures,attr"`
	Tests    int           `xml:"tests,attr"`
	Results  []JunitResult `xml:"testcase"`
}

type JunitResult struct {
	Name      string  `xml:"name,attr"`
	Time      float64 `xml:"time,attr"`
	ClassName string  `xml:"classname,attr"`
	Failure   *string `xml:"failure"`
	Output    *string `xml:"system-out"`
	Skipped   *string `xml:"skipped"`
}

type BuildResult struct {
	Started  int64
	Finished int64
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
	for _, ap := range artifacts {
		ar, err := build.Bucket.Object(ap).NewReader(build.Context)
		if err != nil {
			return nil, fmt.Errorf("could not read %s: %v", ap, err)
		}
		if r := ar.Remain(); r > 50e6 {
			return nil, fmt.Errorf("too large: %s is %d > 50M", ap, r)
		}
		buf, err := ioutil.ReadAll(ar)
		if err != nil {
			return nil, fmt.Errorf("failed to read all of %s: %v", ap, err)
		}

		var suite JunitSuite
		err = xml.Unmarshal(buf, &suite)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s: %v", ap, err)
		}
		for _, s := range suite.Results {
			if s.Skipped != nil && len(*s.Skipped) == 0 {
				continue
			}

			n := s.Name
			i := 0
			if br.Results == nil {
				br.Results = make(map[string]state.Row_Result)
			}
			for {
				_, ok := br.Results[n]
				if !ok {
					break
				}
				i++
				n = fmt.Sprintf("%s [%d]", s.Name, i)
			}
			switch {
			case s.Failure != nil:
				// TODO: extract failure reason
				br.Results[n] = state.Row_FAIL
			case s.Skipped != nil:
				// TODO: extract skip reason
				br.Results[n] = state.Row_PASS_WITH_SKIPS
			default:
				br.Results[n] = state.Row_PASS
			}
		}
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

func ReadBuilds(builds chan Build, max int, dur time.Duration) {
	log.Println("Reading builds...")
	i := 0
	var stop time.Time
	if dur != 0 {
		stop = time.Now().Add(-dur)
	}
	for b := range builds {
		i++
		if max > 0 && i > max {
			log.Printf("Hit ceiling of %d results", max)
			break
		}
		br, err := ReadBuild(b)
		if err != nil {
			log.Printf("FAIL %s: %v", b.Prefix, err)
			continue
		}
		log.Printf("found! %+v", br)
		if br.Started < stop.Unix() {
			log.Printf("Latest result before %s", stop)
			break
		}
	}
	log.Println("Finished reading builds.")
	for _ = range builds {
	}
}

func Days(d int) time.Duration {
	return 24 * time.Duration(d) * time.Hour // Close enough
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
	go ReadBuilds(builds, 10, Days(30))
	if err = ListBuilds(client, ctx, "gs://kubernetes-jenkins/pr-logs/pull-ingress-gce-e2e", builds); err != nil {
		log.Fatalf("Failed to list builds: %v", err)
	}
	close(builds)
	log.Println("Sleep")
	time.Sleep(10 * time.Second)
}
