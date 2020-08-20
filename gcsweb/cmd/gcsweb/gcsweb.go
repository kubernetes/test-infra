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
	"encoding/xml"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	net_url "net/url"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/gcsweb/pkg/version"

	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/prow/logrusutil"
)

const (
	// The base URL for GCS's HTTP API.
	gcsBaseURL = "https://storage.googleapis.com"

	// path for GCS browsing on this server
	gcsPath = "/gcs"

	// The base URL for GCP's GCS browser.
	gcsBrowserURL = "https://console.cloud.google.com/storage/browser"

	iconFile = "/icons/file.png"
	iconDir  = "/icons/dir.png"
	iconBack = "/icons/back.png"
)

type strslice []string

// String prints the strlice as a string.
func (ss *strslice) String() string {
	return fmt.Sprintf("%v", *ss)
}

// Set appends a value onto the strslice.
func (ss *strslice) Set(value string) error {
	*ss = append(*ss, value)
	return nil
}

type options struct {
	flPort int

	flIcons        string
	flStyles       string
	oauthTokenFile string

	flVersion bool

	// Only buckets in this list will be served.
	allowedBuckets strslice
}

var flUpgradeProxiedHTTPtoHTTPS bool

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	fs.IntVar(&o.flPort, "p", 8080, "port number on which to listen")

	fs.StringVar(&o.flIcons, "i", "/icons", "path to the icons directory")
	fs.StringVar(&o.flStyles, "s", "/styles", "path to the styles directory")
	fs.StringVar(&o.oauthTokenFile, "oauth-token-file", "", "Path to the file containing the OAuth 2.0 Bearer Token secret.")

	fs.BoolVar(&o.flVersion, "version", false, "print version and exit")
	fs.BoolVar(&flUpgradeProxiedHTTPtoHTTPS, "upgrade-proxied-http-to-https", false, "upgrade any proxied request (e.g. from GCLB) from http to https")

	fs.Var(&o.allowedBuckets, "b", "GCS bucket to serve (may be specified more than once)")

	fs.Parse(os.Args[1:])
	return o
}

func (o *options) validate() error {
	if _, err := os.Stat(o.flIcons); os.IsNotExist(err) {
		return fmt.Errorf("icons path '%s' doesn't exists.", o.flIcons)
	}
	if _, err := os.Stat(o.flStyles); os.IsNotExist(err) {
		return fmt.Errorf("styles path '%s' doesn't exists.", o.flStyles)
	}
	if o.oauthTokenFile != "" {
		if _, err := os.Stat(o.oauthTokenFile); os.IsNotExist(err) {
			return fmt.Errorf("oauth token file '%s' doesn't exists.", o.oauthTokenFile)
		}
	}
	return nil
}

func main() {
	o := gatherOptions()
	if err := o.validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	if o.flVersion {
		fmt.Println(version.VERSION)
		os.Exit(0)
	}

	logrusutil.ComponentInit()

	s := &server{
		httpClient: &http.Client{},
	}

	if o.oauthTokenFile != "" {
		secretAgent := &secret.Agent{}
		if err := secretAgent.Start([]string{o.oauthTokenFile}); err != nil {
			logrus.WithError(err).Fatal("Error starting secrets agent.")
		}
		s.tokenGenerator = secretAgent.GetTokenGenerator(o.oauthTokenFile)
	}

	logrus.Info("Starting GCSWeb")
	rand.Seed(time.Now().UTC().UnixNano())

	// Canonicalize allowed buckets.
	for i := range o.allowedBuckets {
		bucket := joinPath(gcsPath, o.allowedBuckets[i])
		logrus.WithField("bucket", bucket).Info("allowing bucket")
		http.HandleFunc(bucket+"/", s.gcsRequest)
		http.HandleFunc(bucket, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, bucket+"/", http.StatusPermanentRedirect)
		})
	}
	// Handle unknown buckets.
	http.HandleFunc("/gcs/", unknownBucketRequest)

	// Serve icons and styles.
	longCacheServer := func(h http.Handler) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if upgradeToHTTPS(w, r, newTxnLogger(r)) {
				return
			}
			// Mark as never expiring as per https://www.ietf.org/rfc/rfc2616.txt
			w.Header().Add("Cache-Control", "max-age=31536000")
			h.ServeHTTP(w, r)
		}
	}
	http.Handle("/icons/", longCacheServer(http.StripPrefix("/icons/", http.FileServer(http.Dir(o.flIcons)))))
	http.Handle("/styles/", longCacheServer(http.StripPrefix("/styles/", http.FileServer(http.Dir(o.flStyles)))))

	// Serve HTTP.
	http.HandleFunc("/healthz", healthzRequest)
	http.HandleFunc("/robots.txt", robotsRequest)
	http.HandleFunc("/", otherRequest)

	logrus.Infof("serving on port %d", o.flPort)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", o.flPort), nil); err != nil {
		logrus.WithError(err).Fatal("couldn't start the http server")
	}
}

