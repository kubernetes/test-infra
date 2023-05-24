package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/sethvargo/gcs-cacher/cacher"
	"github.com/sethvargo/go-signalcontext"
	"google.golang.org/api/googleapi"
)

var (
	stdout = os.Stdout
	stderr = os.Stderr

	// bucket is the Cloud Storage bucket.
	bucket string

	// cache is the key to use to cache.
	cache string

	// restore is the list of restore keys to use to restore.
	restore stringSliceFlag

	// allowFailure allows a command to fail.
	allowFailure bool

	// dir is the directory on disk to cache or the destination in which to
	// restore.
	dir string

	// hash is the glob pattern to hash.
	hash string

	// debug enables debug logging.
	debug bool
)

func init() {
	flag.StringVar(&bucket, "bucket", "", "Bucket name without gs:// prefix.")
	flag.StringVar(&dir, "dir", "", "Directory to cache or restore.")

	flag.StringVar(&cache, "cache", "", "Key with which to cache.")
	flag.Var(&restore, "restore", "Keys to search to restore (can use multiple times).")
	flag.BoolVar(&allowFailure, "allow-failure", false, "Allow the command to fail.")
	flag.StringVar(&hash, "hash", "", "Glob pattern to hash.")

	flag.BoolVar(&debug, "debug", false, "Print verbose debug logs.")
}

func main() {
	ctx, done := signalcontext.OnInterrupt()

	err := realMain(ctx)
	done()

	if err != nil {
		fmt.Fprintf(stderr, "%s\n", err)

		// Check if the error is a Google API error and print extra information.
		var gerr *googleapi.Error
		if errors.As(err, &gerr) {
			fmt.Fprintf(stderr, "Error is a googleapi error:\n%s\n", err.Error())
		}

		if !allowFailure {
			os.Exit(1)
		}
	}
}

func realMain(ctx context.Context) error {
	args := os.Args
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "help" {
			flag.PrintDefaults()
			return nil
		}
	}

	flag.Parse()
	if len(flag.Args()) > 0 {
		return fmt.Errorf("no arguments expected")
	}

	c, err := cacher.New(ctx)
	if err != nil {
		return err
	}
	c.Debug(debug)

	switch {
	case cache != "":
		parsed, err := parseTemplate(c, cache)
		if err != nil {
			return err
		}

		if err := c.Save(ctx, &cacher.SaveRequest{
			Bucket: bucket,
			Dir:    dir,
			Key:    parsed,
		}); err != nil {
			return err
		}

		fmt.Fprintf(stdout, "finished saving cache\n")
		return nil
	case restore != nil:
		keys := make([]string, len(restore))
		for i, key := range restore {
			parsed, err := parseTemplate(c, key)
			if err != nil {
				return err
			}
			keys[i] = parsed
		}

		if err := c.Restore(ctx, &cacher.RestoreRequest{
			Bucket: bucket,
			Dir:    dir,
			Keys:   keys,
		}); err != nil {
			return err
		}

		fmt.Fprintf(stdout, "finished restoring cache\n")
		return nil
	default:
		return fmt.Errorf("missing command operation")
	}
}

func parseTemplate(c *cacher.Cacher, key string) (string, error) {
	tmpl, err := template.New("").
		Option("missingkey=error").
		Funcs(templateFuncs(c)).
		Parse(key)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var b bytes.Buffer
	if err := tmpl.Execute(&b, nil); err != nil {
		return "", fmt.Errorf("failed to process template: %w", err)
	}
	return b.String(), nil
}

func templateFuncs(c *cacher.Cacher) template.FuncMap {
	return template.FuncMap{
		"hashGlob": func(key string) (string, error) {
			return c.HashGlob(key)
		},
	}
}

type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	if s == nil {
		return ""
	}
	return strings.Join(*s, ",")
}

func (s *stringSliceFlag) Set(value string) error {
	var vals []string
	for _, val := range strings.Split(value, ",") {
		if k := strings.TrimSpace(val); k != "" {
			vals = append(vals, k)
		}
	}
	*s = append(*s, vals...)
	return nil
}
