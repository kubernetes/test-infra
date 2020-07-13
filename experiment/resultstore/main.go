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
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"github.com/GoogleCloudPlatform/testgrid/config"
	"github.com/GoogleCloudPlatform/testgrid/metadata"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/GoogleCloudPlatform/testgrid/resultstore"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/yaml"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/logrusutil"
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
	account        string
	buckets        flagutil.Strings
	gcsAuth        bool
	jobs           flagutil.Strings
	latest         int
	override       bool
	path           gcs.Path
	pending        bool
	project        string
	repeat         time.Duration
	secret         string
	testgridConfig string
	timeout        time.Duration
	maxFiles       int
}

func (o *options) parse(flags *flag.FlagSet, args []string) error {
	flags.Var(&o.path, "build", "Download a specific gs://bucket/to/job/build-1234 url (instead of latest builds for each --job)")
	flags.Var(&o.jobs, "job", "Configures specific jobs to update (repeatable, all jobs when --job and --build are both empty)")
	flags.Var(&o.buckets, "bucket", "Filter to specific gs://buckets (repeatable, all buckets when --bucket is empty)")
	flags.StringVar(&o.testgridConfig, "config", "", "Path to /some/testgrid/config.pb (optional gs:// prefix)")
	flags.IntVar(&o.latest, "latest", 1, "Configures the number of latest builds to migrate")
	flags.BoolVar(&o.override, "override", false, "Replace the existing ResultStore data for each build")
	flags.StringVar(&o.account, "service-account", "", "Authenticate with the service account at specified path")
	flags.BoolVar(&o.gcsAuth, "gcs-auth", false, "Use service account for gcs auth if set (default auth if unset)")
	flags.BoolVar(&o.pending, "pending", false, "Include pending results when set (otherwise ignore them)")
	flags.StringVar(&o.project, "upload", "", "Upload results to specified gcp project instead of stdout")
	flags.StringVar(&o.secret, "secret", "", "Use the specified secret guid instead of randomly generating one.")
	flags.DurationVar(&o.timeout, "timeout", 0, "Timeout after the specified deadling duration (use 0 for no timeout)")
	flags.DurationVar(&o.repeat, "repeat", 0, "Repeatedly transfer after sleeping for this duration (exit after one run when 0)")
	flags.IntVar(&o.maxFiles, "max-files", 10000, "Ceiling for number of artifact files (0 for unlimited, server may reject)")
	flags.Parse(args)
	return nil
}

func parseOptions() options {
	var o options
	if err := o.parse(flag.CommandLine, os.Args[1:]); err != nil {
		logrus.WithError(err).Fatal("Invalid flags")
	}
	return o
}

func main() {
	logrusutil.ComponentInit()

	opt := parseOptions()
	for {
		err := run(opt)
		if opt.repeat == 0 {
			if err != nil {
				logrus.WithError(err).Fatal("Failed transfer")
			}
			return
		}
		if err != nil {
			logrus.WithError(err).Error("Failed transfer")
		}
		if opt.repeat > time.Second {
			logrus.Infof("Sleeping for %s...", opt.repeat)
		}
		time.Sleep(opt.repeat)
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

func bucketListChecker(buckets ...string) (bucketChecker, error) {
	bucketNames := map[string]bool{}
	for _, b := range buckets {
		var path gcs.Path
		if err := path.Set(b); err != nil {
			return nil, fmt.Errorf("%q: %w", b, err)
		}
		bucketNames[path.Bucket()] = true
	}
	return func(_ context.Context, name string) bool {
		return bucketNames[name]
	}, nil
}

func run(opt options) error {
	var ctx context.Context
	var cancel context.CancelFunc
	if opt.timeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), opt.timeout)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	defer cancel()

	var gcsAccount string
	if opt.gcsAuth {
		gcsAccount = opt.account
	}
	storageClient, err := storageClient(ctx, gcsAccount)
	if err != nil {
		return fmt.Errorf("storage client: %v", err)
	}

	logrus.WithFields(logrus.Fields{"testgrid": opt.testgridConfig}).Info("Reading testgrid config...")
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
		return transferBuild(ctx, storageClient, rsClient, opt.project, opt.path, opt.override, true, opt.maxFiles)
	}

	groups, err := findGroups(cfg, opt.jobs.Strings()...)
	if err != nil {
		return fmt.Errorf("find groups: %v", err)
	}

	var checkBucket bucketChecker
	if len(opt.buckets.Strings()) > 0 {
		var err error
		if checkBucket, err = bucketListChecker(opt.buckets.String()); err != nil {
			return fmt.Errorf("parse bucket list: %w", err)
		}
	} else {
		checkWritable := func(ctx context.Context, name string) bool {
			const want = "storage.objects.create"
			have, err := storageClient.Bucket(name).IAM().TestPermissions(ctx, []string{want})
			if err != nil || len(have) != 1 || have[0] != want {
				logrus.WithError(err).WithFields(logrus.Fields{"bucket": name, "want": want, "have": have}).Error("No write access")
				return false
			}
			return true
		}
		checkBucket = checkWritable
	}

	groups, err = filterBuckets(ctx, checkBucket, groups...)

	logrus.Infof("Finding latest builds for %d groups...\n", len(groups))
	prefilteredBuckets := len(opt.buckets.Strings()) > 0
	buildsChan, buildsErrChan := findBuilds(ctx, storageClient, groups, prefilteredBuckets)
	transferErrChan := transfer(ctx, storageClient, rsClient, opt, buildsChan)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-buildsErrChan:
		if err != nil {
			return fmt.Errorf("find builds: %v", err)
		}
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-transferErrChan:
		if err != nil {
			return fmt.Errorf("transfer: %v", err)
		}
	}
	return nil
}