func upgradeToHTTPS(w http.ResponseWriter, r *http.Request, logger *logrus.Entry) bool {
	if flUpgradeProxiedHTTPtoHTTPS && r.Header.Get("X-Forwarded-Proto") == "http" {
		newURL := *r.URL
		newURL.Scheme = "https"
		if newURL.Host == "" {
			newURL.Host = r.Host
		}
		logger.Infof("redirect to %s [https upgrade]", newURL.String())
		http.Redirect(w, r, newURL.String(), http.StatusPermanentRedirect)
		return true
	}
	return false
}

func healthzRequest(w http.ResponseWriter, r *http.Request) {
	newTxnLogger(r)

	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	fmt.Fprintf(w, "ok")
}

func robotsRequest(w http.ResponseWriter, r *http.Request) {
	logger := newTxnLogger(r)

	if upgradeToHTTPS(w, r, logger) {
		return
	}
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	fmt.Fprintf(w, "User-agent: *\nDisallow: /\n")
}

func unknownBucketRequest(w http.ResponseWriter, r *http.Request) {
	logger := newTxnLogger(r)

	if upgradeToHTTPS(w, r, logger) {
		return
	}
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Assume that a / suffix means a directory, and redirect to
	// the official bucket browser for it.
	if strings.HasSuffix(r.URL.Path, "/") {
		// e.g. "/gcs/bucket/path/to/object" -> "/bucket/path/to/object"
		path := strings.TrimPrefix(r.URL.Path, gcsPath)
		http.Redirect(w, r, gcsBrowserURL+path, http.StatusSeeOther)
		return
	}

	http.NotFound(w, r)
}

func otherRequest(w http.ResponseWriter, r *http.Request) {
	logger := newTxnLogger(r)
	if upgradeToHTTPS(w, r, logger) {
		return
	}
	http.NotFound(w, r)
}

type server struct {
	httpClient     *http.Client
	tokenGenerator func() []byte
}

func (s *server) gcsRequest(w http.ResponseWriter, r *http.Request) {
	logger := newTxnLogger(r)

	if upgradeToHTTPS(w, r, logger) {
		return
	}
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// e.g. "/gcs/bucket/path/to/object" -> "/bucket/path/to/object"
	path := strings.TrimPrefix(r.URL.Path, gcsPath)
	// e.g. "/bucket/path/to/object" -> ["bucket", "path/to/object"]
	bucket, object := splitBucketObject(path)

	url := joinPath(gcsBaseURL, bucket)
	url += "?delimiter=/"

	if object != "" {
		// Adding the last slash forces the server to give me a clue about
		// whether the object is a file or a dir.  If it is a dir, the
		// contents will include a record for itself.  If it is a file it
		// will not.
		url += "&prefix=" + net_url.QueryEscape(object+"/")
	}

	markers, found := r.URL.Query()["marker"]
	if found {
		url += "&marker=" + markers[0]
	}

	urlLogger := logger.WithFields(logrus.Fields{
		"url":     url,
		"bucket":  bucket,
		"object":  object,
		"markers": markers})

	// Create a new request using http
	req, err := http.NewRequest(http.MethodGet, url, nil)

	if s.tokenGenerator != nil {
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", string(s.tokenGenerator())))
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		urlLogger.WithError(err).Error("failed to GET from GCS")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "http.Get: %v", err)
		return
	}
	defer resp.Body.Close()

	urlLogger.WithField("status", resp.Status).Info("URL processed")

	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(resp.StatusCode)
		return
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		urlLogger.WithError(err).Error("error while reading response body")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "ioutil.ReadAll: %v", err)
		return
	}
	dir, err := parseXML(body, object+"/")
	if err != nil {
		urlLogger.WithError(err).Error("error while unmarshaling the XML from response body")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "xml.Unmarshal: %v", err)
		return
	}
	if dir == nil {
		// It was a request for a file, send them there directly.
		url := joinPath(gcsBaseURL, bucket, object)
		urlLogger.Infof("redirect to %s", url)
		http.Redirect(w, r, url, http.StatusTemporaryRedirect)
		return
	}
	dir.Render(w, path)
}

