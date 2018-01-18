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
	"path"
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

type JunitSuites struct {
	XMLName xml.Name     `xml:"testsuites"`
	Suites  []JunitSuite `xml:"testsuite"`
}

type JunitSuite struct {
	XMLName  xml.Name      `xml:"testsuite"`
	Name     string        `xml:"name,attr"`
	Time     float64       `xml:"time,attr"`
	Failures int           `xml:"failures,attr"`
	Tests    int           `xml:"tests,attr"`
	Results  []JunitResult `xml:"testcase"`
	/*
	* <properties><property name="go.version" value="go1.8.3"/></properties>
	 */
}

type JunitResult struct {
	Name      string  `xml:"name,attr"`
	Time      float64 `xml:"time,attr"`
	ClassName string  `xml:"classname,attr"`
	Failure   *string `xml:"failure"`
	Output    *string `xml:"system-out"`
	Skipped   *string `xml:"skipped"`
}

func (jr JunitResult) RowResult() state.Row_Result {
	switch {
	case jr.Failure != nil:
		return state.Row_FAIL
	case jr.Skipped != nil:
		return state.Row_PASS_WITH_SKIPS
	}
	return state.Row_PASS
}

type BuildResult struct {
	Id       string
	Started  int64
	Finished int64
	Passed   bool
	Results  map[string]state.Row_Result
}

func (br BuildResult) Overall() state.Row_Result {
	switch {
	case br.Finished > 0:
		// Completed
		if br.Passed {
			return state.Row_PASS
		}
		return state.Row_FAIL
	case time.Now().Add(-24*time.Hour).Unix() > br.Started:
		// Timed out
		return state.Row_FAIL
	default:
		return state.Row_RUNNING
	}
}

func AppendResult(row *state.Row, result state.Row_Result, count int) {
	latest := int32(result)
	n := len(row.Results)
	switch {
	case n == 0, row.Results[n-2] != latest:
		row.Results = append(row.Results, latest, int32(count))
	default:
		row.Results[n-1] += int32(count)
	}
}

func AppendColumn(grid *state.Grid, rows map[string]*state.Row, build BuildResult) {
	c := state.Column{
		Build:   build.Id,
		Started: float64(build.Started * 1000),
	}
	grid.Columns = append(grid.Columns, &c)

	missing := map[string]*state.Row{}
	for name, row := range rows {
		missing[name] = row
	}

	for name, result := range build.Results {
		delete(missing, name)
		r, ok := rows[name]
		if !ok {
			r = &state.Row{
				Name: name,
				Id:   name,
			}
			rows[name] = r
			grid.Rows = append(grid.Rows, r)
			if n := len(grid.Columns); n > 0 {
				// Add missing entries for later builds
				AppendResult(r, state.Row_NO_RESULT, n-1)
			}
		}

		AppendResult(r, result, 1)
	}

	for _, row := range missing {
		AppendResult(row, state.Row_NO_RESULT, 1)
	}
}

func ReadBuild(build Build) (*BuildResult, error) {
	br := BuildResult{
		Id: path.Base(build.Prefix),
	}
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
	br.Results = map[string]state.Row_Result{
		"Overall": br.Overall(),
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

		var suites JunitSuites
		err = xml.Unmarshal(buf, &suites)
		if err != nil {
			suites.Suites = append([]JunitSuite(nil), JunitSuite{})
			ie := xml.Unmarshal(buf, &suites.Suites[0])
			if ie != nil {
				return nil, fmt.Errorf("failed to parse %s: %v and %v", ap, err, ie)
			}
		}
		for _, suite := range suites.Suites {
			for _, sr := range suite.Results {
				if sr.Skipped != nil && len(*sr.Skipped) == 0 {
					continue
				}

				prefix := sr.Name
				if len(suite.Name) > 0 {
					prefix = suite.Name + "." + prefix
				}
				n := prefix
				i := 0
				for {
					_, ok := br.Results[n]
					if !ok {
						break
					}
					i++
					n = fmt.Sprintf("%s [%d]", prefix, i)
				}
				br.Results[n] = sr.RowResult()
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
	var backwards []Build
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

		//fmt.Println("Found name:", objAttrs.Name, "prefix:", objAttrs.Prefix)
		backwards = append(backwards, Build{
			Bucket:  bkt,
			Context: ctx,
			Prefix:  objAttrs.Prefix,
		})
	}
	for i := len(backwards) - 1; i >= 0; i-- {
		builds <- backwards[i]
	}
	return nil
}

func ReadBuilds(builds chan Build, max int, dur time.Duration) state.Grid {
	log.Println("Reading builds...")
	i := 0
	var stop time.Time
	if dur != 0 {
		stop = time.Now().Add(-dur)
	}
	grid := &state.Grid{}
	rows := map[string]*state.Row{}
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
		AppendColumn(grid, rows, *br)
		log.Printf("found! %+v", br)
		if br.Started < stop.Unix() {
			log.Printf("Latest result before %s", stop)
			break
		}
	}
	log.Println("Finished reading builds.")
	for range builds {
	}
	return *grid
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
	uPR := "gs://kubernetes-jenkins/pr-logs/pull-ingress-gce-e2e"
	uCI := "gs://kubernetes-jenkins/logs/ci-kubernetes-test-go"
	_ = uCI
	_ = uPR
	u := uCI
	go func() {
		if err = ListBuilds(client, ctx, u, builds); err != nil {
			log.Fatalf("Failed to list builds: %v", err)
		}
		close(builds)
	}()
	grid := ReadBuilds(builds, 1000, Days(1))
	log.Printf("Grid: %v", grid)
}
