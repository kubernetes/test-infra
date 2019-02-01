/*
Copyright 2016 The Kubernetes Authors.

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
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/NYTimes/gziphandler"
	"github.com/gorilla/sessions"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"google.golang.org/api/option"
	coreapi "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"sigs.k8s.io/yaml"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	prowv1 "k8s.io/test-infra/prow/client/clientset/versioned/typed/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/deck/jobs"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/githuboauth"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/prstatus"
	"k8s.io/test-infra/prow/spyglass"

	// Import standard spyglass viewers

	"k8s.io/test-infra/prow/spyglass/lenses"
	_ "k8s.io/test-infra/prow/spyglass/lenses/buildlog"
	_ "k8s.io/test-infra/prow/spyglass/lenses/junit"
	_ "k8s.io/test-infra/prow/spyglass/lenses/metadata"
)

type options struct {
	configPath            string
	jobConfigPath         string
	buildCluster          string
	kubernetes            prowflagutil.KubernetesOptions
	tideURL               string
	hookURL               string
	oauthURL              string
	githubOAuthConfigFile string
	cookieSecretFile      string
	redirectHTTPTo        string
	hiddenOnly            bool
	pregeneratedData      string
	staticFilesLocation   string
	templateFilesLocation string
	spyglass              bool
	spyglassFilesLocation string
	gcsCredentialsFile    string
}

func (o *options) Validate() error {
	if err := o.kubernetes.Validate(false); err != nil {
		return err
	}

	if o.configPath == "" {
		return errors.New("required flag --config-path was unset")
	}
	if o.oauthURL != "" {
		if o.githubOAuthConfigFile == "" {
			return errors.New("an OAuth URL was provided but required flag --github-oauth-config-file was unset")
		}
		if o.cookieSecretFile == "" {
			return errors.New("an OAuth URL was provided but required flag --cookie-secret was unset")
		}
	}
	return nil
}

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&o.configPath, "config-path", "/etc/config/config.yaml", "Path to config.yaml.")
	fs.StringVar(&o.jobConfigPath, "job-config-path", "", "Path to prow job configs.")
	fs.StringVar(&o.tideURL, "tide-url", "", "Path to tide. If empty, do not serve tide data.")
	fs.StringVar(&o.hookURL, "hook-url", "", "Path to hook plugin help endpoint.")
	fs.StringVar(&o.oauthURL, "oauth-url", "", "Path to deck user dashboard endpoint.")
	fs.StringVar(&o.githubOAuthConfigFile, "github-oauth-config-file", "/etc/github/secret", "Path to the file containing the GitHub App Client secret.")
	fs.StringVar(&o.cookieSecretFile, "cookie-secret", "/etc/cookie/secret", "Path to the file containing the cookie secret key.")
	// use when behind a load balancer
	fs.StringVar(&o.redirectHTTPTo, "redirect-http-to", "", "Host to redirect http->https to based on x-forwarded-proto == http.")
	// use when behind an oauth proxy
	fs.BoolVar(&o.hiddenOnly, "hidden-only", false, "Show only hidden jobs. Useful for serving hidden jobs behind an oauth proxy.")
	fs.StringVar(&o.pregeneratedData, "pregenerated-data", "", "Use API output from another prow instance. Used by the prow/cmd/deck/runlocal script")
	fs.BoolVar(&o.spyglass, "spyglass", false, "Use Prow built-in job viewing instead of Gubernator")
	fs.StringVar(&o.spyglassFilesLocation, "spyglass-files-location", "/lenses", "Location of the static files for spyglass.")
	fs.StringVar(&o.staticFilesLocation, "static-files-location", "/static", "Path to the static files")
	fs.StringVar(&o.templateFilesLocation, "template-files-location", "/template", "Path to the template files")
	fs.StringVar(&o.gcsCredentialsFile, "gcs-credentials-file", "", "Path to the GCS credentials file")
	o.kubernetes.AddFlags(fs)
	fs.Parse(os.Args[1:])
	return o
}

func staticHandlerFromDir(dir string) http.Handler {
	return gziphandler.GzipHandler(handleCached(http.FileServer(http.Dir(dir))))
}

func main() {
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "deck"}),
	)

	mux := http.NewServeMux()

	// setup config agent, pod log clients etc.
	configAgent := &config.Agent{}
	if err := configAgent.Start(o.configPath, o.jobConfigPath); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}
	cfg := configAgent.Config

	// setup common handlers for local and deployed runs
	mux.Handle("/static/", http.StripPrefix("/static", staticHandlerFromDir(o.staticFilesLocation)))
	mux.Handle("/config", gziphandler.GzipHandler(handleConfig(cfg)))
	mux.Handle("/favicon.ico", gziphandler.GzipHandler(handleFavicon(o.staticFilesLocation, cfg)))

	// Set up handlers for template pages.
	mux.Handle("/pr", gziphandler.GzipHandler(handleSimpleTemplate(o, cfg, "pr.html", nil)))
	mux.Handle("/command-help", gziphandler.GzipHandler(handleSimpleTemplate(o, cfg, "command-help.html", nil)))
	mux.Handle("/plugin-help", http.RedirectHandler("/command-help", http.StatusMovedPermanently))
	mux.Handle("/tide", gziphandler.GzipHandler(handleSimpleTemplate(o, cfg, "tide.html", nil)))
	mux.Handle("/tide-history", gziphandler.GzipHandler(handleSimpleTemplate(o, cfg, "tide-history.html", nil)))
	mux.Handle("/plugins", gziphandler.GzipHandler(handleSimpleTemplate(o, cfg, "plugins.html", nil)))

	indexHandler := handleSimpleTemplate(o, cfg, "index.html", struct{ SpyglassEnabled bool }{o.spyglass})

	runLocal := o.pregeneratedData != ""

	var fallbackHandler func(http.ResponseWriter, *http.Request)
	if runLocal {
		localDataHandler := staticHandlerFromDir(o.pregeneratedData)
		fallbackHandler = localDataHandler.ServeHTTP
	} else {
		fallbackHandler = http.NotFound
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			fallbackHandler(w, r)
			return
		}
		indexHandler(w, r)
	})

	if runLocal {
		mux = localOnlyMain(cfg, o, mux)
	} else {
		mux = prodOnlyMain(cfg, o, mux)
	}

	// setup done, actually start the server
	logrus.WithError(http.ListenAndServe(":8080", mux)).Fatal("ListenAndServe returned.")
}

// localOnlyMain contains logic used only when running locally, and is mutually exclusive with
// prodOnlyMain.
func localOnlyMain(cfg config.Getter, o options, mux *http.ServeMux) *http.ServeMux {
	mux.Handle("/github-login", gziphandler.GzipHandler(handleSimpleTemplate(o, cfg, "github-login.html", nil)))

	if o.spyglass {
		initSpyglass(cfg, o, mux, nil)
	}

	return mux
}

type podLogClient struct {
	client corev1.PodInterface
}

func (c *podLogClient) GetLogs(name string, opts *coreapi.PodLogOptions) ([]byte, error) {
	reader, err := c.client.GetLogs(name, &coreapi.PodLogOptions{Container: kube.TestContainerName}).Stream()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return ioutil.ReadAll(reader)
}

type filteringProwJobLister struct {
	client      prowv1.ProwJobInterface
	hiddenRepos sets.String
	hiddenOnly  bool
}

func (c *filteringProwJobLister) ListProwJobs(selector string) ([]prowapi.ProwJob, error) {
	prowJobList, err := c.client.List(metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}

	var filtered []prowapi.ProwJob
	for _, item := range prowJobList.Items {
		if item.Spec.Refs == nil && len(item.Spec.ExtraRefs) == 0 {
			// periodic jobs with no refs cannot be filtered
			filtered = append(filtered, item)
		}

		refs := item.Spec.Refs
		if refs == nil {
			refs = &item.Spec.ExtraRefs[0]
		}
		shouldHide := c.hiddenRepos.HasAny(fmt.Sprintf("%s/%s", refs.Org, refs.Repo), refs.Org)
		if shouldHide == c.hiddenOnly {
			// this is a hidden job, show it if we're asked
			// to only show hidden jobs otherwise hide it
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

// prodOnlyMain contains logic only used when running deployed, not locally
func prodOnlyMain(cfg config.Getter, o options, mux *http.ServeMux) *http.ServeMux {
	prowJobClient, err := o.kubernetes.ProwJobClient(cfg().ProwJobNamespace, false)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting ProwJob client for infrastructure cluster.")
	}

	buildClusterClients, err := o.kubernetes.BuildClusterClients(cfg().PodNamespace, false)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting Kubernetes client.")
	}

	podLogClients := map[string]jobs.PodLogClient{}
	for clusterContext, client := range buildClusterClients {
		podLogClients[clusterContext] = &podLogClient{client: client}
	}

	ja := jobs.NewJobAgent(&filteringProwJobLister{
		client:      prowJobClient,
		hiddenRepos: sets.NewString(cfg().Deck.HiddenRepos...),
		hiddenOnly:  o.hiddenOnly,
	}, podLogClients, cfg)
	ja.Start()

	// setup prod only handlers
	mux.Handle("/data.js", gziphandler.GzipHandler(handleData(ja)))
	mux.Handle("/prowjobs.js", gziphandler.GzipHandler(handleProwJobs(ja)))
	mux.Handle("/badge.svg", gziphandler.GzipHandler(handleBadge(ja)))
	mux.Handle("/log", gziphandler.GzipHandler(handleLog(ja)))
	mux.Handle("/rerun", gziphandler.GzipHandler(handleRerun(prowJobClient)))

	if o.spyglass {
		initSpyglass(cfg, o, mux, ja)
	}

	if o.hookURL != "" {
		mux.Handle("/plugin-help.js",
			gziphandler.GzipHandler(handlePluginHelp(newHelpAgent(o.hookURL))))
	}

	if o.tideURL != "" {
		ta := &tideAgent{
			log:  logrus.WithField("agent", "tide"),
			path: o.tideURL,
			updatePeriod: func() time.Duration {
				return cfg().Deck.TideUpdatePeriod
			},
			hiddenRepos: cfg().Deck.HiddenRepos,
			hiddenOnly:  o.hiddenOnly,
		}
		ta.start()
		mux.Handle("/tide.js", gziphandler.GzipHandler(handleTidePools(cfg, ta)))
		mux.Handle("/tide-history.js", gziphandler.GzipHandler(handleTideHistory(ta)))
	}

	// Enable Git OAuth feature if oauthURL is provided.
	if o.oauthURL != "" {
		githubOAuthConfigRaw, err := loadToken(o.githubOAuthConfigFile)
		if err != nil {
			logrus.WithError(err).Fatal("Could not read github oauth config file.")
		}

		cookieSecretRaw, err := loadToken(o.cookieSecretFile)
		if err != nil {
			logrus.WithError(err).Fatal("Could not read cookie secret file.")
		}

		var githubOAuthConfig config.GithubOAuthConfig
		if err := yaml.Unmarshal(githubOAuthConfigRaw, &githubOAuthConfig); err != nil {
			logrus.WithError(err).Fatal("Error unmarshalling github oauth config")
		}
		if !isValidatedGitOAuthConfig(&githubOAuthConfig) {
			logrus.Fatal("Error invalid github oauth config")
		}

		decodedSecret, err := base64.StdEncoding.DecodeString(string(cookieSecretRaw))
		if err != nil {
			logrus.WithError(err).Fatal("Error decoding cookie secret")
		}
		if len(decodedSecret) == 0 {
			logrus.Fatal("Cookie secret should not be empty")
		}
		cookie := sessions.NewCookieStore(decodedSecret)
		githubOAuthConfig.InitGithubOAuthConfig(cookie)

		goa := githuboauth.NewAgent(&githubOAuthConfig, logrus.WithField("client", "githuboauth"))
		oauthClient := &oauth2.Config{
			ClientID:     githubOAuthConfig.ClientID,
			ClientSecret: githubOAuthConfig.ClientSecret,
			RedirectURL:  githubOAuthConfig.RedirectURL,
			Scopes:       githubOAuthConfig.Scopes,
			Endpoint:     github.Endpoint,
		}

		repoSet := make(map[string]bool)
		for r := range cfg().Presubmits {
			repoSet[r] = true
		}
		for _, q := range cfg().Tide.Queries {
			for _, v := range q.Repos {
				repoSet[v] = true
			}
		}
		var repos []string
		for k, v := range repoSet {
			if v {
				repos = append(repos, k)
			}
		}

		prStatusAgent := prstatus.NewDashboardAgent(
			repos,
			&githubOAuthConfig,
			logrus.WithField("client", "pr-status"))

		mux.Handle("/pr-data.js", handleNotCached(
			prStatusAgent.HandlePrStatus(prStatusAgent)))
		// Handles login request.
		mux.Handle("/github-login", goa.HandleLogin(oauthClient))
		// Handles redirect from Github OAuth server.
		mux.Handle("/github-login/redirect", goa.HandleRedirect(oauthClient, githuboauth.NewGithubClientGetter()))
	}

	// optionally inject http->https redirect handler when behind loadbalancer
	if o.redirectHTTPTo != "" {
		redirectMux := http.NewServeMux()
		redirectMux.Handle("/", func(oldMux *http.ServeMux, host string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("x-forwarded-proto") == "http" {
					redirectURL, err := url.Parse(r.URL.String())
					if err != nil {
						logrus.Errorf("Failed to parse URL: %s.", r.URL.String())
						http.Error(w, "Failed to perform https redirect.", http.StatusInternalServerError)
						return
					}
					redirectURL.Scheme = "https"
					redirectURL.Host = host
					http.Redirect(w, r, redirectURL.String(), http.StatusMovedPermanently)
				} else {
					oldMux.ServeHTTP(w, r)
				}
			}
		}(mux, o.redirectHTTPTo))
		mux = redirectMux
	}
	return mux
}

func initSpyglass(cfg config.Getter, o options, mux *http.ServeMux, ja *jobs.JobAgent) {
	var c *storage.Client
	var err error
	if o.gcsCredentialsFile == "" {
		c, err = storage.NewClient(context.Background(), option.WithoutAuthentication())
	} else {
		c, err = storage.NewClient(context.Background(), option.WithCredentialsFile(o.gcsCredentialsFile))
	}
	if err != nil {
		logrus.WithError(err).Fatal("Error getting GCS client")
	}
	sg := spyglass.New(ja, cfg, c)

	mux.Handle("/spyglass/static/", http.StripPrefix("/spyglass/static", staticHandlerFromDir(o.spyglassFilesLocation)))
	mux.Handle("/spyglass/lens/", gziphandler.GzipHandler(http.StripPrefix("/spyglass/lens/", handleArtifactView(o, sg, cfg))))
	mux.Handle("/view/", gziphandler.GzipHandler(handleRequestJobViews(sg, cfg, o)))
	mux.Handle("/job-history/", gziphandler.GzipHandler(handleJobHistory(o, cfg, c)))
	mux.Handle("/pr-history/", gziphandler.GzipHandler(handlePRHistory(o, cfg, c)))
}

func loadToken(file string) ([]byte, error) {
	raw, err := ioutil.ReadFile(file)
	if err != nil {
		return []byte{}, err
	}
	return bytes.TrimSpace(raw), nil
}

// copy a http.Request
// see: https://go-review.googlesource.com/c/go/+/36483/3/src/net/http/server.go
func dupeRequest(original *http.Request) *http.Request {
	r2 := new(http.Request)
	*r2 = *original
	r2.URL = new(url.URL)
	*r2.URL = *original.URL
	return r2
}

func handleCached(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This looks ridiculous but actually no-cache means "revalidate" and
		// "max-age=0" just means there is no time in which it can skip
		// revalidation. We also need to set must-revalidate because no-cache
		// doesn't imply must-revalidate when using the back button
		// https://www.w3.org/Protocols/rfc2616/rfc2616-sec14.html#sec14.9.1
		// TODO(bentheelder): consider setting a longer max-age
		// setting it this way means the content is always revalidated
		w.Header().Set("Cache-Control", "public, max-age=0, no-cache, must-revalidate")
		next.ServeHTTP(w, r)
	})
}

func setHeadersNoCaching(w http.ResponseWriter) {
	// Note that we need to set both no-cache and no-store because only some
	// browsers decided to (incorrectly) treat no-cache as "never store"
	// IE "no-store". for good measure to cover older browsers we also set
	// expires and pragma: https://stackoverflow.com/a/2068407
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

func handleNotCached(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setHeadersNoCaching(w)
		next.ServeHTTP(w, r)
	}
}

func handleProwJobs(ja *jobs.JobAgent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setHeadersNoCaching(w)
		jobs := ja.ProwJobs()
		if v := r.URL.Query().Get("omit"); v == "pod_spec" {
			for i := range jobs {
				jobs[i].Spec.PodSpec = nil
			}
		}
		jd, err := json.Marshal(struct {
			Items []prowapi.ProwJob `json:"items"`
		}{jobs})
		if err != nil {
			logrus.WithError(err).Error("Error marshaling jobs.")
			jd = []byte("{}")
		}
		// If we have a "var" query, then write out "var value = {...};".
		// Otherwise, just write out the JSON.
		if v := r.URL.Query().Get("var"); v != "" {
			fmt.Fprintf(w, "var %s = %s;", v, string(jd))
		} else {
			fmt.Fprint(w, string(jd))
		}
	}
}

func handleData(ja *jobs.JobAgent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setHeadersNoCaching(w)
		jobs := ja.Jobs()
		jd, err := json.Marshal(jobs)
		if err != nil {
			logrus.WithError(err).Error("Error marshaling jobs.")
			jd = []byte("[]")
		}
		// If we have a "var" query, then write out "var value = {...};".
		// Otherwise, just write out the JSON.
		if v := r.URL.Query().Get("var"); v != "" {
			fmt.Fprintf(w, "var %s = %s;", v, string(jd))
		} else {
			fmt.Fprint(w, string(jd))
		}
	}
}

// handleBadge handles requests to get a badge for one or more jobs
// The url must look like this, where `jobs` is a comma-separated
// list of globs:
//
// /badge.svg?jobs=<glob>[,<glob2>]
//
// Examples:
// - /badge.svg?jobs=pull-kubernetes-bazel-build
// - /badge.svg?jobs=pull-kubernetes-*
// - /badge.svg?jobs=pull-kubernetes-e2e*,pull-kubernetes-*,pull-kubernetes-integration-*
func handleBadge(ja *jobs.JobAgent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setHeadersNoCaching(w)
		wantJobs := r.URL.Query().Get("jobs")
		if wantJobs == "" {
			http.Error(w, "missing jobs query parameter", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")

		allJobs := ja.ProwJobs()
		_, _, svg := renderBadge(pickLatestJobs(allJobs, wantJobs))
		w.Write(svg)
	}
}

// handleJobHistory handles requests to get the history of a given job
// The url must look like this for presubmits:
//
// /job-history/<gcs-bucket-name>/pr-logs/directory/<job-name>
//
// Example:
// - /job-history/kubernetes-jenkins/pr-logs/directory/pull-test-infra-verify-gofmt
//
// For periodics or postsubmits, the url must look like this:
//
// /job-history/<gcs-bucket-name>/logs/<job-name>
//
// Example:
// - /job-history/kubernetes-jenkins/logs/ci-kubernetes-e2e-prow-canary
func handleJobHistory(o options, cfg config.Getter, gcsClient *storage.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setHeadersNoCaching(w)
		tmpl, err := getJobHistory(r.URL, cfg(), gcsClient)
		if err != nil {
			msg := fmt.Sprintf("failed to get job history: %v", err)
			logrus.WithField("url", r.URL).Error(msg)
			http.Error(w, msg, http.StatusInternalServerError)
			return
		}
		handleSimpleTemplate(o, cfg, "job-history.html", tmpl)(w, r)
	}
}

// handlePRHistory handles requests to get the test history if a given PR
// The url must look like this:
//
// /pr-history/<org>/<repo>/<pr number>
func handlePRHistory(o options, cfg config.Getter, gcsClient *storage.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setHeadersNoCaching(w)
		tmpl, err := getPRHistory(r.URL, cfg(), gcsClient)
		if err != nil {
			msg := fmt.Sprintf("failed to get PR history: %v", err)
			logrus.WithField("url", r.URL).Error(msg)
			http.Error(w, msg, http.StatusInternalServerError)
			return
		}
		handleSimpleTemplate(o, cfg, "pr-history.html", tmpl)(w, r)
	}
}

// handleRequestJobViews handles requests to get all available artifact views for a given job.
// The url must specify a storage key type, such as "prowjob" or "gcs":
//
// /view/<key-type>/<key>
//
// Examples:
// - /view/gcs/kubernetes-jenkins/pr-logs/pull/test-infra/9557/pull-test-infra-verify-gofmt/15688/
// - /view/prowjob/echo-test/1046875594609922048
func handleRequestJobViews(sg *spyglass.Spyglass, cfg config.Getter, o options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		setHeadersNoCaching(w)
		src := strings.TrimPrefix(r.URL.Path, "/view/")

		page, err := renderSpyglass(sg, cfg, src, o)
		if err != nil {
			logrus.WithError(err).Error("error rendering spyglass page")
			message := fmt.Sprintf("error rendering spyglass page: %v", err)
			http.Error(w, message, http.StatusInternalServerError)
			return
		}

		fmt.Fprint(w, page)
		elapsed := time.Since(start)
		logrus.WithFields(logrus.Fields{
			"duration": elapsed.String(),
			"endpoint": r.URL.Path,
			"source":   src,
		}).Info("Loading view completed.")
	}
}

// renderSpyglass returns a pre-rendered Spyglass page from the given source string
func renderSpyglass(sg *spyglass.Spyglass, cfg config.Getter, src string, o options) (string, error) {
	renderStart := time.Now()
	artifactNames, err := sg.ListArtifacts(src)
	if err != nil {
		return "", fmt.Errorf("error listing artifacts: %v", err)
	}
	if len(artifactNames) == 0 {
		return "", fmt.Errorf("found no artifacts for %s", src)
	}

	viewerCache := map[string][]string{}
	viewersRegistry := cfg().Deck.Spyglass.Viewers
	regexCache := cfg().Deck.Spyglass.RegexCache

	for re, viewerNames := range viewersRegistry {
		matches := []string{}
		for _, a := range artifactNames {
			if regexCache[re].MatchString(a) {
				matches = append(matches, a)
			}
		}
		if len(matches) > 0 {
			for _, vName := range viewerNames {
				viewerCache[vName] = matches
			}
		}
	}

	ls := sg.Lenses(viewerCache)
	lensNames := []string{}
	for _, l := range ls {
		lensNames = append(lensNames, l.Name())
	}

	jobHistLink := ""
	jobPath, err := sg.JobPath(src)
	if err == nil {
		jobHistLink = path.Join("/job-history", jobPath)
	}
	logrus.Infof("job history link: %s", jobHistLink)

	var viewBuf bytes.Buffer
	type lensesTemplate struct {
		Lenses        []lenses.Lens
		LensNames     []string
		Source        string
		LensArtifacts map[string][]string
		JobHistLink   string
	}
	lTmpl := lensesTemplate{
		Lenses:        ls,
		LensNames:     lensNames,
		Source:        src,
		LensArtifacts: viewerCache,
		JobHistLink:   jobHistLink,
	}
	t := template.New("spyglass.html")

	if _, err := prepareBaseTemplate(o, cfg, t); err != nil {
		return "", fmt.Errorf("error preparing base template: %v", err)
	}
	t, err = t.ParseFiles(path.Join(o.templateFilesLocation, "spyglass.html"))
	if err != nil {
		return "", fmt.Errorf("error parsing template: %v", err)
	}

	if err = t.Execute(&viewBuf, lTmpl); err != nil {
		return "", fmt.Errorf("error rendering template: %v", err)
	}
	renderElapsed := time.Since(renderStart)
	logrus.WithFields(logrus.Fields{
		"duration": renderElapsed.String(),
		"source":   src,
	}).Info("Rendered spyglass views.")
	return viewBuf.String(), nil
}

// handleArtifactView handles requests to load a single view for a job. This is what viewers
// will use to call back to themselves.
// Query params:
// - name: required, specifies the name of the viewer to load
// - src: required, specifies the job source from which to fetch artifacts
func handleArtifactView(o options, sg *spyglass.Spyglass, cfg config.Getter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setHeadersNoCaching(w)
		pathSegments := strings.Split(r.URL.Path, "/")
		if len(pathSegments) != 2 {
			http.NotFound(w, r)
			return
		}
		lensName := pathSegments[0]
		resource := pathSegments[1]

		lens, err := lenses.GetLens(lensName)
		if err != nil {
			http.Error(w, fmt.Sprintf("No such template: %s (%v)", lensName, err), http.StatusNotFound)
			return
		}

		lensResourcesDir := lenses.ResourceDirForLens(o.spyglassFilesLocation, lens.Name())

		reqString := r.URL.Query().Get("req")
		var request spyglass.LensRequest
		err = json.Unmarshal([]byte(reqString), &request)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to parse request: %v", err), http.StatusBadRequest)
			return
		}

		artifacts, err := sg.FetchArtifacts(request.Source, "", cfg().Deck.Spyglass.SizeLimit, request.Artifacts)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to retrieve expected artifacts: %v", err), http.StatusInternalServerError)
			return
		}

		switch resource {
		case "iframe":
			t, err := template.ParseFiles(path.Join(o.templateFilesLocation, "spyglass-lens.html"))
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to load template: %v", err), http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "text/html; encoding=utf-8")
			t.Execute(w, struct {
				Title   string
				BaseURL string
				Head    template.HTML
				Body    template.HTML
			}{
				lens.Title(),
				"/spyglass/static/" + lensName + "/",
				template.HTML(lens.Header(artifacts, lensResourcesDir)),
				template.HTML(lens.Body(artifacts, lensResourcesDir, "")),
			})
		case "rerender":
			data, err := ioutil.ReadAll(r.Body)
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to read body: %v", err), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; encoding=utf-8")
			w.Write([]byte(lens.Body(artifacts, lensResourcesDir, string(data))))
		case "callback":
			data, err := ioutil.ReadAll(r.Body)
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to read body: %v", err), http.StatusInternalServerError)
				return
			}
			w.Write([]byte(lens.Callback(artifacts, lensResourcesDir, string(data))))
		default:
			http.NotFound(w, r)
		}
	}
}

func handleTidePools(cfg config.Getter, ta *tideAgent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setHeadersNoCaching(w)
		queryConfigs := ta.filterHiddenQueries(cfg().Tide.Queries)
		queries := make([]string, 0, len(queryConfigs))
		for _, qc := range queryConfigs {
			queries = append(queries, qc.Query())
		}

		ta.Lock()
		pools := ta.pools
		ta.Unlock()

		payload := tidePools{
			Queries:     queries,
			TideQueries: queryConfigs,
			Pools:       pools,
		}
		pd, err := json.Marshal(payload)
		if err != nil {
			logrus.WithError(err).Error("Error marshaling payload.")
			pd = []byte("{}")
		}
		// If we have a "var" query, then write out "var value = {...};".
		// Otherwise, just write out the JSON.
		if v := r.URL.Query().Get("var"); v != "" {
			fmt.Fprintf(w, "var %s = %s;", v, string(pd))
		} else {
			fmt.Fprint(w, string(pd))
		}
	}
}

func handleTideHistory(ta *tideAgent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setHeadersNoCaching(w)

		ta.Lock()
		history := ta.history
		ta.Unlock()

		payload := tideHistory{
			History: history,
		}
		pd, err := json.Marshal(payload)
		if err != nil {
			logrus.WithError(err).Error("Error marshaling payload.")
			pd = []byte("{}")
		}
		// If we have a "var" query, then write out "var value = {...};".
		// Otherwise, just write out the JSON.
		if v := r.URL.Query().Get("var"); v != "" {
			fmt.Fprintf(w, "var %s = %s;", v, string(pd))
		} else {
			fmt.Fprint(w, string(pd))
		}
	}
}

func handlePluginHelp(ha *helpAgent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setHeadersNoCaching(w)
		help, err := ha.getHelp()
		if err != nil {
			logrus.WithError(err).Error("Getting plugin help from hook.")
			help = &pluginhelp.Help{}
		}
		b, err := json.Marshal(*help)
		if err != nil {
			logrus.WithError(err).Error("Marshaling plugin help.")
			b = []byte("[]")
		}
		// If we have a "var" query, then write out "var value = [...];".
		// Otherwise, just write out the JSON.
		if v := r.URL.Query().Get("var"); v != "" {
			fmt.Fprintf(w, "var %s = %s;", v, string(b))
		} else {
			fmt.Fprint(w, string(b))
		}
	}
}

type logClient interface {
	GetJobLog(job, id string) ([]byte, error)
}

// TODO(spxtr): Cache, rate limit.
func handleLog(lc logClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setHeadersNoCaching(w)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		job := r.URL.Query().Get("job")
		id := r.URL.Query().Get("id")
		logger := logrus.WithFields(logrus.Fields{"job": job, "id": id})
		if err := validateLogRequest(r); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log, err := lc.GetJobLog(job, id)
		if err != nil {
			http.Error(w, fmt.Sprintf("Log not found: %v", err), http.StatusNotFound)
			logger := logger.WithError(err)
			msg := "Log not found."
			if strings.Contains(err.Error(), "PodInitializing") {
				// PodInitializing is really common and not something
				// that has any actionable items for administrators
				// monitoring logs, so we should log it as information
				logger.Info(msg)
			} else {
				logger.Warning(msg)
			}
			return
		}
		if _, err = w.Write(log); err != nil {
			logger.WithError(err).Warning("Error writing log.")
		}
	}
}

func validateLogRequest(r *http.Request) error {
	job := r.URL.Query().Get("job")
	id := r.URL.Query().Get("id")

	if job == "" {
		return errors.New("request did not provide the 'job' query parameter")
	}
	if id == "" {
		return errors.New("request did not provide the 'id' query parameter")
	}
	return nil
}

func handleRerun(prowJobClient prowv1.ProwJobInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("prowjob")
		if name == "" {
			http.Error(w, "request did not provide the 'name' query parameter", http.StatusBadRequest)
			return
		}
		pj, err := prowJobClient.Get(name, metav1.GetOptions{})
		if err != nil {
			http.Error(w, fmt.Sprintf("ProwJob not found: %v", err), http.StatusNotFound)
			logrus.WithError(err).Warning("ProwJob not found.")
			return
		}
		pjutil := pjutil.NewProwJob(pj.Spec, pj.ObjectMeta.Labels)
		b, err := yaml.Marshal(&pjutil)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error marshaling: %v", err), http.StatusInternalServerError)
			logrus.WithError(err).Error("Error marshaling jobs.")
			return
		}
		if _, err := w.Write(b); err != nil {
			logrus.WithError(err).Error("Error writing log.")
		}
	}
}

func handleConfig(cfg config.Getter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// TODO(bentheelder): add the ability to query for portions of the config?
		setHeadersNoCaching(w)
		config := cfg()
		b, err := yaml.Marshal(config)
		if err != nil {
			logrus.WithError(err).Error("Error marshaling config.")
			http.Error(w, "Failed to marhshal config.", http.StatusInternalServerError)
			return
		}
		buff := bytes.NewBuffer(b)
		_, err = buff.WriteTo(w)
		if err != nil {
			logrus.WithError(err).Error("Error writing config.")
			http.Error(w, "Failed to write config.", http.StatusInternalServerError)
		}
	}
}

func handleFavicon(staticFilesLocation string, cfg config.Getter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		config := cfg()
		if config.Deck.Branding != nil && config.Deck.Branding.Favicon != "" {
			http.ServeFile(w, r, staticFilesLocation+"/"+config.Deck.Branding.Favicon)
		} else {
			http.ServeFile(w, r, staticFilesLocation+"/favicon.ico")
		}
	}
}

func isValidatedGitOAuthConfig(githubOAuthConfig *config.GithubOAuthConfig) bool {
	return githubOAuthConfig.ClientID != "" && githubOAuthConfig.ClientSecret != "" &&
		githubOAuthConfig.RedirectURL != "" &&
		githubOAuthConfig.FinalRedirectURL != ""
}
