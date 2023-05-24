// Package cacher defines utilities for saving and restoring caches from Google
// Cloud storage.
package cacher

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"hash"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/storage"
	"golang.org/x/crypto/blake2b"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

const (
	contentType  = "application/gzip"
	cacheControl = "public,max-age=3600"
)

// Cacher is responsible for saving and restoring caches.
type Cacher struct {
	client *storage.Client

	debug bool
}

// New creates a new cacher capable of saving and restoring the cache.
func New(ctx context.Context) (*Cacher, error) {
	client, err := storage.NewClient(ctx,
		option.WithUserAgent("gcs-cacher/1.0"))
	if err != nil {
		return nil, fmt.Errorf("failed to create storage client: %w", err)
	}

	return &Cacher{
		client: client,
	}, nil
}

// Debug enables or disables debugging for the cacher.
func (c *Cacher) Debug(val bool) {
	c.debug = val
}

// SaveRequest is used as input to the Save operation.
type SaveRequest struct {
	// Bucket is the name of the bucket from which to cache.
	Bucket string

	// Key is the cache key.
	Key string

	// Dir is the directory on disk to cache.
	Dir string
}

// Save caches the given directory in storage.
func (c *Cacher) Save(ctx context.Context, i *SaveRequest) (retErr error) {
	if i == nil {
		retErr = fmt.Errorf("missing cache options")
		return
	}

	bucket := i.Bucket
	if bucket == "" {
		retErr = fmt.Errorf("missing bucket")
		return
	}

	dir := i.Dir
	if dir == "" {
		retErr = fmt.Errorf("missing directory")
		return
	}

	key := i.Key
	if key == "" {
		retErr = fmt.Errorf("missing key")
		return
	}

	// Check if the object already exists. If it already exists, we do not want to
	// waste time overwriting the cache.
	attrs, err := c.client.Bucket(bucket).Object(key).Attrs(ctx)
	if err != nil && !errors.Is(err, storage.ErrObjectNotExist) {
		retErr = fmt.Errorf("failed to check if cached object exists: %w", err)
		return
	}
	if attrs != nil {
		c.log("cached object already exists, skipping")
		return
	}

	// Create the storage writer
	dne := storage.Conditions{DoesNotExist: true}
	gcsw := c.client.Bucket(bucket).Object(key).If(dne).NewWriter(ctx)
	defer func() {
		c.log("closing gcs writer")
		if cerr := gcsw.Close(); cerr != nil {
			if retErr != nil {
				retErr = fmt.Errorf("%v: failed to close gcs writer: %w", retErr, cerr)
				return
			}
			retErr = fmt.Errorf("failed to close gcs writer: %w", cerr)
		}
	}()

	gcsw.ChunkSize = 128_000_000
	gcsw.ObjectAttrs.ContentType = contentType
	gcsw.ObjectAttrs.CacheControl = cacheControl
	gcsw.ProgressFunc = func(soFar int64) {
		fmt.Printf("uploaded %d bytes\n", soFar)
	}

	// Create the gzip writer
	gzw := gzip.NewWriter(gcsw)
	defer func() {
		c.log("closing gzip writer")
		if cerr := gzw.Close(); cerr != nil {
			if retErr != nil {
				retErr = fmt.Errorf("%v: failed to close gzip writer: %w", retErr, cerr)
				return
			}
			retErr = fmt.Errorf("failed to close gzip writer: %w", cerr)
		}
	}()

	// Create the tar writer
	tw := tar.NewWriter(gzw)
	defer func() {
		c.log("closing tar writer")
		if cerr := tw.Close(); cerr != nil {
			if retErr != nil {
				retErr = fmt.Errorf("%v: failed to close tar writer: %w", retErr, cerr)
				return
			}
			retErr = fmt.Errorf("failed to close tar writer: %w", cerr)
		}
	}()

	// Walk all files create tar
	if err := filepath.Walk(dir, func(name string, f os.FileInfo, err error) error {
		c.log("walking file %s", name)

		if err != nil {
			return err
		}

		if !f.Mode().IsRegular() {
			c.log("file %s is not regular", name)
			return nil
		}

		// Create the tar header
		header, err := tar.FileInfoHeader(f, f.Name())
		if err != nil {
			return fmt.Errorf("failed to create tar header for %s: %w", f.Name(), err)
		}
		header.Name = strings.TrimPrefix(strings.Replace(name, dir, "", -1), string(filepath.Separator))

		// Write header to tar
		c.log("writing tar header for %s", name)
		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write tar header for %s: %w", f.Name(), err)
		}

		// Open and write file to tar
		c.log("opening %s", name)
		file, err := os.Open(name)
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", f.Name(), err)
		}

		c.log("copying %s to tar", name)
		if _, err := io.Copy(tw, file); err != nil {
			if cerr := file.Close(); cerr != nil {
				return fmt.Errorf("failed to close %s: %v: failed to write tar: %w", f.Name(), cerr, err)
			}
			return fmt.Errorf("failed to write tar for %s: %w", f.Name(), err)
		}

		// Close tar
		c.log("closing %s", name)
		if err := file.Close(); err != nil {
			return fmt.Errorf("failed to close: %w", err)
		}

		return nil
	}); err != nil {
		retErr = fmt.Errorf("failed to walk files: %w", err)
		return
	}

	return
}

