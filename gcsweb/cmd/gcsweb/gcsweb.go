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
	"log"
	"math/rand"
	"net/http"
	net_url "net/url"
	"os"
	"strings"
	"time"

	"k8s.io/test-infra/gcsweb/pkg/version"
)

// The base URL for GCS's HTTP API.
const gcsBaseURL = "https://storage.googleapis.com"
const gcsPath = "/gcs" // path for GCS browsing on this server

// The base URL for GCP's GCS browser.
const gcsBrowserURL = "https://console.cloud.google.com/storage/browser"

var flPort = flag.Int("p", 8080, "port number on which to listen")
var flIcons = flag.String("i", "/icons", "path to the icons directory")
var flStyles = flag.String("s", "/styles", "path to the styles directory")
var flVersion = flag.Bool("version", false, "print version and exit")
var flUpgradeProxiedHTTPtoHTTPS = flag.Bool("upgrade-proxied-http-to-https", false,
	"upgrade any proxied request (e.g. from GCLB) from http to https")

const (
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

// Only buckets in this list will be served.
var allowedBuckets strslice

func main() {
	flag.Var(&allowedBuckets, "b", "GCS bucket to serve (may be specified more than once)")
	flag.Parse()

	if *flVersion {
		fmt.Println(version.VERSION)
		os.Exit(0)
	}

	log.Printf("starting")
	rand.Seed(time.Now().UTC().UnixNano())

	// Canonicalize allowed buckets.
	for i := range allowedBuckets {
		bucket := joinPath(gcsPath, allowedBuckets[i])
		log.Printf("allowing %s", bucket)
		http.HandleFunc(bucket+"/", gcsRequest)
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
	http.Handle("/icons/", longCacheServer(http.StripPrefix("/icons/", http.FileServer(http.Dir(*flIcons)))))
	http.Handle("/styles/", longCacheServer(http.StripPrefix("/styles/", http.FileServer(http.Dir(*flStyles)))))

	// Serve HTTP.
	http.HandleFunc("/healthz", healthzRequest)
	http.HandleFunc("/robots.txt", robotsRequest)
	http.HandleFunc("/", otherRequest)
	log.Printf("serving on port %d", *flPort)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *flPort), nil))
}

func upgradeToHTTPS(w http.ResponseWriter, r *http.Request, logger txnLogger) bool {
	if *flUpgradeProxiedHTTPtoHTTPS && r.Header.Get("X-Forwarded-Proto") == "http" {
		newURL := *r.URL
		newURL.Scheme = "https"
		if newURL.Host == "" {
			newURL.Host = r.Host
		}
		logger.Printf("redirect to %s [https upgrade]", newURL.String())
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

func gcsRequest(w http.ResponseWriter, r *http.Request) {
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

	if markers, found := r.URL.Query()["marker"]; found {
		url += "&marker=" + markers[0]
	}

	resp, err := http.Get(url)
	if err != nil {
		logger.Printf("GET %s: %s", url, err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "http.Get: %v", err)
		return
	}
	defer resp.Body.Close()

	logger.Printf("GET %s: %s", url, resp.Status)

	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(resp.StatusCode)
		return
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logger.Printf("ioutil.ReadAll: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "ioutil.ReadAll: %v", err)
		return
	}
	dir, err := parseXML(body, object+"/")
	if err != nil {
		logger.Printf("xml.Unmarshal: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "xml.Unmarshal: %v", err)
		return
	}
	if dir == nil {
		// It was a request for a file, send them there directly.
		url := joinPath(gcsBaseURL, bucket, object)
		logger.Printf("redirect to %s", url)
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

// A logger-wrapper that logs a transaction's metadata.
type txnLogger struct {
	nonce string
}

// Printf logs a formatted line to the logging output.
func (tl txnLogger) Printf(fmt string, args ...interface{}) {
	args = append([]interface{}{tl.nonce}, args...)
	log.Printf("[txn-%s] "+fmt, args...)
}

func newTxnLogger(r *http.Request) txnLogger {
	nonce := fmt.Sprintf("%08x", rand.Int31())
	logger := txnLogger{nonce}
	logger.Printf("request: %s %s", r.Method, r.URL.Path)
	return logger
}
