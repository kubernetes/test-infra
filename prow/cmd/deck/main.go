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
	"path"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/NYTimes/gziphandler"
	"github.com/ghodss/yaml"
	"github.com/gorilla/sessions"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/option"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/deck/jobs"
	"k8s.io/test-infra/prow/githuboauth"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/prstatus"
	"k8s.io/test-infra/prow/spyglass"

	// Import standard spyglass viewers

	_ "k8s.io/test-infra/prow/spyglass/viewers/buildlog"
	_ "k8s.io/test-infra/prow/spyglass/viewers/junit"
	_ "k8s.io/test-infra/prow/spyglass/viewers/metadata"
)

type options struct {
	configPath            string
	jobConfigPath         string
	buildCluster          string
	tideURL               string
	hookURL               string
	oauthURL              string
	githubOAuthConfigFile string
	cookieSecretFile      string
	redirectHTTPTo        string
	hiddenOnly            bool
	runLocal              bool
	staticFilesLocation   string
}

func (o *options) Validate() error {
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
	flag.StringVar(&o.configPath, "config-path", "/etc/config/config.yaml", "Path to config.yaml.")
	flag.StringVar(&o.jobConfigPath, "job-config-path", "", "Path to prow job configs.")
	flag.StringVar(&o.buildCluster, "build-cluster", "", "Path to file containing a YAML-marshalled kube.Cluster object. If empty, uses the local cluster.")
	flag.StringVar(&o.tideURL, "tide-url", "", "Path to tide. If empty, do not serve tide data.")
	flag.StringVar(&o.hookURL, "hook-url", "", "Path to hook plugin help endpoint.")
	flag.StringVar(&o.oauthURL, "oauth-url", "", "Path to deck user dashboard endpoint.")
	flag.StringVar(&o.githubOAuthConfigFile, "github-oauth-config-file", "/etc/github/secret", "Path to the file containing the GitHub App Client secret.")
	flag.StringVar(&o.cookieSecretFile, "cookie-secret", "/etc/cookie/secret", "Path to the file containing the cookie secret key.")
	// use when behind a load balancer
	flag.StringVar(&o.redirectHTTPTo, "redirect-http-to", "", "Host to redirect http->https to based on x-forwarded-proto == http.")
	// use when behind an oauth proxy
	flag.BoolVar(&o.hiddenOnly, "hidden-only", false, "Show only hidden jobs. Useful for serving hidden jobs behind an oauth proxy.")
	flag.BoolVar(&o.runLocal, "run-local", false, "Serve a local copy of the UI, used by the prow/cmd/deck/runlocal script")
	flag.StringVar(&o.staticFilesLocation, "static-files-location", "/static", "Path to the static files")
	flag.Parse()
	return o
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

	staticHandlerFromDir := func(dir string) http.Handler {
		return defaultExtension(".html",
			gziphandler.GzipHandler(handleCached(http.FileServer(http.Dir(dir)))))
	}

	// setup config agent, pod log clients etc.
	configAgent := &config.Agent{}
	if err := configAgent.Start(o.configPath, o.jobConfigPath); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	// setup common handlers for local and deployed runs
	mux.Handle("/", staticHandlerFromDir(o.staticFilesLocation))
	mux.Handle("/config", gziphandler.GzipHandler(handleConfig(configAgent)))
	mux.Handle("/branding.js", gziphandler.GzipHandler(handleBranding(configAgent)))
	mux.Handle("/favicon.ico", gziphandler.GzipHandler(handleFavicon(o.staticFilesLocation, configAgent)))

	// when deployed, do the full main
	if !o.runLocal {
		mux = prodOnlyMain(configAgent, o, mux)
	}

	// setup done, actually start the server
	logrus.WithError(http.ListenAndServe(":8080", mux)).Fatal("ListenAndServe returned.")
}

