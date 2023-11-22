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
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"

	"k8s.io/test-infra/gcsweb/pkg/version"
	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/flagutil"
	pkgio "k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/io/providers"

	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pjutil"
)

const (
	// path for GCS browsing on this server
	gcsPath = "gcs"

	// The base URL for GCP's GCS browser.
	gcsBrowserURL = "https://console.cloud.google.com/storage/browser"

	iconFile = "/icons/file.png"
	iconDir  = "/icons/dir.png"
	iconBack = "/icons/back.png"
)

//go:embed icons/* styles/*
var embededStatic embed.FS

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

	flIcons  string
	flStyles string
	flagutil.StorageClientOptions
	oauthTokenFile string
	// deprecated and ineffective
	defaultCredentials bool

	flVersion bool

	// Only buckets in this list will be served.
	allowedBuckets strslice
	// allowedProwPaths is the parsed list of allowedBuckets
	allowedProwPaths []*prowv1.ProwPath

	instrumentationOptions flagutil.InstrumentationOptions
}

var flUpgradeProxiedHTTPtoHTTPS bool

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	fs.IntVar(&o.flPort, "p", 8080, "port number on which to listen")

	fs.StringVar(&o.flIcons, "i", "", "path to the icons directory")
	fs.StringVar(&o.flStyles, "s", "", "path to the styles directory")

	o.StorageClientOptions.AddFlags(fs)
	fs.StringVar(&o.oauthTokenFile, "oauth-token-file", "", "Path to the file containing the OAuth 2.0 Bearer Token secret.")
	// StorageClientOptions.StorageClient / io.NewOpener automatically uses application default credentials as a fallback.
	// Mark this flag as ineffective but don't remove it for backward-compatibility.
	fs.BoolVar(&o.defaultCredentials, "use-default-credentials", false, "Use application default credentials "+
		"(deprecated and ineffective, is assumed to be true if --gcs-credentials-file is not set)")

	fs.BoolVar(&o.flVersion, "version", false, "print version and exit")
	fs.BoolVar(&flUpgradeProxiedHTTPtoHTTPS, "upgrade-proxied-http-to-https", false, "upgrade any proxied request (e.g. from GCLB) from http to https")

	fs.Var(&o.allowedBuckets, "b", "Buckets to serve (may be specified more than once). Can be GCS buckets (gs:// prefix) or S3 buckets (s3:// prefix).\n"+
		"If the bucket doesn't have a prefix, gs:// is assumed (deprecated, add the gs:// prefix).")

	o.instrumentationOptions.AddFlags(fs)

	fs.Parse(os.Args[1:])
	return o
}

func (o *options) validate() error {
	// validate and parse bucket list
	for _, bucket := range o.allowedBuckets {
		// canonicalize buckets: adds the gs:// prefix if omitted
		prowPath, err := prowv1.ParsePath(bucket)
		if err != nil {
			return fmt.Errorf("bucket %q is not a valid bucket: %w", bucket, err)
		}

		o.allowedProwPaths = append(o.allowedProwPaths, prowPath)
	}

	if o.flIcons != "" {
		if _, err := os.Stat(o.flIcons); os.IsNotExist(err) {
			return fmt.Errorf("icons path %q doesn't exist", o.flIcons)
		}
	}
	if o.flStyles != "" {
		if _, err := os.Stat(o.flStyles); os.IsNotExist(err) {
			return fmt.Errorf("styles path %q doesn't exist", o.flStyles)
		}
	}
	if o.oauthTokenFile != "" && o.GCSCredentialsFile != "" {
		return errors.New("specifying both --oauth-token-file and --gcs-credentials-file is not allowed")
	}
	if o.oauthTokenFile != "" && o.S3CredentialsFile != "" {
		return errors.New("specifying both --oauth-token-file and --s3-credentials-file is not allowed")
	}

	if o.oauthTokenFile != "" {
		if _, err := os.Stat(o.oauthTokenFile); os.IsNotExist(err) {
			return fmt.Errorf("oauth token file %q doesn't exist", o.oauthTokenFile)
		}
	}

	if o.GCSCredentialsFile != "" {
		if _, err := os.Stat(o.GCSCredentialsFile); os.IsNotExist(err) {
			return fmt.Errorf("gcs crendentials file %q doesn't exist", o.GCSCredentialsFile)
		}
	}

	if o.S3CredentialsFile != "" {
		if _, err := os.Stat(o.S3CredentialsFile); os.IsNotExist(err) {
			return fmt.Errorf("s3 crendentials file %q doesn't exist", o.S3CredentialsFile)
		}
	}

	return nil
}

