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
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"time"

	"github.com/NYTimes/gziphandler"
	"github.com/ghodss/yaml"
	"github.com/gorilla/sessions"
	"github.com/sirupsen/logrus"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/githuboauth"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/userdashboard"
)

var (
	configPath            = flag.String("config-path", "/etc/config/config", "Path to config.yaml.")
	buildCluster          = flag.String("build-cluster", "", "Path to file containing a YAML-marshalled kube.Cluster object. If empty, uses the local cluster.")
	tideURL               = flag.String("tide-url", "", "Path to tide. If empty, do not serve tide data.")
	hookURL               = flag.String("hook-url", "", "Path to hook plugin help endpoint.")
	oauthUrl              = flag.String("oauth-url", "", "Path to deck user dashboard endpoint.")
	githubOAuthConfigFile = flag.String("github-oauth-config-file", "/etc/github/app", "Path to the file containing the Git App Client secret.")
	cookieSecretFile      = flag.String("cookie-secret", "/etc/cookie/cookie-secret", "Path to the file containing the cookie secret key.")
	// use when behind a load balancer
	redirectHTTPTo = flag.String("redirect-http-to", "", "Host to redirect http->https to based on x-forwarded-proto == http.")
	// use when behind an oauth proxy
	hiddenOnly = flag.Bool("hidden-only", false, "Show only hidden jobs. Useful for serving hidden jobs behind an oauth proxy.")
	runLocal   = flag.Bool("run-local", false, "Serve a local copy of the UI, used by the prow/cmd/deck/runlocal script")
)

// Matches letters, numbers, hyphens, and underscores.
var objReg = regexp.MustCompile(`^[\w-]+$`)

func main() {
	// common setup
	flag.Parse()

	logrus.SetFormatter(&logrus.JSONFormatter{})
	logger := logrus.WithField("component", "deck")

	mux := http.NewServeMux()

	staticHandlerFromDir := func(dir string) http.Handler {
		return defaultExtension(".html",
			gziphandler.GzipHandler(handleCached(http.FileServer(http.Dir(dir)))))
	}

	// locally just serve from ./static, otherwise do the full main
	if *runLocal {
		mux.Handle("/", staticHandlerFromDir("./static"))
	} else {
		mux.Handle("/", staticHandlerFromDir("/static"))
		prodOnlyMain(logger, mux)
	}

	// setup done, actually start the server
	logger.WithError(http.ListenAndServe(":8080", mux)).Fatal("ListenAndServe returned.")
}