type bucketChecker func(context.Context, string) bool

func filterBuckets(parent context.Context, checkBucket bucketChecker, groups ...configpb.TestGroup) ([]configpb.TestGroup, error) {
	buckets := map[string]bool{}
	valid := func(path gcs.Path) bool {
		name := path.Bucket()
		if good, ok := buckets[name]; ok {
			return good
		}

		ctx, cancel := context.WithTimeout(parent, 10*time.Second)
		defer cancel()
		result := checkBucket(ctx, name)
		buckets[name] = result
		return result
	}

	var ret []configpb.TestGroup
	var path gcs.Path
	for _, g := range groups {
		if err := path.Set("gs://" + g.GcsPrefix); err != nil {
			return nil, fmt.Errorf("bad group prefix %s: %w", g.Name, err)
		}
		if !valid(path) {
			logrus.WithFields(logrus.Fields{
				"testgroup":  g.Name,
				"gcs_prefix": "gs://" + g.GcsPrefix,
			}).Info("Skip unwritable group")
			continue
		}
		ret = append(ret, g)
	}
	return ret, nil
}

func joinErrs(errs []error, sep string) string {
	var out []string
	for _, e := range errs {
		out = append(out, e.Error())
	}
	return strings.Join(out, sep)
}

func transfer(ctx context.Context, storageClient *storage.Client, rsClient *resultstore.Client, opt options, buildsChan <-chan buildsInfo) <-chan error {
	retChan := make(chan error)
	go func() {
		transferErrChan := make(chan error)
		var wg sync.WaitGroup
		var total int
		for info := range buildsChan {
			total++
			wg.Add(1)
			go func(info buildsInfo) {
				defer wg.Done()
				if err := transferLatest(ctx, storageClient, rsClient, opt.project, info.builds, opt.latest, opt.override, opt.pending, opt.maxFiles); err != nil {
					logrus.WithError(err).Error("Transfer failed")
					select {
					case <-ctx.Done():
					case transferErrChan <- fmt.Errorf("transfer %s in %s: %v", info.name, info.prefix, err):
					}
				}
			}(info)
		}
		go func() {
			defer close(transferErrChan)
			wg.Wait()
		}()
		var errs []error
		for err := range transferErrChan {
			errs = append(errs, err)
		}
		var err error
		if n := len(errs); n > 0 {
			err = fmt.Errorf("%d errors transferring %d groups: %v", n, total, joinErrs(errs, ", "))
		}
		select {
		case <-ctx.Done():
		case retChan <- err:
		}

	}()
	return retChan
}