// splitBucketObject breaks a path into the first part (the bucket), and
// everything else (the object).
func splitBucketObject(path string) (string, string) {
	path = strings.Trim(path, "/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 {
		return "", ""
	}
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

// joinPath joins a set of path elements, but does not remove duplicate /
// characters, making it URL-safe.
func joinPath(paths ...string) string {
	return strings.Join(paths, "/")
}

// dirname returns the logical parent directory of the path.  This is different
// than path.Split() in that we want dirname("foo/bar/") -> "foo/", whereas
// path.Split() returns "foo/bar/".
func dirname(path string) string {
	leading := ""
	if strings.HasPrefix(path, "/") {
		leading = "/"
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) > 1 {
		return leading + strings.Join(parts[0:len(parts)-1], "/") + "/"
	}
	return leading
}

// parseXML extracts a gcsDir object from XML.  If this returns a nil gcsDir,
// the XML indicated that this was not a directory at all.
func parseXML(body []byte, object string) (*gcsDir, error) {
	dir := new(gcsDir)
	if err := xml.Unmarshal(body, &dir); err != nil {
		return nil, err
	}
	// We think this is a dir if the object is "/" (just the bucket) or if we
	// find any Contents or CommonPrefixes.
	isDir := object == "/" || len(dir.Contents)+len(dir.CommonPrefixes) > 0
	selfIndex := -1
	for i := range dir.Contents {
		rec := &dir.Contents[i]
		name := strings.TrimPrefix(rec.Name, object)
		if name == "" {
			selfIndex = i
			continue
		}
		rec.Name = name
		if strings.HasSuffix(name, "/") {
			rec.isDir = true
		}
	}

	for i := range dir.CommonPrefixes {
		cp := &dir.CommonPrefixes[i]
		cp.Prefix = strings.TrimPrefix(cp.Prefix, object)
	}

	if !isDir {
		return nil, nil
	}

	if selfIndex >= 0 {
		// Strip out the record that indicates this object.
		dir.Contents = append(dir.Contents[:selfIndex], dir.Contents[selfIndex+1:]...)
	}
	return dir, nil
}

// gcsDir represents a bucket in GCS, decoded from XML.
type gcsDir struct {
	XMLName        xml.Name `xml:"ListBucketResult"`
	Name           string   `xml:"Name"`
	Prefix         string   `xml:"Prefix"`
	Marker         string   `xml:"Marker"`
	NextMarker     string   `xml:"NextMarker"`
	Contents       []Record `xml:"Contents"`
	CommonPrefixes []Prefix `xml:"CommonPrefixes"`
}

const tmplPageHeaderText = `
    <!doctype html>
   	<html>
   	<head>
   	    <link rel="stylesheet" type="text/css" href="/styles/style.css">
   	    <meta charset="utf-8">
   	    <meta name="viewport" content="width=device-width, initial-scale=1.0">
   	    <title>GCS browser: {{.Name}}</title>
		<style>
		header {
			margin-left: 10px;
		}

		.next-button {
			margin: 10px 0;
		}

		.grid-head {
			border-bottom: 1px solid black;
		}

		.resource-grid {
			margin-right: 20px;
		}

		li.grid-row:nth-child(even) {
			background-color: #ddd;
		}

		li div {
			box-sizing: border-box;
			border-left: 1px solid black;
			padding-left: 5px;
			overflow-wrap: break-word;
		}
		li div:first-child {
			border-left: none;
		}

		</style>
   	</head>
   	<body>
`

var tmplPageHeader = template.Must(template.New("page-header").Parse(tmplPageHeaderText))

func htmlPageHeader(out io.Writer, name string) error {
	args := struct {
		Name string
	}{
		Name: name,
	}
	return tmplPageHeader.Execute(out, args)
}

const tmplPageFooterText = `</body></html>`

var tmplPageFooter = template.Must(template.New("page-footer").Parse(tmplPageFooterText))

func htmlPageFooter(out io.Writer) error {
	return tmplPageFooter.Execute(out, struct{}{})
}

const tmplContentHeaderText = `
    <header>
        <h1>{{.DirName}}</h1>
        <h3>{{.Path}}</h3>
    </header>
    <ul class="resource-grid">
`

var tmplContentHeader = template.Must(template.New("content-header").Parse(tmplContentHeaderText))

func htmlContentHeader(out io.Writer, dirname, path string) error {
	args := struct {
		DirName string
		Path    string
	}{
		DirName: dirname,
		Path:    path,
	}
	return tmplContentHeader.Execute(out, args)
}

const tmplContentFooterText = `</ul>`

var tmplContentFooter = template.Must(template.New("content-footer").Parse(tmplContentFooterText))

func htmlContentFooter(out io.Writer) error {
	return tmplContentFooter.Execute(out, struct{}{})
}

const tmplNextButtonText = `
    <a href="{{.Path}}?marker={{.Marker}}"
	   class="pure-button next-button">
	   Next page
	</a>
`

var tmplNextButton = template.Must(template.New("next-button").Parse(tmplNextButtonText))

func htmlNextButton(out io.Writer, path, marker string) error {
	args := struct {
		Path   string
		Marker string
	}{
		Path:   path,
		Marker: marker,
	}
	return tmplNextButton.Execute(out, args)
}

const tmplGridHeaderText = `
	<li class="pure-g">
		<div class="pure-u-2-5 grid-head">Name</div>
		<div class="pure-u-1-5 grid-head">Size</div>
		<div class="pure-u-2-5 grid-head">Modified</div>
	</li>
`

var tmplGridHeader = template.Must(template.New("grid-header").Parse(tmplGridHeaderText))

func htmlGridHeader(out io.Writer) error {
	return tmplGridHeader.Execute(out, struct{}{})
}

const tmplGridItemText = `
    <li class="pure-g grid-row">
	    <div class="pure-u-2-5"><a href="{{.URL}}"><img src="{{.Icon}}"> {{.Name}}</a></div>
	    <div class="pure-u-1-5">{{.Size}}</div>
	    <div class="pure-u-2-5">{{.Modified}}</div>
	</li>
`

var tmplGridItem = template.Must(template.New("grid-item").Parse(tmplGridItemText))

func htmlGridItem(out io.Writer, icon, url, name, size, modified string) error {
	args := struct {
		URL      string
		Icon     string
		Name     string
		Size     string
		Modified string
	}{
		URL:      url,
		Icon:     icon,
		Name:     name,
		Size:     size,
		Modified: modified,
	}
	return tmplGridItem.Execute(out, args)
}

// Render writes HTML representing this gcsDir to the provided output.
func (dir *gcsDir) Render(out http.ResponseWriter, inPath string) {
	htmlPageHeader(out, dir.Name)

	if !strings.HasSuffix(inPath, "/") {
		inPath += "/"
	}

	htmlContentHeader(out, dir.Name, inPath)

	if dir.NextMarker != "" {
		htmlNextButton(out, gcsPath+inPath, dir.NextMarker)
	}

	htmlGridHeader(out)
	if parent := dirname(inPath); parent != "" {
		url := gcsPath + parent
		htmlGridItem(out, iconBack, url, "..", "-", "-")
	}
	for i := range dir.CommonPrefixes {
		dir.CommonPrefixes[i].Render(out, inPath)
	}
	for i := range dir.Contents {
		dir.Contents[i].Render(out, inPath)
	}

	if dir.NextMarker != "" {
		htmlNextButton(out, gcsPath+inPath, dir.NextMarker)
	}

	htmlContentFooter(out)

	htmlPageFooter(out)
}

// Record represents a single "Contents" entry in a GCS bucket.
type Record struct {
	Name  string `xml:"Key"`
	MTime string `xml:"LastModified"`
	Size  int64  `xml:"Size"`
	isDir bool
}

// Render writes HTML representing this Record to the provided output.
func (rec *Record) Render(out http.ResponseWriter, inPath string) {
	mtime := "<unknown>"
	ts, err := time.Parse(time.RFC3339, rec.MTime)
	if err == nil {
		mtime = ts.Format("02 Jan 2006 15:04:05")
	}
	var url, size string
	if rec.isDir {
		url = gcsPath + inPath + rec.Name
		size = "-"
	} else {
		url = gcsBaseURL + inPath + rec.Name
		size = fmt.Sprintf("%v", rec.Size)
	}
	htmlGridItem(out, iconFile, url, rec.Name, size, mtime)
}

// Prefix represents a single "CommonPrefixes" entry in a GCS bucket.
type Prefix struct {
	Prefix string `xml:"Prefix"`
}

// Render writes HTML representing this Prefix to the provided output.
func (pfx *Prefix) Render(out http.ResponseWriter, inPath string) {
	url := gcsPath + inPath + pfx.Prefix
	htmlGridItem(out, iconDir, url, pfx.Prefix, "-", "-")
}

func newTxnLogger(r *http.Request) *logrus.Entry {
	return logrus.WithFields(logrus.Fields{
		"txn":      fmt.Sprintf("%08x", rand.Int31()),
		"method":   r.Method,
		"url-path": r.URL.Path,
	})
}