// prodOnlyMain contains logic only used when running deployed, not locally
func prodOnlyMain(logger *logrus.Entry, mux *http.ServeMux) {
	// setup config agent, pod log clients etc.
	configAgent := &config.Agent{}
	if err := configAgent.Start(*configPath); err != nil {
		logger.WithError(err).Fatal("Error starting config agent.")
	}

	kc, err := kube.NewClientInCluster(configAgent.Config().ProwJobNamespace)
	if err != nil {
		logger.WithError(err).Fatal("Error getting client.")
	}
	kc.SetHiddenReposProvider(func() []string { return configAgent.Config().Deck.HiddenRepos }, *hiddenOnly)

	var pkcs map[string]*kube.Client
	if *buildCluster == "" {
		pkcs = map[string]*kube.Client{kube.DefaultClusterAlias: kc.Namespace(configAgent.Config().PodNamespace)}
	} else {
		pkcs, err = kube.ClientMapFromFile(*buildCluster, configAgent.Config().PodNamespace)
		if err != nil {
			logger.WithError(err).Fatal("Error getting kube client to build cluster.")
		}
	}
	plClients := map[string]podLogClient{}
	for alias, client := range pkcs {
		plClients[alias] = client
	}

	ja := &JobAgent{
		kc:   kc,
		pkcs: plClients,
		c:    configAgent,
	}
	ja.Start()

	// setup prod only handlers
	mux.Handle("/data.js", gziphandler.GzipHandler(handleData(ja)))
	mux.Handle("/prowjobs.js", gziphandler.GzipHandler(handleProwJobs(ja)))
	mux.Handle("/log", gziphandler.GzipHandler(handleLog(ja)))
	mux.Handle("/rerun", gziphandler.GzipHandler(handleRerun(kc)))
	mux.Handle("/config", gziphandler.GzipHandler(handleConfig(configAgent)))
	mux.Handle("/branding.js", gziphandler.GzipHandler(handleBranding(configAgent)))

	if *hookURL != "" {
		mux.Handle("/plugin-help.js",
			gziphandler.GzipHandler(handlePluginHelp(newHelpAgent(*hookURL))))
	}

	if *tideURL != "" {
		ta := &tideAgent{
			log:  logger.WithField("agent", "tide"),
			path: *tideURL,
			updatePeriod: func() time.Duration {
				return configAgent.Config().Deck.TideUpdatePeriod
			},
			hiddenRepos: configAgent.Config().Deck.HiddenRepos,
			hiddenOnly:  *hiddenOnly,
		}
		ta.start()
		mux.Handle("/tide.js", gziphandler.GzipHandler(handleTide(configAgent, ta)))
	}

	// Enable Git OAuth feature if oauthUrl is provided.
	if *oauthUrl != "" {
		githubOAuthConfigRaw, err := loadToken(*githubOAuthConfigFile)
		if err != nil {
			logger.WithError(err).Fatal("Could not read github oauth config file.")
		}

		cookieSecretRaw, err := loadToken(*cookieSecretFile)
		if err != nil {
			logger.WithError(err).Fatal("Could not read cookie secret file.")
		}

		mux.Handle("/user-data.js", handleUserData(*oauthUrl))

		var githubOAuthConfig config.GithubOAuthConfig
		if err := yaml.Unmarshal(githubOAuthConfigRaw, &githubOAuthConfig); err != nil {
			logger.WithError(err).Fatal("Error unmarshalling github oauth config")
		}
		if !isValidatedGitOAuthConfig(&githubOAuthConfig) {
			logger.Fatal("Error invalid github oauth config")
		}
		if err := githubOAuthConfig.Decode(); err != nil {
			logger.WithError(err).Fatal("Error with decoding git oauth config")
		}

		var cookieSecret config.Cookie
		if err := yaml.Unmarshal(cookieSecretRaw, &cookieSecret); err != nil {
			logger.WithError(err).Fatal("Error unmarshalling cookie secret")
		}
		decodedSecret, err := hex.DecodeString(cookieSecret.Secret)
		if err != nil {
			logger.WithError(err).Fatal("Error decoding cookie secret")
		}
		if len(decodedSecret) == 0 {
			logger.Fatal("Cookie secret should not be empty")
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

		userDashboardAgent := userdashboard.NewDashboardAgent(&githubOAuthConfig, logrus.WithField("client", "user-dashboard"))
		mux.Handle("/user-dashboard", userDashboardAgent.HandleUserDashboard(userDashboardAgent))
		// Handles login request.
		mux.Handle("/user-dashboard/login", goa.HandleLogin(oauthClient))
		// Handles redirect from Github OAuth server.
		mux.Handle("/user-dashboard/redirect", goa.HandleRedirect(oauthClient))
	}

	// optionally inject http->https redirect handler when behind loadbalancer
	if *redirectHTTPTo != "" {
		redirectMux := http.NewServeMux()
		redirectMux.Handle("/", func(oldMux *http.ServeMux, host string) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("x-forwarded-proto") == "http" {
					redirectURL, err := url.Parse(r.URL.String())
					if err != nil {
						logger.Errorf("Failed to parse URL: %s.", r.URL.String())
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
		}(mux, *redirectHTTPTo))
		mux = redirectMux
	}
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

// serve with handler but map extensionless URLs to to the default
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

func handleProwJobs(ja *JobAgent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setHeadersNoCaching(w)
		jobs := ja.ProwJobs()
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

func handleData(ja *JobAgent) http.HandlerFunc {
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

func handleUserData(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setHeadersNoCaching(w)
		userData, err := getUserDashboardData(path)
		if err != nil {
			logrus.WithError(err).Error("Getting data from deck")
			userData = userdashboard.UserData{}
		}
		marshalData, err := json.Marshal(userData)
		if err != nil {
			logrus.WithError(err).Error("Marshaling user data")
			marshalData = []byte("[]")
		}
		if v := r.URL.Query().Get("var"); v != "" {
			fmt.Fprintf(w, "var %s = %s;", v, string(marshalData))
		} else {
			fmt.Fprint(w, string(marshalData))
		}
	}
}

type logClient interface {
	GetJobLog(job, id string) ([]byte, error)
	// Add ability to stream logs with options enabled. This call is used to follow logs
	// using kubernetes client API. All other options on the Kubernetes log api can
	// also be enabled.
	GetJobLogStream(job, id string, options map[string]string) (io.ReadCloser, error)
}

func httpChunking(log io.ReadCloser, w http.ResponseWriter) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		logrus.Warning("Error getting flusher.")
	}
	reader := bufio.NewReader(log)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			// TODO(rmmh): The log stops streaming after 30s.
			// This seems to be an apiserver limitation-- investigate?
			// logrus.WithError(err).Error("chunk failed to read!")
			break
		}
		w.Write(line)
		if flusher != nil {
			flusher.Flush()
		}
	}
}

