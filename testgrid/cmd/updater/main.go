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
	"bytes"
	"compress/zlib"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"hash/crc32"
	"io/ioutil"
	"log"
	"net/url"
	"path"
	"regexp"
	"time"

	"k8s.io/test-infra/testgrid/config"
	"k8s.io/test-infra/testgrid/state"

	"cloud.google.com/go/storage"
	"github.com/golang/protobuf/proto"
	"google.golang.org/api/iterator"
)

type Build struct {
	Bucket  *storage.BucketHandle
	Context context.Context
	Prefix  string
}

type Started struct {
	Timestamp   int64             `json:"timestamp"` // epoch seconds
	RepoVersion string            `json:"repo-version"`
	Node        string            `json:"node"`
	Pull        string            `json:"pull"`
	Repos       map[string]string `json:"repos"` // {repo: branch_or_pull} map
}

type Finished struct {
	Timestamp  int64    `json:"timestamp"` // epoch seconds
	Passed     bool     `json:"passed"`
	JobVersion string   `json:"job-version"`
	Metadata   Metadata `json:"metadata"`
}

// infra-commit, repos, repo, repo-commit, others
type Metadata map[string]interface{}

func (m Metadata) String(name string) (*string, bool) {
	if v, ok := m[name]; !ok {
		return nil, false
	} else if t, good := v.(string); !good {
		return nil, true
	} else {
		return &t, true
	}
}

func (m Metadata) Meta(name string) (*Metadata, bool) {
	if v, ok := m[name]; !ok {
		return nil, true
	} else if t, good := v.(Metadata); !good {
		return nil, false
	} else {
		return &t, true
	}
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

func extractRows(buf []byte, results map[string]state.Row_Result) error {
	var suites JunitSuites
	// Try to parse it as a <testsuites/> object
	err := xml.Unmarshal(buf, &suites)
	if err != nil {
		// Maybe it is a <testsuite/> object instead
		suites.Suites = append([]JunitSuite(nil), JunitSuite{})
		ie := xml.Unmarshal(buf, &suites.Suites[0])
		if ie != nil {
			// Nope, it just doesn't parse
			return fmt.Errorf("not valid testsuites: %v nor testsuite: %v", err, ie)
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
				_, ok := results[n]
				if !ok {
					break
				}
				i++
				n = fmt.Sprintf("%s [%d]", prefix, i)
			}
			results[n] = sr.RowResult()
		}
	}
	return nil
}

