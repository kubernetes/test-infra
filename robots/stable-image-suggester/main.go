/*
Copyright 2019 The Kubernetes Authors.

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
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/docker/docker/client"
	"github.com/go-yaml/yaml"
	"github.com/sirupsen/logrus"

	"k8s.io/kube-openapi/pkg/util/sets"
	"k8s.io/test-infra/experiment/autobumper/bumper"
	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/prow/errorutil"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/robots/pr-creator/updater"
)

const srcImageRepo = "k8s-prow-edge"

var imageRe = regexp.MustCompile(fmt.Sprintf(`gcr.io/%s/([^:/]+):([^\s"']+)`, srcImageRepo))

type options struct {
	github  flagutil.GitHubOptions
	confirm bool

	refPaths        flagutil.Strings
	cipManifestPath string
}

func (o options) validate() error {
	if len(o.refPaths.Strings()) == 0 {
		return errors.New("--ref-path must be specified at least once in order to find image tags to promote")
	}
	if len(o.cipManifestPath) == 0 {
		return errors.New("--cip-manifest-path must be specified")
	}
	return o.github.Validate(!o.confirm)
}

func optionsFromFlags() options {
	var o options
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	o.github.AddFlags(fs)
	fs.BoolVar(&o.confirm, "confirm", false, "Set to mutate github instead of a dry run")
	fs.Var(&o.refPaths, "ref-path", "Identifies a file or directory in which Prow images are referenced. May be used multiple times.")
	fs.StringVar(&o.cipManifestPath, "cip-manifest-path", "", "Path to the container image promoter manifest file to update.")
	fs.Parse(os.Args[1:])
	return o
}

func main() {
	logrus.SetLevel(logrus.DebugLevel)
	o := optionsFromFlags()
	if err := o.validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid flag.")
	}

	jamesBond := &secret.Agent{}
	if err := jamesBond.Start([]string{o.github.TokenPath}); err != nil {
		logrus.WithError(err).Fatal("Failed to start secrets agent")
	}
	stdout := bumper.HideSecretsWriter{Delegate: os.Stdout, Censor: jamesBond}
	stderr := bumper.HideSecretsWriter{Delegate: os.Stderr, Censor: jamesBond}
	gc, err := o.github.GitHubClient(jamesBond, !o.confirm)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to create github client")
	}

	user, err := gc.BotUser()
	if err != nil {
		logrus.WithError(err).Fatal("Failed to get the user data for the provided GH token.")
	}

	// Checkout repo at yesterday
	sha, err := yesterdayCommit(stdout, stderr)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to find commit SHA before yesterday.")
	}
	if err := gitCheckout(sha, stdout, stderr); err != nil {
		logrus.WithError(err).Fatal("Failed to checkout commit before yesterday.")
	}

	// Find all tags for all images
	images, err := findImages(o.refPaths.Strings())
	if err != nil {
		logrus.WithError(err).Fatal("Error looking for images to promote.")
	}
	// Pick the tag to promote.
	// NOTE: We only want to promote Prow images if we can promote all images at the same version so
	// that users can always use a single tag across all Prow images.
	tag, err := pickPromotionTag(images)
	if err != nil {
		// Not all Prow components were using the same tag.
		// Give up, but don't fail the Job.
		logrus.WithError(err).Warn("Failed to determine a single tag to promote for all images. Giving up.")
		return
	}

	// Checkout master
	if err := gitCheckout("master", stdout, stderr); err != nil {
		logrus.WithError(err).Fatal("Failed to checkout master.")
	}

	// Convert tags to digests
	docker, err := client.NewEnvClient()
	if err != nil {
		logrus.WithError(err).Fatal("Failed to create Docker client.")
	}
	digests, err := imageDigests(docker, tag, sets.StringKeySet(images))
	if err != nil {
		logrus.WithError(err).Fatal("Failed to determine image digest(s).")
	}

	// Update manifest to promote all the tags we found
	if err := updateCIPManifest(o.cipManifestPath, tag, digests); err != nil {
		logrus.WithError(err).Fatalf("Failed to update container image promote manifest file %q.", o.cipManifestPath)
	}

	matchTitle := "Promote edge Prow images to stable: "
	title := matchTitle + tag

	// Commit and push changes
	remote := fmt.Sprintf("git@github.com:%s/test-infra.git", user.Login)
	remoteBranch := "stable-image-suggestion"
	if err := bumper.GitCommitAndPush(remote, remoteBranch, user.Name, user.Email, title, stdout, stderr); err != nil {
		logrus.WithError(err).Fatal("Failed to commit and push CIP manifest changes.")
	}

	// Ensure PR exists and update if needed.
	body := title // TODO: improve this
	source := fmt.Sprintf("%s:%s", user.Login, remoteBranch)
	n, err := updater.EnsurePR("kubernetes", "test-infra", title, body, source, "master", matchTitle, gc)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to ensure PR exists.")
	}

	logrus.Infof("PR kubernetes/test-infra#%d will merge %s into master: %s", *n, source, title)
}

func imageDigests(docker *client.Client, tag string, images sets.String) (map[string]string, error) {
	digests := make(map[string]string, images.Len())
	for _, image := range images.UnsortedList() {
		taggedImage := fmt.Sprintf("gcr.io/%s/%s:%s", srcImageRepo, image, tag)
		di, err := docker.DistributionInspect(context.Background(), taggedImage, "")
		if err != nil {
			return nil, fmt.Errorf("failed to determine digest for %q: %v", taggedImage, err)
		}
		digests[image] = string(di.Descriptor.Digest)
	}
	return digests, nil
}

func yesterdayCommit(stdout, stderr io.Writer) (string, error) {
	var buf bytes.Buffer
	if err := call(&buf, stderr, "git", strings.Split("rev-list -n 1 --first-parent --before=yesterday master", " ")...); err != nil {
		return "", fmt.Errorf("rev-list failed: %v", err)
	}
	sha, err := ioutil.ReadAll(io.TeeReader(&buf, stdout))
	if err != nil {
		return "", fmt.Errorf("failed to read SHA from command output buffer: %v", err)
	}
	return strings.TrimSpace(string(sha)), nil
}

func gitCheckout(ref string, stdout, stderr io.Writer) error {
	if err := call(stdout, stderr, "git", "checkout", string(ref)); err != nil {
		return fmt.Errorf("failed to checkout ref %q: %v", ref, err)
	}
	return nil
}

func call(stdout, stderr io.Writer, cmd string, args ...string) error {
	c := exec.Command(cmd, args...)
	c.Stdout = stdout
	c.Stderr = stderr
	logrus.WithField("command", fmt.Sprintf("%s %s", cmd, strings.Join(args, " "))).Info("Running command.")
	return c.Run()
}

func findImages(paths []string) (map[string]sets.String, error) {
	images := map[string]sets.String{}
	for _, path := range paths {
		err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".yaml") {
				return nil // Skip this file or directory.
			}
			content, err := ioutil.ReadFile(path)
			if err != nil {
				return fmt.Errorf("failed to read %s: %v", path, err)
			}
			for _, match := range imageRe.FindAllStringSubmatch(string(content), -1) {
				if len(match) != 3 {
					return fmt.Errorf("impossible regexp match in %q, expected 3 groups, got: %q", path, match)
				}
				if _, exists := images[match[1]]; !exists {
					images[match[1]] = sets.NewString()
				}
				images[match[1]].Insert(match[2])
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return images, nil
}

func pickPromotionTag(images map[string]sets.String) (string, error) {
	var errs []error
	candidates := sets.NewString()
	// First check for images with more than 1 tag.
	for image, tags := range images {
		if tags.Len() != 1 {
			errs = append(errs, fmt.Errorf("did not find 1 tag for %s; tags: %q", image, tags.List()))
			continue
		}
		candidates = candidates.Union(tags)
	}
	// Now check that all images had the same tag.
	switch candidates.Len() {
	case 0:
		errs = append(errs, errors.New("failed to find any image tags to promote"))
	case 1:
	default:
		errs = append(errs, fmt.Errorf("found different image tags for different images: %q", candidates.List()))
	}
	if err := errorutil.NewAggregate(errs...); err != nil {
		return "", err
	}
	return candidates.List()[0], nil
}

func updateCIPManifest(path, tag string, digests map[string]string) error {
	// Load and parse existing manifest file.
	existingContent, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read %s: %v", path, err)
	}
	var manifest Manifest
	if err := yaml.Unmarshal(existingContent, &manifest); err != nil {
		return fmt.Errorf("failed to unmarshal existing CIP manifest: %v", err)
	}
	// Create or update the entry for each image.
LOOP:
	for imageName, digest := range digests {
		dmap := DigestTags{digest: []string{tag, "latest"}}
		// First check for an existing entry to update.
		for i := range manifest.Images {
			if manifest.Images[i].ImageName == imageName {
				manifest.Images[i].Dmap = dmap
				continue LOOP
			}
		}
		// No existing entry to update, make a new one.
		manifest.Images = append(manifest.Images, Image{ImageName: imageName, Dmap: dmap})
	}
	// Marshal and write the updated manifest file back to disk.
	content, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("error marshaling updated manifest: %v", err)
	}
	if err := ioutil.WriteFile(path, content, 0666); err != nil {
		return fmt.Errorf("error writing updated manifest to disk: %v", err)
	}
	return nil
}