// prodOnlyMain contains logic only used when running deployed, not locally
func prodOnlyMain(configAgent *config.Agent, o options, mux *http.ServeMux) *http.ServeMux {
	kc, err := kube.NewClientInCluster(configAgent.Config().ProwJobNamespace)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting client.")
	}
	kc.SetHiddenReposProvider(func() []string { return configAgent.Config().Deck.HiddenRepos }, o.hiddenOnly)

	var pkcs map[string]*kube.Client
	if o.buildCluster == "" {
		pkcs = map[string]*kube.Client{kube.DefaultClusterAlias: kc.Namespace(configAgent.Config().PodNamespace)}
	} else {
		pkcs, err = kube.ClientMapFromFile(o.buildCluster, configAgent.Config().PodNamespace)
		if err != nil {
			logrus.WithError(err).Fatal("Error getting kube client to build cluster.")
		}
	}
	plClients := map[string]jobs.PodLogClient{}
	for alias, client := range pkcs {
		plClients[alias] = client
	}

	ja := jobs.NewJobAgent(kc, plClients, configAgent)
	ja.Start()

	//TODO: Need to support authenticated buckets, #8910
	c, err := storage.NewClient(context.Background(), option.WithoutAuthentication())
	if err != nil {
		logrus.WithError(err).Fatal("Error getting GCS client")
	}
	sg := spyglass.New(ja, []spyglass.ArtifactFetcher{spyglass.NewGCSArtifactFetcher(c)})

	// setup prod only handlers
	mux.Handle("/data.js", gziphandler.GzipHandler(handleData(ja)))
	mux.Handle("/prowjobs.js", gziphandler.GzipHandler(handleProwJobs(ja)))
	mux.Handle("/badge.svg", gziphandler.GzipHandler(handleBadge(ja)))
	mux.Handle("/log", gziphandler.GzipHandler(handleLog(ja)))
	mux.Handle("/rerun", gziphandler.GzipHandler(handleRerun(kc)))
	mux.Handle("/view/render", gziphandler.GzipHandler(handleArtifactView(sg, configAgent)))
	mux.Handle("/view/prowjob/", gziphandler.GzipHandler(handleRequestProwJobViews(sg, configAgent)))
	mux.Handle("/view/gcs/", gziphandler.GzipHandler(handleRequestGCSJobViews(sg, configAgent)))
	mux.Handle("/view/", gziphandler.GzipHandler(handleRequestJobViews(sg, configAgent)))

	if o.hookURL != "" {
		mux.Handle("/plugin-help.js",
			gziphandler.GzipHandler(handlePluginHelp(newHelpAgent(o.hookURL))))
	}

	if o.tideURL != "" {
		ta := &tideAgent{
			log:  logrus.WithField("agent", "tide"),
			path: o.tideURL,
			updatePeriod: func() time.Duration {
				return configAgent.Config().Deck.TideUpdatePeriod
			},
			hiddenRepos: configAgent.Config().Deck.HiddenRepos,
			hiddenOnly:  o.hiddenOnly,
		}
		ta.start()
		mux.Handle("/tide.js", gziphandler.GzipHandler(handleTide(configAgent, ta)))
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

		goa := githuboauth.NewGithubOAuthAgent(&githubOAuthConfig, logrus.WithField("client", "githuboauth"))
		oauthClient := &oauth2.Config{
			ClientID:     githubOAuthConfig.ClientID,
			ClientSecret: githubOAuthConfig.ClientSecret,
			RedirectURL:  githubOAuthConfig.RedirectURL,
			Scopes:       githubOAuthConfig.Scopes,
			Endpoint:     github.Endpoint,
		}

		repoSet := make(map[string]bool)
		for r := range configAgent.Config().Presubmits {
			repoSet[r] = true
		}
		for _, q := range configAgent.Config().Tide.Queries {
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

// serve with handler but map extensionless URLs to the default
func defaultExtension(extension string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Path) > 0 &&
			r.URL.Path[len(r.URL.Path)-1] != '/' && path.Ext(r.URL.Path) == "" {
			r2 := dupeRequest(r)
			r2.URL.Path = r.URL.Path + extension
			h.ServeHTTP(w, r2)
		} else {
			h.ServeHTTP(w, r)
		}
	})
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
	// broswers decided to (incorrectly) treat no-cache as "never store"
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
			Items []kube.ProwJob `json:"items"`
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

// handleRequestProwJobViews pre-renders a Spyglass page for a Prowjob source
// A valid Prow job view url takes the form:
//
// /view/prowjob/<jobname>/<buildID>
//
// Example:
// - /view/prowjob/echo-test/1021530234601607168
func handleRequestProwJobViews(sg *spyglass.Spyglass, ca *config.Agent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setHeadersNoCaching(w)
		start := time.Now()
		src := strings.TrimPrefix(r.URL.Path, "/view/")
		fmt.Fprint(w, renderSpyglass(sg, ca, src))
		elapsed := time.Since(start)
		logrus.Info("Time taken to return Prowjob view page: ", elapsed)
	}
}

// handleRequestGCSJobViews pre-renders a Spyglass page for a GCS source
// A valid job GCS Job view url takes the form:
//
// /view/gcs/<bucketname>/<jobprefix>
//
// Example:
// - /view/gcs/kubernetes-jenkins/logs/ci-kubernetes-e2e-gce-large-performance/121
func handleRequestGCSJobViews(sg *spyglass.Spyglass, ca *config.Agent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setHeadersNoCaching(w)
		start := time.Now()
		srcData := strings.TrimPrefix(r.URL.Path, "/view/gcs/")
		src := fmt.Sprintf("gs://%s", srcData)
		fmt.Fprint(w, renderSpyglass(sg, ca, src))
		elapsed := time.Since(start)
		logrus.Info("Time taken to return GCS view page: ", elapsed)
	}
}

// handleRequestJobViews handles requests to get all available artifact views for a given job via
// a general src parameter, which can contain any string used to obtain job artifacts
// A valid general job view url takes the form:
//
// /view/?src=<URI-encoded-source-string>
//
// Example:
// - /view/?src=gs:%2F%2Fkubernetes-jenkins%2Fpr-logs%2Fpull%2Fkubeflow_kubeflow%2F1195%2Fkubeflow-presubmit%2F2558
func handleRequestJobViews(sg *spyglass.Spyglass, ca *config.Agent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setHeadersNoCaching(w)
		start := time.Now()
		src := r.URL.Query().Get("src")
		if src == "" {
			http.Error(w, "missing src query parameter", http.StatusBadRequest)
			return
		}

		fmt.Fprint(w, renderSpyglass(sg, ca, src))
		elapsed := time.Since(start)
		logrus.Info("Time taken to return view page: ", elapsed)
	}
}