func getStorageClient(o options) (pkgio.Opener, error) {
	ctx := context.Background()

	// prow/io doesn't support oauth token files, gcsweb is the only component supporting this.
	// If --oauth-token-file is set create a storage client manually and pass it to NewGCSOpener.
	if o.oauthTokenFile != "" {
		b, err := os.ReadFile(o.oauthTokenFile)
		if err != nil {
			return nil, fmt.Errorf("error reading oauth token file %s: %w", o.oauthTokenFile, err)
		}

		storageClient, err := storage.NewClient(ctx, option.WithAPIKey(string(bytes.TrimSpace(b))))
		if err != nil {
			return nil, fmt.Errorf("couldn't create the gcs storage client: %w", err)
		}

		return pkgio.NewGCSOpener(storageClient), nil
	}

	return o.StorageClient(ctx)
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

	mux := http.NewServeMux()

	// Handle allowed buckets.
	for _, prowPath := range o.allowedProwPaths {
		prefix := pathPrefix(prowPath)

		logrus.WithField("bucket", prowPath.BucketWithScheme()).Info("allowing bucket")
		mux.HandleFunc(prefix+"/", s.storageRequest)
		mux.HandleFunc(prefix, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, prefix+"/", http.StatusPermanentRedirect)
		})
	}

	// Handle unknown GCS buckets.
	mux.HandleFunc("/gcs/", unknownGCSBucketRequest)

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

	if o.flIcons != "" { // If user specifies custom icons path then read it at runtime
		mux.Handle("/icons/", longCacheServer(http.StripPrefix("/icons/", http.FileServer(http.Dir(o.flIcons)))))
	} else {
		mux.Handle("/icons/", longCacheServer(http.FileServer(http.FS(embededStatic))))
	}
	if o.flStyles != "" { // If user specifies custom styles path then read it at runtime
		mux.Handle("/styles/", longCacheServer(http.StripPrefix("/styles/", http.FileServer(http.Dir(o.flStyles)))))
	} else {
		mux.Handle("/styles/", longCacheServer(http.FileServer(http.FS(embededStatic))))
	}

	// Serve HTTP.
	mux.HandleFunc("/robots.txt", robotsRequest)
	mux.HandleFunc("/", otherRequest)

	health := pjutil.NewHealthOnPort(o.instrumentationOptions.HealthPort)
	health.ServeReady()

	logrus.Infof("serving on port %d", o.flPort)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", o.flPort), mux); err != nil {
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

func unknownGCSBucketRequest(w http.ResponseWriter, r *http.Request) {
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
		path := strings.TrimPrefix(r.URL.Path, "/"+gcsPath)
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
	storageClient pkgio.Opener
}

type objectHeaders struct {
	contentType        string
	contentEncoding    string
	contentDisposition string
	contentLanguage    string
}

func (s *server) handleObject(w http.ResponseWriter, prowPath *prowv1.ProwPath, headers objectHeaders) error {
	objReader, err := s.storageClient.Reader(context.Background(), prowPath.String())
	if err != nil {
		return fmt.Errorf("couldn't create the object reader: %w", err)
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
		return fmt.Errorf("coudln't copy data to the response writer: %w", err)
	}

	return nil
}

func (s *server) handleDirectory(w http.ResponseWriter, prowPath *prowv1.ProwPath, path string) error {
	// Get all object that exist in the parent folder only. We can do that by adding a
	// slash at the end of the prefix and use this as a delimiter in the gcs query.
	o, err := s.storageClient.Iterator(context.Background(), prowPath.String()+"/", "/")
	if err != nil {
		return fmt.Errorf("couldn't create the object iterator: %w", err)
	}

	var files []Record
	var dirs []Prefix

	for {
		objAttrs, err := o.Next(context.Background())
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error while processing object: %w", err)
		}

		if !objAttrs.IsDir {
			files = append(files, Record{
				Name:  objAttrs.ObjName,
				MTime: objAttrs.Updated,
				Size:  objAttrs.Size,
			})
			continue
		}

		dirs = append(dirs, Prefix{Prefix: fmt.Sprintf("%s/", filepath.Base(objAttrs.Name))})
	}

	dir := &gcsDir{
		ProwPath:       prowPath,
		Contents:       files,
		CommonPrefixes: dirs,
	}
	dir.Render(w, path)

	return nil
}

func (s *server) storageRequest(w http.ResponseWriter, r *http.Request) {
	logger := newTxnLogger(r)

	if upgradeToHTTPS(w, r, logger) {
		return
	}
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	path := r.URL.Path
	prowPath, err := parsePath(path)
	if err != nil {
		logger.WithError(err).Error("error parsing path")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Error: %v", err)
		return
	}

	objectLogger := logger.WithFields(logrus.Fields{"bucket": prowPath.BucketWithScheme(), "object": strings.Trim(prowPath.Path, "/")})

	objectLogger.Info("Processing request...")
	// Getting the object attributes directly will determine if is a folder or a file.
	objAttrs, err := s.storageClient.Attributes(context.Background(), prowPath.String())

	// This means that the object is a file.
	if err == nil {
		headers := objectHeaders{
			contentType:        objAttrs.ContentType,
			contentEncoding:    objAttrs.ContentEncoding,
			contentDisposition: objAttrs.ContentDisposition,
			contentLanguage:    objAttrs.ContentLanguage,
		}

		if err := s.handleObject(w, prowPath, headers); err != nil {
			objectLogger.WithError(err).Error("error while handling object")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Error: %v", err)
			return
		}
	} else {
		err := s.handleDirectory(w, prowPath, path)
		if err != nil {
			objectLogger.WithError(err).Error("error while handling objects")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Error: %v", err)
			return
		}
	}
}

