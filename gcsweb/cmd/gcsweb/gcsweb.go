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
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"k8s.io/test-infra/gcsweb/pkg/version"

	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pjutil"
)

const (
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

	flIcons            string
	flStyles           string
	oauthTokenFile     string
	gcsCredentialsFile string

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
	fs.StringVar(&o.gcsCredentialsFile, "gcs-credentials-file", "", "Path to the file containing the gcs service account credentials.")

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
	if o.oauthTokenFile != "" && o.gcsCredentialsFile != "" {
		return errors.New("specifying both --oauth-token-file and --gcs-credentials-file is not allowed.")
	}

	if o.oauthTokenFile != "" {
		if _, err := os.Stat(o.oauthTokenFile); os.IsNotExist(err) {
			return fmt.Errorf("oauth token file '%s' doesn't exists.", o.oauthTokenFile)
		}
	}

	if o.gcsCredentialsFile != "" {
		if _, err := os.Stat(o.gcsCredentialsFile); os.IsNotExist(err) {
			return fmt.Errorf("gcs service account crendentials file '%s' doesn't exists.", o.gcsCredentialsFile)
		}
	}

	return nil
}

func getStorageClient(o options) (*storage.Client, error) {
	ctx := context.Background()
	clientOption := option.WithoutAuthentication()

	if o.oauthTokenFile != "" {
		b, err := ioutil.ReadFile(o.oauthTokenFile)
		if err != nil {
			return nil, fmt.Errorf("error reading oauth token file %s: %v", o.oauthTokenFile, err)
		}
		clientOption = option.WithAPIKey(string(bytes.TrimSpace(b)))
	}

	if o.gcsCredentialsFile != "" {
		clientOption = option.WithCredentialsFile(o.gcsCredentialsFile)
	}

	storageClient, err := storage.NewClient(ctx, clientOption)
	if err != nil {
		return nil, fmt.Errorf("couldn't create the gcs storage client: %v", err)
	}

	return storageClient, nil
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

	storageClient, err := getStorageClient(o)
	if err != nil {
		logrus.WithError(err).Fatal("couldn't get storage client")
	}

	s := &server{storageClient: storageClient}

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
	http.HandleFunc("/robots.txt", robotsRequest)
	http.HandleFunc("/", otherRequest)

	health := pjutil.NewHealth()
	health.ServeReady()

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
	storageClient *storage.Client
}

type objectHeaders struct {
	contentType        string
	contentEncoding    string
	contentDisposition string
	contentLanguage    string
}

func (s *server) handleObject(w http.ResponseWriter, bucket, object string, headers objectHeaders) error {
	obj := s.storageClient.Bucket(bucket).Object(object)

	objReader, err := obj.NewReader(context.Background())
	if err != nil {
		return fmt.Errorf("couldn't create the object reader: %v", err)
	}
	defer objReader.Close()

	if headers.contentType != "" {
		if headers.contentEncoding != "" {
			w.Header().Set("Content-Type", fmt.Sprintf("%s; charset=%s", headers.contentType, headers.contentEncoding))
		} else {
			w.Header().Set("Content-Type", headers.contentType)
		}
	}

	if headers.contentDisposition != "" {
		w.Header().Set("Content-Disposition", headers.contentDisposition)
	}
	if headers.contentLanguage != "" {
		w.Header().Set("Content-Language", headers.contentLanguage)
	}

	if _, err := io.Copy(w, objReader); err != nil {
		return fmt.Errorf("coudln't copy data to the response writer: %v", err)
	}

	return nil
}

func (s *server) handleDirectory(w http.ResponseWriter, bucket, object, path string) error {
	// Get all object that exist in the parent folder only. We can do that by adding a
	// slash at the end of the prefix and use this as a delimiter in the gcs query.
	prefix := object + "/"
	o := s.storageClient.Bucket(bucket).Objects(context.Background(), &storage.Query{
		Delimiter: "/",
		Prefix:    prefix,
	})

	var files []Record
	var dirs []Prefix

	for {
		objAttrs, err := o.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("error while processing object: %v", err)
		}

		// That means that the object is a file
		if objAttrs.Name != "" {
			files = append(files, Record{
				Name:  filepath.Base(objAttrs.Name),
				MTime: objAttrs.Updated,
				Size:  objAttrs.Size,
			})
			continue
		}

		dirs = append(dirs, Prefix{Prefix: fmt.Sprintf("%s/", filepath.Base(objAttrs.Prefix))})
	}

	dir := &gcsDir{
		Name:           bucket,
		Prefix:         prefix,
		Contents:       files,
		CommonPrefixes: dirs,
	}
	dir.Render(w, path)

	return nil
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
	objectLogger := logger.WithFields(logrus.Fields{"bucket": bucket, "object": object})

	objectLogger.Info("Processing request...")
	// Getting the object attributes directly will determine if is a folder or a file.
	objAttrs, _ := s.storageClient.Bucket(bucket).Object(object).Attrs(context.Background())

	// This means that the object is a file.
	if objAttrs != nil {
		headers := objectHeaders{
			contentType:        objAttrs.ContentType,
			contentEncoding:    objAttrs.ContentEncoding,
			contentDisposition: objAttrs.ContentDisposition,
			contentLanguage:    objAttrs.ContentLanguage,
		}

		if err := s.handleObject(w, bucket, object, headers); err != nil {
			objectLogger.WithError(err).Error("error while handling object")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Error: %v", err)
			return
		}
	} else {
		err := s.handleDirectory(w, bucket, object, path)
		if err != nil {
			objectLogger.WithError(err).Error("error while handling objects")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Error: %v", err)
			return
		}
	}
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

// gcsDir represents a bucket in GCS, decoded from XML.
type gcsDir struct {
	Name           string
	Prefix         string
	Marker         string
	NextMarker     string
	Contents       []Record
	CommonPrefixes []Prefix
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
	Name  string
	MTime time.Time
	Size  int64
	isDir bool
}

// Render writes HTML representing this Record to the provided output.
func (rec *Record) Render(out http.ResponseWriter, inPath string) {
	htmlGridItem(
		out,
		iconFile,
		gcsPath+inPath+rec.Name,
		rec.Name,
		fmt.Sprintf("%v", rec.Size),
		rec.MTime.Format("01 Jan 2006 15:04:05"),
	)
}

// Prefix represents a single "CommonPrefixes" entry in a GCS bucket.
type Prefix struct {
	Prefix string
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
