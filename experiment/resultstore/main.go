/*
Copyright 2019 The Kubernetes Authors.

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

// Resultstore converts --build=gs://prefix/JOB/NUMBER from prow's pod-utils to a ResultStore invocation suite, which it optionally will --upload=gcp-project.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"sigs.k8s.io/yaml"

	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/testgrid/config"
	"k8s.io/test-infra/testgrid/metadata"
	"k8s.io/test-infra/testgrid/resultstore"
	"k8s.io/test-infra/testgrid/util/gcs"
)

var re = regexp.MustCompile(`( ?|^)\[[^]]+\]( |$)`)

// Converts "[k8s.io] hello world [foo]" into "hello world", []string{"k8s.io", "foo"}
func stripTags(str string) (string, []string) {
	tags := re.FindAllString(str, -1)
	for i, w := range tags {
		w = strings.TrimSpace(w)
		tags[i] = w[1 : len(w)-1]
	}
	var reals []string
	for _, p := range re.Split(str, -1) {
		if p == "" {
			continue
		}
		reals = append(reals, p)
	}
	return strings.Join(reals, " "), tags
}

type options struct {
	path           gcs.Path
	jobs           flagutil.Strings
	latest         int
	override       bool
	update         bool
	account        string
	gcsAuth        bool
	project        string
	secret         string
	testgridConfig string
}

func (o *options) parse(flags *flag.FlagSet, args []string) error {
	flags.Var(&o.path, "build", "Download a specific gs://bucket/to/job/build-1234 url (instead of latest builds for each --job)")
	flags.Var(&o.jobs, "job", "Configures specific jobs to update (repeatable, all jobs when --job and --build are both empty)")
	flags.StringVar(&o.testgridConfig, "config", "gs://k8s-testgrid/config", "Path to local/testgrid/config.pb or gs://bucket/testgrid/config.pb")
	flags.IntVar(&o.latest, "latest", 1, "Configures the number of latest builds to migrate")
	flags.BoolVar(&o.override, "override", false, "Replace the existing ResultStore data for each build")
	flags.BoolVar(&o.update, "update", false, "Attempt to update the existing invocation before creating a new one")
	flags.StringVar(&o.account, "service-account", "", "Authenticate with the service account at specified path")
	flags.BoolVar(&o.gcsAuth, "gcs-auth", false, "Use service account for gcs auth if set (default if set)")
	flags.StringVar(&o.project, "upload", "", "Upload results to specified gcp project instead of stdout")
	flags.StringVar(&o.secret, "secret", "", "Use the specified secret guid instead of randomly generating one.")
	flags.Parse(args)
	return nil
}

func parseOptions() options {
	var o options
	if err := o.parse(flag.CommandLine, os.Args[1:]); err != nil {
		log.Fatalf("Invalid flags: %v", err)
	}
	return o
}

func main() {
	if err := run(parseOptions()); err != nil {
		log.Fatalf("Failed: %v", err)
	}
}

func str(inv interface{}) string {
	buf, err := yaml.Marshal(inv)
	if err != nil {
		panic(err)
	}
	return string(buf)
}

func print(inv ...interface{}) {
	for _, i := range inv {
		fmt.Println(str(i))
	}
}

func trailingSlash(s string) string {
	if strings.HasSuffix(s, "/") {
		return s
	}
	return s + "/"
}

func run(opt options) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	var gcsAccount string
	if opt.gcsAuth {
		gcsAccount = opt.account
	}
	storageClient, err := storageClient(ctx, gcsAccount)
	if err != nil {
		return fmt.Errorf("storage client: %v", err)
	}

	fmt.Println("Reading testgrid config...")
	cfg, err := config.Read(opt.testgridConfig, ctx, storageClient)
	if err != nil {
		return fmt.Errorf("read testgrid config: %v", err)
	}

	var rsClient *resultstore.Client
	if opt.project != "" {
		rsClient, err = resultstoreClient(ctx, opt.account, resultstore.Secret(opt.secret))
		if err != nil {
			return fmt.Errorf("resultstore client: %v", err)
		}
	}

	// Should we just transfer a specific build?
	if opt.path.Bucket() != "" { // All valid --build=gs://whatever values have a non-empty bucket.
		return transfer(ctx, storageClient, rsClient, opt.project, opt.path)
	}

	groups, err := findGroups(cfg, opt.jobs.Strings()...)
	if err != nil {
		return fmt.Errorf("find groups: %v", err)
	}

	fmt.Printf("Finding latest builds for %d groups...\n", len(groups))
	buildsChan, buildsErrChan := findBuilds(ctx, storageClient, groups)
	for builds := range buildsChan {
		if err := transferLatest(ctx, storageClient, rsClient, opt.project, builds, opt.latest); err != nil {
			return fmt.Errorf("transfer: %v", err)
		}
	}

	return <-buildsErrChan

	return nil
}

func findGroups(cfg *config.Configuration, jobs ...string) ([]config.TestGroup, error) {
	var groups []config.TestGroup
	for _, job := range jobs {
		tg := cfg.FindTestGroup(job)
		if tg == nil {
			return nil, fmt.Errorf("job %s not found in test groups", job)
		}
		groups = append(groups, *tg)
	}
	if len(jobs) == 0 {
		for _, tg := range cfg.TestGroups {
			groups = append(groups, *tg)
		}
	}
	return groups, nil
}

func findBuilds(ctx context.Context, storageClient *storage.Client, groups []config.TestGroup) (<-chan []gcs.Build, <-chan error) {
	buildsChan := make(chan []gcs.Build)
	errChan := make(chan error, 1)
	go func() {
		defer close(buildsChan)
		defer close(errChan)
		// TODO(fejta): concurrently list groups
		for _, testGroup := range groups {
			fmt.Printf("Finding latest %s builds in gs://%s...\n", testGroup.Name, testGroup.GcsPrefix)
			tgPath, err := gcs.NewPath("gs://" + testGroup.GcsPrefix)
			if err != nil {
				errChan <- fmt.Errorf("test group %s has invalid gcs_prefix %s: %v", testGroup.Name, testGroup.GcsPrefix, err)
				return
			}
			builds, err := gcs.ListBuilds(ctx, storageClient, *tgPath)
			if err != nil {
				errChan <- err
				return
			}
			buildsChan <- builds
		}
		errChan <- nil
	}()
	return buildsChan, errChan
}

func transferLatest(ctx context.Context, storageClient *storage.Client, rsClient *resultstore.Client, project string, builds gcs.Builds, max int) error {
	for i, build := range builds {
		if i >= max {
			break
		}
		path, err := gcs.NewPath(fmt.Sprintf("gs://%s/%s", build.BucketPath, build.Prefix))
		if err != nil {
			return fmt.Errorf("bad %s path: %v", build, err)
		}
		if err := transfer(ctx, storageClient, rsClient, project, *path); err != nil {
			return fmt.Errorf("%s: %v", build, err)
		}
	}
	return nil
}

func transfer(ctx context.Context, storageClient *storage.Client, rsClient *resultstore.Client, project string, path gcs.Path) error {
	build := gcs.Build{
		Bucket:     storageClient.Bucket(path.Bucket()),
		Context:    ctx,
		Prefix:     trailingSlash(path.Object()),
		BucketPath: path.Bucket(),
	}

	result, err := download(ctx, storageClient, build)
	if err != nil {
		return fmt.Errorf("download: %v", err)
	}

	desc := "Results of " + path.String()
	inv, target, test := convert(project, desc, path, *result)
	print(inv.To(), test.To())

	if project == "" {
		return nil
	}

	viewURL, err := upload(rsClient, inv, target, test)
	if viewURL != "" {
		fmt.Println("See results at " + viewURL)
	}
	if err != nil {
		return fmt.Errorf("upload: %v", err)
	}
	if result.started.Metadata == nil {
		result.started.Metadata = metadata.Metadata{}
	}
	changed, err := insertLink(result.started.Metadata, viewURL)
	if err != nil {
		return fmt.Errorf("insert resultstore link into metadata: %v", err)
	}
	if !changed { // already has the link
		return nil
	}
	if err := updateStarted(ctx, storageClient, path, result.started); err != nil {
		return fmt.Errorf("update started.json: %v", err)
	}
	return nil
}

const (
	linksKey       = "links"
	resultstoreKey = "resultstore"
	urlKey         = "url"
)

// insertLink attempts to set metadata.links.resultstore.url to viewURL.
//
// returns true if started metadata was updated.
func insertLink(meta metadata.Metadata, viewURL string) (bool, error) {
	var changed bool
	top, present := meta.String(resultstoreKey)
	if !present || top == nil || *top != viewURL {
		changed = true
		meta[resultstoreKey] = viewURL
	}
	links, present := meta.Meta(linksKey)
	if present && links == nil {
		return false, fmt.Errorf("metadata.links is not a Metadata value: %v", meta[linksKey])
	}
	if links == nil {
		links = &metadata.Metadata{}
		changed = true
	}
	resultstoreMeta, present := links.Meta(resultstoreKey)
	if present && resultstoreMeta == nil {
		return false, fmt.Errorf("metadata.links.resultstore is not a Metadata value: %v", (*links)[resultstoreKey])
	}
	if resultstoreMeta == nil {
		resultstoreMeta = &metadata.Metadata{}
		changed = true
	}
	val, present := resultstoreMeta.String(urlKey)
	if present && val == nil {
		return false, fmt.Errorf("metadata.links.resultstore.url is not a string value: %v", (*resultstoreMeta)[urlKey])
	}
	if !changed && val != nil && *val == viewURL {
		return false, nil
	}

	(*resultstoreMeta)[urlKey] = viewURL
	(*links)[resultstoreKey] = *resultstoreMeta
	meta[linksKey] = *links
	return true, nil
}

func updateStarted(ctx context.Context, storageClient *storage.Client, path gcs.Path, started gcs.Started) error {
	startedPath, err := path.ResolveReference(&url.URL{Path: "started.json"})
	if err != nil {
		return fmt.Errorf("resolve started.json: %v", err)
	}
	buf, err := json.Marshal(started)
	if err != nil {
		return fmt.Errorf("encode started.json: %v", err)
	}
	// TODO(fejta): compare and swap
	if err := gcs.Upload(ctx, storageClient, *startedPath, buf, gcs.Default); err != nil {
		return fmt.Errorf("upload started.json: %v", err)
	}
	return nil
}