// renderSpyglass returns a pre-rendered Spyglass page from the given source string
func renderSpyglass(sg *spyglass.Spyglass, ca *config.Agent, src string) string {
	artifactNames, err := sg.ListArtifacts(src)
	if err != nil {
		logrus.WithError(err).Error("Error listing artifacts")
	}

	viewerCache := map[string][]string{}
	viewersRegistry := ca.Config().Deck.Spyglass.Viewers

	for re, viewerNames := range viewersRegistry {
		matches := []string{}
		r, err := regexp.Compile(re)
		if err != nil {
			logrus.WithError(err).Error("Regexp failed to compile.")
			continue
		}
		for _, a := range artifactNames {
			if r.MatchString(a) {
				matches = append(matches, a)
			}
		}
		for _, vName := range viewerNames {
			viewerCache[vName] = matches
		}
	}

	viewsStart := time.Now()
	lenses := sg.Views(viewerCache)
	viewsElapsed := time.Since(viewsStart)
	logrus.Info("Got views in ", viewsElapsed)

	var viewBuf bytes.Buffer
	type ViewsTemplate struct {
		Views       []spyglass.Lens
		Source      string
		ViewerCache map[string][]string
	}
	vTmpl := ViewsTemplate{
		Views:       lenses,
		Source:      src,
		ViewerCache: viewerCache,
	}
	t := template.Must(template.ParseFiles("/static/template/spyglass-template.html"))
	tErr := t.Execute(&viewBuf, vTmpl)
	if tErr != nil {
		logrus.WithError(tErr).Error("Error rendering template.")
        return
	}
	return viewBuf.String()
}

// handleArtifactView handles requests to load a single view for a job. This is what viewers
// will use to call back to themselves.
// Query params:
// - name: required, specifies the name of the viewer to load
// - src: required, specifies the job source from which to fetch artifacts
func handleArtifactView(sg *spyglass.Spyglass, ca *config.Agent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		setHeadersNoCaching(w)
		w.Header().Set("Content-Type", "application/json")
		name := r.URL.Query().Get("name")
		src := r.URL.Query().Get("src")
		if name == "" {
			http.Error(w, "missing name query parameter", http.StatusBadRequest)
			return
		}
		if src == "" {
			http.Error(w, "missing src query parameter", http.StatusBadRequest)
			return
		}

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}

		viewReq := &spyglass.ViewRequest{}
		err = json.Unmarshal(body, viewReq)
		if err != nil {
			http.Error(w, "failed to marshal request body", http.StatusBadRequest)
			return
		}

		pd := []byte("{}")
		lens, err := sg.Refresh(src, "", ca.Config().Deck.Spyglass.SizeLimit, viewReq)
		if err != nil {
			logrus.WithError(err).Error("failed to refresh page")
			return
		} else {
			pd, err = json.Marshal(lens)
			if err != nil {
				logrus.WithError(err).Error("Error marshaling payload.")
				pd = []byte("{}")
			}
		}

		fmt.Fprint(w, string(pd))
		elapsed := time.Since(start)
		logrus.Infof("Time taken to refresh %s: %s", name, elapsed) //TODO (paulangton): move these load times next to the title of the viewer expose in Prometheus metrics
	}
}

func handleTide(ca *config.Agent, ta *tideAgent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setHeadersNoCaching(w)
		queryConfigs := ca.Config().Tide.Queries

		ta.Lock()
		defer ta.Unlock()
		pools := ta.pools
		queryConfigs, pools = ta.filterHidden(queryConfigs, pools)
		queries := make([]string, 0, len(queryConfigs))
		for _, qc := range queryConfigs {
			queries = append(queries, qc.Query())
		}

		payload := tideData{
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
			logger.WithError(err).Warning("Log not found.")
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

type pjClient interface {
	GetProwJob(string) (kube.ProwJob, error)
}

func handleRerun(kc pjClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("prowjob")
		if name == "" {
			http.Error(w, "request did not provide the 'name' query parameter", http.StatusBadRequest)
			return
		}
		pj, err := kc.GetProwJob(name)
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

func handleConfig(ca jobs.ConfigAgent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// TODO(bentheelder): add the ability to query for portions of the config?
		setHeadersNoCaching(w)
		config := ca.Config()
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

func handleBranding(ca jobs.ConfigAgent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setHeadersNoCaching(w)
		config := ca.Config()
		b, err := json.Marshal(config.Deck.Branding)
		if err != nil {
			logrus.WithError(err).Error("Error marshaling branding config.")
			http.Error(w, "Failed to marshal branding config.", http.StatusInternalServerError)
			return
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

func handleFavicon(staticFilesLocation string, ca jobs.ConfigAgent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		config := ca.Config()
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