func providerPrefix(provider string) string {
	// rewrite /gs to legacy /gcs path for compatibility
	if provider == providers.GS {
		provider = gcsPath
	}

	return "/" + provider
}

func pathPrefix(prowPath *prowv1.ProwPath) string {
	return joinPath(providerPrefix(prowPath.StorageProvider()), prowPath.Bucket())
}

func parsePath(path string) (*prowv1.ProwPath, error) {
	// e.g. "/gcs/bucket/path/to/object/" -> "gcs/bucket/path/to/object"
	path = strings.Trim(path, "/")

	// e.g. "gcs/bucket/path/to/object" -> "gs://bucket/path/to/object"
	// e.g. "s3/bucket/path/to/object" -> "s3://bucket/path/to/object"
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 {
		// "/gcs/bucket" is valid, "/gcs/" is invalid
		return nil, fmt.Errorf("invalid path: expected at least 1 slash: %s", path)
	}

	provider := parts[0]
	if provider == "gcs" {
		// rewrite legacy /gcs path to gs provider for compatibility
		provider = providers.GS
	}

	return prowv1.ParsePath(fmt.Sprintf("%s://%s", provider, parts[1]))
}

// joinPath joins a set of path elements, but does not remove duplicate /
// characters, making it URL-safe.
func joinPath(paths ...string) string {
	return strings.Join(paths, "/")
}

// getParent is basically path.Dir but handles two special cases for gcsweb:
// - it treats paths with and without trailing slash equally, e.g.: /gcs/foo/bar/ -> /gcs/foo/ and /gcs/foo/bar -> /gcs/foo/
// - it returns the empty string for the bucket root, e.g.: /gcs/foo -> ""
func getParent(inPath string) string {
	parent := path.Dir(strings.TrimSuffix(inPath, "/"))
	if strings.Count(parent, "/") >= 2 {
		return parent + "/"
	}
	// inPath is bucket root
	return ""
}

// gcsDir represents a bucket in GCS, decoded from XML.
type gcsDir struct {
	ProwPath       *prowv1.ProwPath
	Marker         string
	NextMarker     string
	Contents       []Record
	CommonPrefixes []Prefix
}

// Render writes HTML representing this gcsDir to the provided output.
func (dir *gcsDir) Render(out http.ResponseWriter, inPath string) {
	htmlPageHeader(out, providers.DisplayName(dir.ProwPath.StorageProvider()), dir.ProwPath.Bucket())

	if !strings.HasSuffix(inPath, "/") {
		inPath += "/"
	}

	htmlContentHeader(out, dir.ProwPath.Bucket(), strings.TrimPrefix(inPath, providerPrefix(dir.ProwPath.StorageProvider())))

	if dir.NextMarker != "" {
		htmlNextButton(out, inPath, dir.NextMarker)
	}

	htmlGridHeader(out)
	if parent := getParent(inPath); parent != "" {
		htmlGridItem(out, iconBack, parent, "..", "-", "-")
	}
	for i := range dir.CommonPrefixes {
		dir.CommonPrefixes[i].Render(out, inPath)
	}
	for i := range dir.Contents {
		dir.Contents[i].Render(out, inPath)
	}

	if dir.NextMarker != "" {
		htmlNextButton(out, inPath, dir.NextMarker)
	}

	htmlContentFooter(out, dir.ProwPath)

	htmlPageFooter(out)
}

// Record represents a single "Contents" entry in a GCS bucket.
type Record struct {
	Name  string
	MTime time.Time
	Size  int64
}

// Render writes HTML representing this Record to the provided output.
func (rec *Record) Render(out http.ResponseWriter, inPath string) {
	htmlGridItem(
		out,
		iconFile,
		inPath+rec.Name,
		rec.Name,
		fmt.Sprintf("%v", rec.Size),
		rec.MTime.Format(time.RFC1123),
	)
}

// Prefix represents a single "CommonPrefixes" entry in a GCS bucket.
type Prefix struct {
	Prefix string
}

// Render writes HTML representing this Prefix to the provided output.
func (pfx *Prefix) Render(out http.ResponseWriter, inPath string) {
	url := inPath + pfx.Prefix
	htmlGridItem(out, iconDir, url, pfx.Prefix, "-", "-")
}

func newTxnLogger(r *http.Request) *logrus.Entry {
	return logrus.WithFields(logrus.Fields{
		"txn":      fmt.Sprintf("%08x", rand.Int31()),
		"method":   r.Method,
		"url-path": r.URL.Path,
	})
}