func findGroups(cfg *configpb.Configuration, jobs ...string) ([]configpb.TestGroup, error) {
	var groups []configpb.TestGroup
	for _, job := range jobs {
		tg := config.FindTestGroup(job, cfg)
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

type buildsInfo struct {
	name   string
	prefix gcs.Path
	builds []gcs.Build
}

func findGroupBuilds(ctx context.Context, storageClient *storage.Client, group configpb.TestGroup, buildsChan chan<- buildsInfo, errChan chan<- error) {
	log := logrus.WithFields(logrus.Fields{
		"testgroup":  group.Name,
		"gcs_prefix": "gs://" + group.GcsPrefix,
	})
	log.Debug("Get latest builds...")
	tgPath, err := gcs.NewPath("gs://" + group.GcsPrefix)
	if err != nil {
		log.WithError(err).Error("Bad build URL")
		err = fmt.Errorf("test group %s: gs://%s prefix invalid: %v", group.Name, group.GcsPrefix, err)
		select {
		case <-ctx.Done():
		case errChan <- err:
		}
		return
	}

	builds, err := gcs.ListBuilds(ctx, storageClient, *tgPath)
	if err != nil {
		log.WithError(err).Error("Failed to list builds")
		err := fmt.Errorf("test group %s: list %s: %v", group.Name, *tgPath, err)
		select {
		case <-ctx.Done():
		case errChan <- err:
		}
		return
	}
	info := buildsInfo{
		name:   group.Name,
		prefix: *tgPath,
		builds: builds,
	}
	select {
	case <-ctx.Done():
	case buildsChan <- info:
	}
}

func findBuilds(ctx context.Context, storageClient *storage.Client, groups []configpb.TestGroup, prefilteredBuckets bool) (<-chan buildsInfo, <-chan error) {
	buildsChan := make(chan buildsInfo)
	errChan := make(chan error)
	go func() {
		innerErrChan := make(chan error)
		defer close(buildsChan)
		defer close(errChan)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		var wg sync.WaitGroup
		for _, testGroup := range groups {
			wg.Add(1)
			go func(testGroup configpb.TestGroup) {
				defer wg.Done()
				findGroupBuilds(ctx, storageClient, testGroup, buildsChan, innerErrChan)
			}(testGroup)
		}
		go func() {
			defer close(innerErrChan)
			wg.Wait()
		}()
		var errs []error
		for err := range innerErrChan {
			errs = append(errs, err)
		}

		var err error
		if n := len(errs); n > 0 {
			err = fmt.Errorf("%d errors finding builds from %d groups: %v", n, len(groups), joinErrs(errs, ", "))
		}

		select {
		case <-ctx.Done():
		case errChan <- err:
		}
	}()
	return buildsChan, errChan
}

func transferLatest(ctx context.Context, storageClient *storage.Client, rsClient *resultstore.Client, project string, builds gcs.Builds, max int, override bool, includePending bool, maxFiles int) error {

	for i, build := range builds {
		if i >= max {
			break
		}
		path, err := gcs.NewPath(fmt.Sprintf("gs://%s/%s", build.BucketPath, build.Prefix))
		if err != nil {
			return fmt.Errorf("bad %s path: %v", build, err)
		}
		if err := transferBuild(ctx, storageClient, rsClient, project, *path, override, includePending, maxFiles); err != nil {
			return fmt.Errorf("%s: %v", build, err)
		}
	}
	return nil
}

func transferBuild(ctx context.Context, storageClient *storage.Client, rsClient *resultstore.Client, project string, path gcs.Path, override bool, includePending bool, maxFiles int) error {
	build := gcs.Build{
		Bucket:     storageClient.Bucket(path.Bucket()),
		Prefix:     trailingSlash(path.Object()),
		BucketPath: path.Bucket(),
	}

	log := logrus.WithFields(logrus.Fields{"build": build})

	log.Debug("Downloading...")
	result, err := download(ctx, storageClient, build)
	if err != nil {
		return fmt.Errorf("download: %v", err)
	}

	switch val, _ := result.started.Metadata.String(resultstoreKey); {
	case val != nil && override:
		log = log.WithFields(logrus.Fields{"previously": *val})
		log.Warn("Replacing result...")
	case val != nil:
		log.WithFields(logrus.Fields{
			"resultstore": *val,
		}).Debug("Already transferred")
		return nil
	}

	if (result.started.Pending || result.finished.Running) && !includePending {
		log.Debug("Skip pending result")
		return nil
	}

	desc := "Results of " + path.String()
	log.Debug("Converting...")
	inv, target, test := convert(project, desc, path, *result, maxFiles)

	if project == "" {
		print(inv.To(), test.To())
		return nil
	}

	log.Debug("Uploading...")
	viewURL, err := upload(rsClient, inv, target, test)
	if err != nil {
		return fmt.Errorf("upload %s: %v", viewURL, err)
	}
	log = log.WithFields(logrus.Fields{"resultstore": viewURL})
	log.Info("Transferred result")
	changed, err := insertLink(&result.started, viewURL)
	if err != nil {
		return fmt.Errorf("insert resultstore link into metadata: %v", err)
	}
	if !changed { // already has the link
		return nil
	}
	log.Debug("Inserting link...")
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
func insertLink(started *gcs.Started, viewURL string) (bool, error) {
	if started.Metadata == nil {
		started.Metadata = metadata.Metadata{}
	}
	meta := started.Metadata
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
	startedPath, err := path.ResolveReference(&url.URL{Path: prowv1.StartedStatusFile})
	if err != nil {
		return fmt.Errorf("resolve started.json: %v", err)
	}
	buf, err := json.Marshal(started)
	if err != nil {
		return fmt.Errorf("encode started.json: %v", err)
	}
	// TODO(fejta): compare and swap
	if err := gcs.Upload(ctx, storageClient, *startedPath, buf, gcs.DefaultAcl, ""); err != nil {
		return fmt.Errorf("upload started.json: %v", err)
	}
	return nil
}