type BuildResult struct {
	Id       string
	Started  int64
	Finished int64
	Passed   bool
	Results  map[string]state.Row_Result
	Metadata Metadata
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

var uniq int

func AppendResult(row *state.Row, result state.Row_Result, count int) {
	latest := int32(result)
	n := len(row.Results)
	switch {
	case n == 0, row.Results[n-2] != latest:
		row.Results = append(row.Results, latest, int32(count))
	default:
		row.Results[n-1] += int32(count)
	}
	for i := 0; i < count; i++ {
		row.CellId = append(row.CellId, fmt.Sprintf("%d", uniq))
		row.Text = append(row.Text, fmt.Sprintf("messsage %d", uniq))
		row.Annotations = append(row.Annotations, string(int('A')+uniq%26))
		uniq++
	}
}

func AppendColumn(headers []string, grid *state.Grid, rows map[string]*state.Row, build BuildResult) {
	c := state.Column{
		Build:   build.Id,
		Started: float64(build.Started * 1000),
	}
	for _, h := range headers {
		if build.Finished == 0 {
			c.Extra = append(c.Extra, "")
			continue
		}
		trunc := 0
		if h == "Commit" { // TODO(fejta): fix
			h = "repo-commit"
			trunc = 9
		}
		var v string
		p, ok := build.Metadata.String(h)
		if !ok {
			log.Printf("%s metadata missing %s", c.Build, h)
			v = "missing"
		} else if p == nil {
			log.Printf("%s metadata has malformed %s", c.Build, h)
			v = "malformed"
		} else {
			v = *p
		}
		if trunc > 0 && trunc < len(v) {
			v = v[0:trunc]
		}
		c.Extra = append(c.Extra, v)
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
	br.Results = map[string]state.Row_Result{}

	f := build.Bucket.Object(build.Prefix + "finished.json")
	fr, err := f.NewReader(build.Context)
	if err == storage.ErrObjectNotExist {
		br.Results["Overall"] = br.Overall()
		return &br, nil
	}

	var finished Finished
	if err = json.NewDecoder(fr).Decode(&finished); err != nil {
		return nil, fmt.Errorf("could not decode finished.json: %v", err)
	}

	br.Finished = finished.Timestamp
	br.Metadata = finished.Metadata
	br.Passed = finished.Passed

	br.Results["Overall"] = br.Overall()

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

		if err = extractRows(buf, br.Results); err != nil {
			return nil, fmt.Errorf("failed to parse %s: %v", ap, err)
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

func Headers(group config.TestGroup) []string {
	var extra []string
	for _, h := range group.ColumnHeader {
		extra = append(extra, h.ConfigurationValue)
	}
	return extra
}

func ReadBuilds(group config.TestGroup, builds chan Build, max int, dur time.Duration) state.Grid {
	i := 0
	var stop time.Time
	if dur != 0 {
		stop = time.Now().Add(-dur)
	}
	grid := &state.Grid{}
	h := Headers(group)
	rows := map[string]*state.Row{}
	log.Printf("Reading builds after %s (%d)", stop, stop.Unix())
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
		AppendColumn(h, grid, rows, *br)
		log.Printf("found: %s pass:%t %d-%d: %d results", br.Id, br.Passed, br.Started, br.Finished, len(br.Results))
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

func Days(d float64) time.Duration {
	return time.Duration(24*d) * time.Hour // Close enough
}

func ReadConfig(obj *storage.ObjectHandle, ctx context.Context) (*config.Configuration, error) {
	r, err := obj.NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to open config: %v", err)
	}
	buf, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %v", err)
	}
	var cfg config.Configuration
	if err = proto.Unmarshal(buf, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse: %v", err)
	}
	return &cfg, nil
}

func Group(cfg config.Configuration, name string) (*config.TestGroup, bool) {
	for _, g := range cfg.TestGroups {
		if g.Name == name {
			return g, true
		}
	}
	return nil, false
}

func main() {
	b := "fejternetes"
	o := "ci-kubernetes-test-go"

	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("Failed to create storage client: %v", err)
	}

	cfg, err := ReadConfig(client.Bucket(b).Object("config"), ctx)
	if err != nil {
		log.Fatalf("Failed to read gs://%s/config: %v", b, err)
	}
	tg, ok := Group(*cfg, o)
	if !ok {
		log.Fatalf("Failed to find %s in gs://%s/config", o, b)
	}
	log.Println(tg)

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
	grid := ReadBuilds(*tg, builds, 1000, Days(0.25))
	log.Printf("Grid: %d %s", len(grid.Columns), grid.String())
	buf, err := proto.Marshal(&grid)
	if err != nil {
		log.Fatalf("Failed to encode grid: %v", err)
	}
	var zbuf bytes.Buffer
	zw := zlib.NewWriter(&zbuf)
	if _, err = zw.Write(buf); err != nil {
		log.Fatalf("Failed to compress gs://%s/%s: %v", b, o, err)
	}
	if err = zw.Close(); err != nil {
		log.Fatalf("Failed to close zlib gs://%s/%s buffer: %v", b, o, err)
	}
	if b == "k8s-testgrid" {
		log.Fatalf("do not change prod")
	}
	w := client.Bucket(b).Object(o).NewWriter(ctx)
	w.SendCRC32C = true
	buf = zbuf.Bytes()
	w.ObjectAttrs.CRC32C = crc32.Checksum(buf, crc32.MakeTable(crc32.Castagnoli))
	w.ProgressFunc = func(bytes int64) {
		log.Printf("Uploading gs://%s/%s: %d/%d...", b, o, bytes, len(buf))
	}
	if n, err := w.Write(buf); err != nil {
		log.Fatalf("Failed to write gs://%s/%s: %v", b, o, err)
	} else if n != len(buf) {
		log.Fatalf("Partial gs://%s/%s write: %d < %d", b, o, n, len(buf))
	}
	if err = w.Close(); err != nil {
		log.Fatalf("Failed to close write to gs://%s/%s: %v", b, o, err)
	}
	log.Print("Success!")
}