// RestoreRequest is used as input to the Restore operation.
type RestoreRequest struct {
	// Bucket is the name of the bucket from which to cache.
	Bucket string

	// Keys is the ordered list of keys to restore.
	Keys []string

	// Dir is the directory on disk to cache.
	Dir string
}

// Restore restores the key from the cache into the dir on disk.
func (c *Cacher) Restore(ctx context.Context, i *RestoreRequest) (retErr error) {
	if i == nil {
		retErr = fmt.Errorf("missing cache options")
		return
	}

	bucket := i.Bucket
	if bucket == "" {
		retErr = fmt.Errorf("missing bucket")
		return
	}

	dir := i.Dir
	if dir == "" {
		retErr = fmt.Errorf("missing directory")
		return
	}

	keys := i.Keys
	if len(keys) < 1 {
		retErr = fmt.Errorf("expected at least one cache key")
		return
	}

	// Get the bucket handle
	bucketHandle := c.client.Bucket(bucket)

	// Try to find an earlier cached item by looking for the "newest" item with
	// one of the provided key fallbacks as a prefix.
	var match *storage.ObjectAttrs
	for _, key := range keys {
		c.log("searching for objects with prefix %s", key)

		it := bucketHandle.Objects(ctx, &storage.Query{
			Prefix: key,
		})

		for {
			attrs, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				retErr = fmt.Errorf("failed to list %s: %w", key, err)
				return
			}

			c.log("found object %s", key)

			if match == nil || attrs.Updated.After(match.Updated) {
				c.log("setting %s as best candidate", key)
				match = attrs
				continue
			}
		}
	}

	// Ensure we found one
	if match == nil {
		retErr = fmt.Errorf("failed to find cached objects among keys %q", keys)
		return
	}

	// Ensure the output directory exists
	c.log("making target directory %s", dir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		retErr = fmt.Errorf("failed to make target directory: %w", err)
		return
	}

	// Create the gcs reader
	gcsr, err := bucketHandle.Object(match.Name).NewReader(ctx)
	if err != nil {
		retErr = fmt.Errorf("failed to create object reader: %w", err)
		return
	}
	defer func() {
		c.log("closing gcs reader")
		if cerr := gcsr.Close(); cerr != nil {
			if retErr != nil {
				retErr = fmt.Errorf("%v: failed to close gcs reader: %w", retErr, cerr)
				return
			}
			retErr = fmt.Errorf("failed to close gcs reader: %w", cerr)
		}
	}()

	// Create the gzip reader
	gzr, err := gzip.NewReader(gcsr)
	if err != nil {
		retErr = fmt.Errorf("failed to create gzip reader: %w", err)
		return
	}
	defer func() {
		c.log("closing gzip reader")
		if cerr := gzr.Close(); cerr != nil {
			if retErr != nil {
				retErr = fmt.Errorf("%v: failed to close gzip reader: %w", retErr, cerr)
				return
			}
			retErr = fmt.Errorf("failed to close gzip reader: %w", cerr)
		}
	}()

	// Create the tar reader
	tr := tar.NewReader(gzr)

	// Unzip and untar each file into the target directory
	if err := func() error {
		for {
			header, err := tr.Next()
			if err != nil {
				if err == io.EOF {
					// No more files
					return nil
				}

				return fmt.Errorf("failed to read header: %w", err)
			}

			// Not entirely sure how this happens? I think it was because I uploaded a
			// bad tarball. Nonetheless, we shall check.
			if header == nil {
				c.log("header is nil")
				continue
			}

			target := filepath.Join(dir, header.Name)
			c.log("working on %s", target)

			switch header.Typeflag {
			case tar.TypeDir:
				c.log("creating directory %s", target)

				if err := os.MkdirAll(target, 0755); err != nil {
					return fmt.Errorf("failed to make directory %s: %w", target, err)
				}
			case tar.TypeReg:
				c.log("creating file %s", target)

				// Create the parent directory in case it does not exist...
				parent := filepath.Dir(target)
				if err := os.MkdirAll(parent, 0755); err != nil {
					return fmt.Errorf("failed to make parent directory %s: %w", parent, err)
				}

				c.log("opening %s", target)
				f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
				if err != nil {
					return fmt.Errorf("failed to open %s: %w", target, err)
				}

				c.log("copying %s to disk", target)
				if _, err := io.Copy(f, tr); err != nil {
					if cerr := f.Close(); cerr != nil {
						return fmt.Errorf("failed to close %s: %v: failed to untar: %w", target, cerr, err)
					}
					return fmt.Errorf("failed to untar %s: %w", target, err)
				}

				// Close f here instead of deferring
				c.log("closing %s", target)
				if err := f.Close(); err != nil {
					return fmt.Errorf("failed to close %s: %w", target, err)
				}
			default:
				return fmt.Errorf("unknown header type %v for %s", header.Typeflag, target)
			}
		}
	}(); err != nil {
		retErr = fmt.Errorf("failed to download file: %w", err)
		return
	}

	return
}

