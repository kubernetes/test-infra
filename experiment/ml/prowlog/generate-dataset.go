/*
Copyright 2022 The Kubernetes Authors.

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

// Package main will process annotated builds listed in the tsv file.
package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"bitbucket.org/creachadair/stringset"
	"cloud.google.com/go/storage"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"google.golang.org/api/option"
)

var (
	annotations = flag.String("annotations", "", "path to annotations.tsv")
	output      = flag.String("output", "", "output classified lines to the this directory")
	maxLen      = flag.Int("max-length", 1000, "Truncate examples larger than this")
	minLines    = flag.Int("min-lines", 5, "Minimum lines per page")
	cache       = flag.String("cache", "", "Cache build content in the specified zip file.")
	skipResolve = flag.Bool("skip-resolve", false, "Do not resolve documents with different highlight ranges.")
	skipReplace = flag.Bool("skip-replace", false, "Do not replace annotations after resolving.")
	valSplit    = flag.Float64("validation-split", 0.2, "Reserve this many builds for the validation set")
	testSplit   = flag.Float64("test-split", 0, "Reserve this many builds for the test set")
)

func main() {
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var labels map[string]*stringset.Set
	var suffixes map[string]string
	var sources map[string]build
	labels, suffixes, sources = generateDataset(ctx)

	if err := sanityCheck(labels); err != nil {
		log.Fatalf("Sanity check fails: %v", err)
	}

	if err := zipLabels(ctx, *output, labels, suffixes, sources); err != nil {
		log.Fatalf("Failed to zip dataset: %v", err)
	}

	log.Println("Created", *output)
}

func generateDataset(ctx context.Context) (map[string]*stringset.Set, map[string]string, map[string]build) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var opts []option.ClientOption
	storageClient, err := storage.NewClient(ctx, opts...)
	if err != nil {
		log.Fatalf("create client: %v", err)
	}
	client := gcs.NewClient(storageClient)

	documents := make(chan document)

	go func(documents chan<- document) {
		if err := parseAnnotations(ctx, *annotations, documents); err != nil {
			log.Fatalf("Failed to parse %s: %v", *annotations, err)
		}
		close(documents)
	}(documents)

	if !*skipResolve {
		// If a document has multiple highlights,
		// check GCS for the current highlight.
		originalDocuments := documents
		documents = make(chan document)

		go func() {
			var allDocs []document
			for doc := range originalDocuments {
				allDocs = append(allDocs, doc)
			}
			resolved, err := resolveDocuments(ctx, storageClient, allDocs...)
			if err != nil {
				log.Fatalf("Failed to resolve: %v", err)
			}
			if len(allDocs) != len(resolved) && !*skipReplace {
				log.Println("Removing duplicate entries from", *annotations)
				if err := writeDocuments(ctx, *annotations, resolved...); err != nil {
					log.Fatalf("Failed to rewrite %s: %v", *annotations, err)
				}
			}

			for _, doc := range resolved {
				select {
				case <-ctx.Done():
					log.Fatal(ctx.Err())
				case documents <- doc:
				}
			}
			close(documents)
		}()
	}

	builds := make(chan build)

	go func() {
		defer close(builds)
		if err := parseBuilds(ctx, client, documents, builds); err != nil {
			log.Fatalf("Failed to parse builds: %v", err)
		}
	}()

	return pageByPage(ctx, builds)
}

type document struct {
	path  gcs.Path
	start int
	end   int
}

func (d document) Build() string {
	return filepath.Base(filepath.Dir(d.path.Object()))
}

func (d document) Job() string {
	return filepath.Base(filepath.Dir(filepath.Dir(d.path.Object())))
}

func (d document) String() string {
	return fmt.Sprintf("%s#%d-%d", d.path, d.start+1, d.end+1)
}

func parseAnnotations(ctx context.Context, path string, documents chan<- document) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("Failed to open %s: %v", path, err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comma = '\t'

	var i int
	for {
		i++
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("%d: %v", i, err)
		}
		if len(rec) != 3 {
			return fmt.Errorf("%d: not <path> <start> <end>: %v", i, rec)
		}
		doc, err := parseRecord(rec[0], rec[1], rec[2])
		if err != nil {
			return fmt.Errorf("%d: parse: %v", i, err)
		}
		if doc.end-doc.start > 100 {
			log.Println("Ignoring excessively long example", doc)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case documents <- *doc:
		}
	}
	return nil
}

func parseRecord(path, start, end string) (*document, error) {
	path = strings.Replace(path, "https://storage.cloud.google.com/", "gs://", 1)
	path = strings.Replace(path, "https://storage.googleapis.com/", "gs://", 1)
	p, err := gcs.NewPath(path)
	if err != nil {
		return nil, fmt.Errorf("path: %v", err)
	}
	s, err := strconv.Atoi(start)
	if err != nil {
		return nil, fmt.Errorf("start: %v", err)
	}
	e, err := strconv.Atoi(end)
	if err != nil {
		return nil, fmt.Errorf("end: %v", err)
	}
	return &document{*p, s - 1, e - 1}, nil
}

func resolveDocuments(ctx context.Context, client *storage.Client, docs ...document) ([]document, error) {
	paths := map[gcs.Path][]document{}

	for _, d := range docs {
		paths[d.path] = append(paths[d.path], d)
	}

	out := make([]document, 0, len(paths))

	for path, docs := range paths {
		switch len(docs) {
		case 0:
		case 1:
			out = append(out, docs[0])
		default:
			log.Println("Determining current highlighted range of", path)
			attrs, err := client.Bucket(path.Bucket()).Object(path.Object()).Attrs(ctx)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			start, end, err := extractRange(attrs.Metadata)
			if err != nil {
				return nil, fmt.Errorf("%s: %v", path, err)
			}
			doc := document{
				path:  path,
				start: start,
				end:   end,
			}
			out = append(out, doc)
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].path.String() < out[j].path.String()
	})

	return out, nil
}

func extractRange(meta map[string]string) (int, int, error) {
	const (
		start = "focus-start"
		end   = "focus-end"
	)
	s, e := meta[start], meta[end]
	si, err := strconv.Atoi(s)
	if err != nil {
		return 0, 0, fmt.Errorf("start: %s: %v", s, err)
	}

	ei, err := strconv.Atoi(e)
	if err != nil {
		return 0, 0, fmt.Errorf("end: %s: %v", e, err)
	}

	return si - 1, ei - 1, nil
}

func writeDocuments(ctx context.Context, path string, docs ...document) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	var didClose bool
	defer func() {
		if didClose {
			return
		}
		f.Close()
	}()
	w := csv.NewWriter(f)
	w.Comma = '\t'
	for i, d := range docs {
		if err := ctx.Err(); err != nil {
			return err
		}
		url := fmt.Sprintf("https://storage.googleapis.com/%s/%s", d.path.Bucket(), d.path.Object())
		values := []string{url, strconv.Itoa(d.start + 1), strconv.Itoa(d.end + 1)}
		if err := w.Write(values); err != nil {
			return fmt.Errorf("line %d: %w", i, err)
		}
	}
	w.Flush()
	didClose = true
	if err := f.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	return nil
}

const (
	labelHighlight = "highlight"
	labelLowlight  = "lowlight"
)

func pageByPage(ctx context.Context, builds <-chan build) (map[string]*stringset.Set, map[string]string, map[string]build) {
	var highlights stringset.Set
	var lowlights stringset.Set
	starts := map[string]int{}
	ends := map[string]int{}
	sources := map[string]build{}

	pageLen := *maxLen
	lineLen := pageLen / *minLines

	labels := map[string]*stringset.Set{}
	for b := range builds {
		allPages := b.annotate()

		pages := splitPages(allPages, lineLen, pageLen)
		pages = append(pages, highlightPages(allPages, lineLen, pageLen)...)

		for i, page := range pages {
			txt, highlight, start, end := renderPage(page, allPages)
			if txt == "" {
				continue
			}
			txt = strings.TrimSpace(txt)
			if len(txt) > pageLen {
				panic(fmt.Sprintf("Page too long: %d: %d > %d:\n%s", i, len(txt), pageLen, txt))
			}
			if highlights.Contains(txt) || lowlights.Contains(txt) {
				continue
			}
			var lbl string
			if highlight {
				highlights.Add(txt)
				lbl = labelHighlight
				if start > 0 {
					if existing, ok := starts[txt]; ok && existing != start {
						log.Println("WARNING: Duplicate starts", existing, start, "was", sources[txt].document, "now", b.document, txt)
					}
					starts[txt] = start
				}
				if end > 0 {
					if existing, ok := ends[txt]; ok && existing != end {
						log.Println("WARNING: Duplicate ends", existing, end, "was", sources[txt].document, "now", b.document, txt)

					}
					ends[txt] = end
				}
			} else {
				const lowlightOversample = 5
				if lowlights.Len() > highlights.Len()*lowlightOversample {
					continue
				}
				lowlights.Add(txt)
				lbl = labelLowlight
			}

			ss, ok := labels[lbl]
			if !ok {
				ss = &stringset.Set{}
				labels[lbl] = ss
			}
			ss.Add(txt)
			sources[txt] = b
		}
		log.Println("Processed", len(pages), "pages from", b.document.path)
	}

	suffixes := map[string]string{}

	var sb strings.Builder
	for _, hightxt := range highlights.Unordered() {
		start, hasStart := starts[hightxt]
		end, hasEnd := ends[hightxt]

		if hasStart {
			sb.WriteString(".start.")
			sb.WriteString(strconv.Itoa(start))
		}

		if hasEnd {
			sb.WriteString(".end.")
			sb.WriteString(strconv.Itoa(end))
		}

		if sb.Len() > 0 {
			suffixes[hightxt] = sb.String()
			sb.Reset()
		}
	}

	return labels, suffixes, sources
}

func splitPages(labels []label, lineLen, pageLen int) [][]label {
	var pages [][]label

	var working int

	var page []label
	for _, l := range labels {
		txt := l.text
		if t := truncateLine(l.text, lineLen); t != nil {
			l.text = *t
			txt = *t
		}
		n := len(txt) + 1 // count the \n at the end
		if n+working > pageLen {
			if len(page) > 0 {
				pages = append(pages, page)
			}
			page = nil
			working = 0
		}
		page = append(page, l)
		working += n
	}
	if len(page) > 0 {
		pages = append(pages, page)
	}
	return pages
}

func highlightPages(labels []label, lineLen, pageLen int) [][]label {
	var focused []label
	var lineno int
	var lbl label
	var before int
	for lineno, lbl = range labels {
		if lbl.highlight {
			if len(focused) == 0 {
				for i := lineno - 1; i >= 0 && before < pageLen; i-- {
					lbl := labels[i]
					before += len(lbl.text) + 1
					if before > pageLen {
						break
					}
					focused = append(focused, lbl)
				}
				for i, j := 0, len(focused)-1; i < j; i, j = i+1, j-1 {
					focused[i], focused[j] = focused[j], focused[i]
				}
			}
			focused = append(focused, lbl)
		} else if len(focused) > 0 {
			lineno--
			break
		}
	}

	var after int
	for i := lineno + 1; after < pageLen && i < len(labels); i++ {
		lbl := labels[i]
		after += len(lbl.text) + 1
		if after > pageLen {
			break
		}
		focused = append(focused, lbl)
	}

	var pages [][]label
	for i := 0; i < len(focused); i++ {
		for _, page := range splitPages(focused[i:], lineLen, pageLen) {
			for _, l := range page {
				if l.highlight {
					pages = append(pages, page)
					break
				}
			}
		}
	}
	return pages
}

func truncateLine(s string, n int) *string {
	if n <= 0 || len(s) <= n {
		return nil
	}
	half := n / 2
	s = strings.ToValidUTF8(s[:half-2]+"..."+s[len(s)-half+1:], "")
	return &s
}

func renderPage(page []label, labels []label) (string, bool, int, int) {
	var sb strings.Builder
	var high bool
	var start, end int
	for _, line := range page {
		if line.highlight {
			high = true
		}
		sb.WriteString(line.text)
		sb.WriteRune('\n')
	}
	if high {
		for i, line := range page {
			if line.highlight {
				idx := line.line - 2
				if idx < 0 || !labels[idx].highlight {
					start = i + 1
				}
				idx = line.line
				if idx >= len(labels) || !labels[idx].highlight {
					end = i + 1
				}
			}
		}
	}
	return sb.String(), high, start, end
}

type build struct {
	document
	lines    []string
	modified time.Time
}

func (b build) String() string {
	var sb strings.Builder
	sb.WriteString(b.path.String())
	sb.WriteString(":\n")
	for _, s := range b.samples() {
		if s.highlight {
			sb.WriteString("+++ ")
		} else {
			sb.WriteString("--- ")
		}
		sb.WriteString(s.text)
		sb.WriteRune('\n')
	}
	return sb.String()
}

func (b build) samples() []label {
	h, m, l := b.sample()
	out := make([]label, 0, len(h)+len(m)+len(l))
	out = append(out, h...)
	out = append(out, m...)
	out = append(out, l...)
	return out
}

func (b build) annotate() []label {
	start, end := b.start, b.end
	labels := make([]label, 0, len(b.lines))
	for lineno, line := range b.lines {
		labels = append(labels, label{
			line:      lineno + 1,
			highlight: lineno >= start && lineno <= end,
			text:      line,
		})
	}

	return labels
}

func (b build) sample() ([]label, []label, []label) {
	start, end := b.start, b.end
	if start > end {
		end, start = start, end
	}
	lines := b.lines
	n := end - start + 1
	negSamples := n * 20
	before := make([]label, 0, negSamples)
	highlight := make([]label, 0, n)
	after := make([]label, 0, negSamples)

	// might be useful to take random lines from elsewhere in the doc
	for i := start - 1 - negSamples; i < start; i++ {
		if i < 0 {
			continue
		}
		before = append(before, label{lines[i], false, i + 1})
	}

	for i := b.start; i <= b.end; i++ {
		highlight = append(highlight, label{lines[i], true, i + 1})
	}

	for i := end + 1 + negSamples; i > end; i-- {
		if i >= len(lines) {
			continue
		}
		after = append(after, label{lines[i], false, i + 1})
	}
	return before, highlight, after
}

type label struct {
	text      string
	highlight bool
	line      int
}

func parseBuilds(ctx context.Context, client gcs.ConditionalClient, documents <-chan document, builds chan<- build) error {
	bc := buildCache{
		archivePath: *cache,
	}
	defer bc.discard()
	for doc := range documents {
		r, when, err := bc.open(ctx, client, doc.path)
		if err != nil {
			log.Printf("Failed to open %s: %v", doc.path, err)
			continue
		}
		lines, err := fetchLines(ctx, r)
		if err != nil {
			log.Printf("Failed to parse %s: %v", doc.path, err)
			continue
		}
		b := build{
			lines:    lines,
			document: doc,
			modified: *when,
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case builds <- b:
		}
	}
	return bc.close()
}

type buildCache struct {
	archivePath string
	existing    *zip.ReadCloser
	additional  *zip.Writer
	additionalF *os.File
	tempPath    string
}

func (bc *buildCache) close() error {
	if bc.additional == nil {
		return nil
	}
	if bc.existing != nil {
		for _, f := range bc.existing.File {
			if f.Method == zip.Deflate {
				if err := bc.additional.Copy(f); err != nil {
					return fmt.Errorf("copy %s: %v", f.Name, err)
				}
			} else {
				log.Println("Compressing", f.Name)
				w, err := bc.additional.CreateHeader(&zip.FileHeader{
					Name:     f.Name,
					Comment:  f.Comment,
					Method:   zip.Deflate,
					Modified: f.Modified,
				})
				if err != nil {
					return fmt.Errorf("create compressed %s: %v", f.Name, err)
				}
				r, err := bc.existing.Open(f.Name)
				if err != nil {
					return fmt.Errorf("open existing %s: %v", f.Name, err)
				}
				if _, err := io.Copy(w, r); err != nil {
					return fmt.Errorf("compress %s: %v", f.Name, err)
				}
				if err := r.Close(); err != nil {
					return fmt.Errorf("close existing %s: %v", f.Name, err)
				}
			}
		}
	}

	if err := bc.additional.Close(); err != nil {
		return fmt.Errorf("close zip: %w", err)
	}

	if err := bc.additionalF.Close(); err != nil {
		return fmt.Errorf("close zip: %w", err)
	}

	from, to := bc.additionalF.Name(), bc.archivePath
	if err := os.Rename(from, to); err != nil {
		return fmt.Errorf("rename %s to %s: %v", from, to, err)
	}

	bc.additional = nil

	return nil
}

func (bc *buildCache) discard() {
	if bc.additional == nil {
		return
	}
	bc.additionalF.Close()
	os.Remove(bc.tempPath)
}

func (bc *buildCache) initAdditional() error {
	if bc.additional != nil {
		return nil
	}
	f, err := os.CreateTemp(filepath.Dir(bc.archivePath), "cached-content-*")
	if err != nil {
		return fmt.Errorf("create %s replacement: %v", bc.archivePath, err)
	}
	bc.additionalF = f
	bc.additional = zip.NewWriter(f)
	return nil
}

func (bc *buildCache) open(ctx context.Context, client gcs.ConditionalClient, path gcs.Path) (io.ReadCloser, *time.Time, error) {
	name := path.Bucket() + "/" + path.Object()
	var f io.ReadCloser
	var when *time.Time
	var err error
	if bc.existing == nil {
		if bc.archivePath != "" {
			bc.existing, err = zip.OpenReader(bc.archivePath)
			if errors.Is(err, fs.ErrNotExist) {
				err = fs.ErrNotExist
			} else if err != nil {
				return nil, nil, fmt.Errorf("open %s: %v", bc.archivePath, err)
			} else {
				for _, f := range bc.existing.File {
					if f.Method != zip.Deflate {
						bc.initAdditional()
						break
					}
				}
			}
		} else {
			err = fs.ErrNotExist
		}
	}
	if bc.existing != nil {
		f, err = bc.existing.Open(name)
	}
	if errors.Is(err, fs.ErrNotExist) {
		r, attrs, err := client.Open(ctx, path)
		if err != nil {
			return nil, nil, err
		}
		if bc.archivePath == "" {
			return r, nil, nil
		}
		buf, err := io.ReadAll(r)
		if err != nil {
			return nil, nil, fmt.Errorf("read: %v", err)
		}
		f = io.NopCloser(bytes.NewBuffer(buf))
		when = &attrs.LastModified
		if err := bc.initAdditional(); err != nil {
			return nil, nil, fmt.Errorf("init additional: %v", err)
		}
		w, err := bc.additional.CreateHeader(&zip.FileHeader{
			Name:     name,
			Comment:  path.String(),
			Modified: attrs.LastModified,
			Method:   zip.Deflate,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("create: %v", err)
		}
		if _, err := w.Write(buf); err != nil {
			return nil, nil, fmt.Errorf("write: %v", err)
		}
		log.Println("Cached", path)
	} else if err != nil {
		return nil, nil, err
	} else {
		info, err := (f.(fs.File)).Stat()
		if err != nil {
			return nil, nil, fmt.Errorf("stat: %v", err)
		}
		t := info.ModTime()
		when = &t
	}

	return f, when, nil
}

func fetchLines(ctx context.Context, r io.ReadCloser) ([]string, error) {
	defer r.Close()
	scanner := bufio.NewScanner(r)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

func sanityCheck(labels map[string]*stringset.Set) error {
	var min, max int
	var minL, maxL string
	for label, set := range labels {
		n := set.Len()
		log.Println(label, n)
		if min == 0 || n < min {
			min = n
			minL = label
		}
		if max == 0 || n > max {
			max = n
			maxL = label
		}
	}
	const weight = 20
	if min*weight < max {
		return fmt.Errorf("%s has %d examples, more than %dx less than %s with %d", minL, min, weight, maxL, max)
	}
	return nil
}

func zipLabels(ctx context.Context, path string, labels map[string]*stringset.Set, suffixes map[string]string, sources map[string]build) error {
	log.Println("Writing", path)
	var zw *zip.Writer
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("open %v", err)
	}
	zw = zip.NewWriter(f)
	defer f.Close()
	defer zw.Close()
	if suffixes == nil {
		suffixes = map[string]string{}
	}

	prefixes := map[string]string{}

	var builds stringset.Set
	for _, b := range sources {
		builds.Add(b.document.String())
	}
	if builds.Len() > 0 && *testSplit+*valSplit > 0 {
		builds := builds.Unordered()
		for i := 0; i < 3; i++ {
			rand.Shuffle(len(builds), func(i, j int) {
				builds[i], builds[j] = builds[j], builds[i]
			})
		}
		if end := int(*testSplit * float64(len(builds))); end > 0 {
			for _, b := range builds[:end] {
				prefixes[b] = "TEST"
			}
			builds = builds[end:]
		}
		if end := int(*valSplit * float64(len(builds))); end > 0 {
			for _, b := range builds[:end] {
				prefixes[b] = "VALIDATION"
			}
			builds = builds[end:]
		}
		for _, b := range builds {
			prefixes[b] = "TRAIN"
		}
	}

	for label, samples := range labels {
		if err := ctx.Err(); err != nil {
			return err
		}
		if samples.Len() == 0 {
			continue
		}
		path := label
		for i, txt := range samples.Unordered() {
			base := strconv.Itoa(i)
			if suffix := suffixes[txt]; suffix != "" && label == "highlight" {
				base = base + suffix
			}
			base += ".txt"
			name := filepath.Join(path, base)
			var when time.Time
			var where string
			if sources != nil {
				if from, ok := sources[txt]; ok {
					when = from.modified
					where = from.document.String()
				}
			}

			if pref := prefixes[where]; pref != "" {
				name = filepath.Join(pref, name)
			}

			w, err := zw.CreateHeader(&zip.FileHeader{
				Name:     name,
				Modified: when,
				Comment:  where,
				Method:   zip.Deflate,
			})
			if err != nil {
				return fmt.Errorf("create %s: %v", name, err)
			}
			if _, err := w.Write([]byte(txt)); err != nil {
				return fmt.Errorf("write %s: %v", name, err)
			}
		}
	}
	return nil
}