func getOptions(values url.Values) map[string]string {
	options := make(map[string]string)
	for k, v := range values {
		if k != "pod" && k != "job" && k != "id" {
			options[k] = v[0]
		}
	}
	return options
}

// TODO(spxtr): Cache, rate limit.
func handleLog(lc logClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setHeadersNoCaching(w)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		job := r.URL.Query().Get("job")
		id := r.URL.Query().Get("id")
		stream := r.URL.Query().Get("follow")
		var logStreamRequested bool
		if ok, _ := strconv.ParseBool(stream); ok {
			// get http chunked responses to the client
			w.Header().Set("Connection", "Keep-Alive")
			w.Header().Set("Transfer-Encoding", "chunked")
			logStreamRequested = true
		}
		if err := validateLogRequest(r); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !logStreamRequested {
			log, err := lc.GetJobLog(job, id)
			if err != nil {
				http.Error(w, fmt.Sprintf("Log not found: %v", err), http.StatusNotFound)
				logrus.WithError(err).Warning("Error returned.")
				return
			}
			if _, err = w.Write(log); err != nil {
				logrus.WithError(err).Warning("Error writing log.")
			}
		} else {
			//run http chunking
			options := getOptions(r.URL.Query())
			log, err := lc.GetJobLogStream(job, id, options)
			if err != nil {
				http.Error(w, fmt.Sprintf("Log stream caused: %v", err), http.StatusNotFound)
				logrus.WithError(err).Warning("Error returned.")
				return
			}
			httpChunking(log, w)
		}
	}
}

func validateLogRequest(r *http.Request) error {
	job := r.URL.Query().Get("job")
	id := r.URL.Query().Get("id")

	if job == "" {
		return errors.New("Missing job query")
	}
	if id == "" {
		return errors.New("Missing ID query")
	}
	if !objReg.MatchString(job) {
		return fmt.Errorf("Invalid job query: %s", job)
	}
	if !objReg.MatchString(id) {
		return fmt.Errorf("Invalid ID query: %s", id)
	}
	return nil
}

type pjClient interface {
	GetProwJob(string) (kube.ProwJob, error)
}

func handleRerun(kc pjClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("prowjob")
		if !objReg.MatchString(name) {
			http.Error(w, "Invalid ProwJob query", http.StatusBadRequest)
			return
		}
		pj, err := kc.GetProwJob(name)
		if err != nil {
			http.Error(w, fmt.Sprintf("ProwJob not found: %v", err), http.StatusNotFound)
			logrus.WithError(err).Warning("Error returned.")
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

func handleConfig(ca configAgent) http.HandlerFunc {
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

func handleBranding(ca configAgent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setHeadersNoCaching(w)
		config := ca.Config()
		b, err := json.Marshal(config.Deck.Branding)
		if err != nil {
			logrus.WithError(err).Error("Error marshaling branding config.")
			http.Error(w, "Failed to marhshal branding config.", http.StatusInternalServerError)
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

func isValidatedGitOAuthConfig(githubOAuthConfig *config.GithubOAuthConfig) bool {
	return githubOAuthConfig.ClientID != "" && githubOAuthConfig.ClientSecret != "" &&
		githubOAuthConfig.RedirectURL != "" &&
		githubOAuthConfig.FinalRedirectURL != ""
}

func getUserDashboardData(path string) (userdashboard.UserData, error) {
	resp, err := http.Get(path)
	if err != nil {
		return userdashboard.UserData{}, fmt.Errorf("error GETing user dashboard data: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return userdashboard.UserData{}, fmt.Errorf("response has status code %d", resp.StatusCode)
	}
	var data userdashboard.UserData
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return userdashboard.UserData{}, fmt.Errorf("error decoding json user dashboard data: %v", err)
	}

	return data, nil
}