// HashGlob hashes the files matched by the given glob.
func (c *Cacher) HashGlob(pattern string) (string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("failed to glob: %w", err)
	}
	return c.HashFiles(matches)
}

// HashFiles hashes the list of file and returns the hex-encoded SHA256.
func (c *Cacher) HashFiles(files []string) (string, error) {
	h, err := blake2b.New(16, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create hash: %w", err)
	}

	hashOne := func(name string, h hash.Hash) (retErr error) {
		c.log("opening %s", name)
		f, err := os.Open(name)
		if err != nil {
			retErr = fmt.Errorf("failed to open file: %w", err)
			return
		}
		defer func() {
			c.log("closing %s", name)
			if cerr := f.Close(); cerr != nil {
				if retErr != nil {
					retErr = fmt.Errorf("%v: failed to close file: %w", retErr, cerr)
					return
				}
				retErr = fmt.Errorf("failed to close file: %w", cerr)
			}
		}()

		c.log("stating %s", name)
		stat, err := f.Stat()
		if err != nil {
			retErr = fmt.Errorf("failed to stat file: %w", err)
			return
		}

		if stat.IsDir() {
			c.log("skipping %s (is a directory)", name)
			return
		}

		c.log("hashing %s", name)
		if _, err := io.Copy(h, f); err != nil {
			retErr = fmt.Errorf("failed to hash: %w", err)
			return
		}

		return
	}

	for _, name := range files {
		if err := hashOne(name, h); err != nil {
			return "", fmt.Errorf("failed to hash %s: %w", name, err)
		}
	}

	dig := h.Sum(nil)
	return fmt.Sprintf("%x", dig), nil
}

func (c *Cacher) log(msg string, vars ...interface{}) {
	if c.debug {
		log.Printf(msg, vars...)
	}
}
