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
	"context"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	prowConfig "k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/testgrid/util/gcs"
	"sigs.k8s.io/yaml"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
)

type multiString []string

func (m multiString) String() string {
	return strings.Join(m, ",")
}

func (m *multiString) Set(v string) error {
	*m = strings.Split(v, ",")
	return nil
}

// How long Configurator waits between file checks in polling mode
const pollingTime = time.Second

type options struct {
	creds              string
	inputs             multiString
	oneshot            bool
	output             string
	printText          bool
	validateConfigFile bool
	worldReadable      bool
	writeYAML          bool
	prowConfig         string
	prowJobConfig      string
	defaultYAML        string
}

func (o *options) gatherOptions(fs *flag.FlagSet, args []string) error {
	fs.StringVar(&o.creds, "gcp-service-account", "", "/path/to/gcp/creds (use local creds if empty)")
	fs.BoolVar(&o.oneshot, "oneshot", false, "Write proto once and exit instead of monitoring --yaml files for changes")
	fs.StringVar(&o.output, "output", "", "write proto to gs://bucket/obj or /local/path")
	fs.BoolVar(&o.printText, "print-text", false, "print generated info in text format to stdout")
	fs.BoolVar(&o.validateConfigFile, "validate-config-file", false, "validate that the given config files are syntactically correct and exit (proto is not written anywhere)")
	fs.BoolVar(&o.worldReadable, "world-readable", false, "when uploading the proto to GCS, makes it world readable. Has no effect on writing to the local filesystem.")
	fs.BoolVar(&o.writeYAML, "output-yaml", false, "Output to TestGrid YAML instead of config proto")
	fs.Var(&o.inputs, "yaml", "comma-separated list of input YAML files or directories")
	fs.StringVar(&o.prowConfig, "prow-config", "", "path to the prow config file. Required by --prow-job-config")
	fs.StringVar(&o.prowJobConfig, "prow-job-config", "", "path to the prow job config. If specified, incorporates testgrid annotations on prowjobs. Requires --prow-config.")
	fs.StringVar(&o.defaultYAML, "default", "", "path to default settings; required for proto outputs")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if len(o.inputs) == 0 || o.inputs[0] == "" {
		return errors.New("--yaml must include at least one file")
	}

	if !o.printText && !o.validateConfigFile && o.output == "" {
		return errors.New("--print-text, --validate-config-file, or --output required")
	}
	if o.validateConfigFile && o.output != "" {
		return errors.New("--validate-config-file doesn't write the proto anywhere")
	}
	if (o.prowConfig == "") != (o.prowJobConfig == "") {
		return errors.New("--prow-config and --prow-job-config must be specified together")
	}
	if o.defaultYAML == "" && !o.writeYAML {
		logrus.Warnf("--default not explicitly specified; assuming %s", o.inputs[0])
		o.defaultYAML = o.inputs[0]
	}
	return nil
}

// announceChanges watches for changes in "paths" and writes them to the channel
func announceChanges(ctx context.Context, paths []string, channel chan []string) {
	defer close(channel)
	modified := map[string]time.Time{}

	// TODO(fejta): consider waiting for a notification rather than polling
	// but performance isn't that big a deal here.
	for {
		// Terminate
		select {
		case <-ctx.Done():
			return
		default:
		}

		var changed []string

		// Check known files for deletions
		for p := range modified {
			_, accessErr := os.Stat(p)
			if os.IsNotExist(accessErr) {
				changed = append(changed, p)
				delete(modified, p)
			}
		}

		// Check given locations for new or modified files
		err := walkForYAMLFiles(paths, func(path string, info os.FileInfo) error {
			lastModTime, present := modified[path]

			if t := info.ModTime(); !present || t.After(lastModTime) {
				changed = append(changed, path)
				modified[path] = t
			}

			return nil
		})

		if err != nil {
			logrus.WithError(err).Error("walk issue in announcer")
			return
		}

		if len(changed) > 0 {
			select {
			case <-ctx.Done():
				return
			case channel <- changed:
			}
		} else {
			time.Sleep(pollingTime)
		}
	}
}

func announceProwChanges(ctx context.Context, pca *prowConfig.Agent, channel chan []string) {
	pch := make(chan prowConfig.Delta)
	pca.Subscribe(pch)
	for {
		<-pch
		select {
		case <-ctx.Done():
			return
		case channel <- []string{"prow config"}:
		}
	}
}

func readToConfig(c *Config, paths []string) error {
	err := walkForYAMLFiles(paths, func(path string, info os.FileInfo) error {
		// Read YAML file and update config
		b, err := ioutil.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %v", path, err)
		}
		if err = c.Update(b); err != nil {
			return fmt.Errorf("failed to merge %s into config: %v", path, err)
		}

		return nil
	})

	return err
}

// walks through paths and directories, calling the passed function on each YAML file
// future modifications to what Configurator sees as a "config file" can be made here
func walkForYAMLFiles(paths []string, callFunc func(path string, info os.FileInfo) error) error {
	for _, path := range paths {
		_, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("Failed status call on %s: %v", path, err)
		}

		err = filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				logrus.WithError(err).Errorf("walking path %q.", path)
				// bad file should not stop us from parsing the directory
				return nil
			}

			if filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" {
				return nil
			}

			if info.IsDir() {
				return nil
			}

			return callFunc(path, info)
		})

		if err != nil {
			return fmt.Errorf("Failed to walk through %s: %v", path, err)
		}
	}
	return nil
}

func write(ctx context.Context, client *storage.Client, path string, bytes []byte, worldReadable bool) error {
	u, err := url.Parse(path)
	if err != nil {
		return fmt.Errorf("invalid url %s: %v", path, err)
	}
	if u.Scheme != "gs" {
		return ioutil.WriteFile(path, bytes, 0644)
	}
	var p gcs.Path
	if err = p.SetURL(u); err != nil {
		return err
	}
	return gcs.Upload(ctx, client, p, bytes, worldReadable)
}

func marshallYAML(c *Config) ([]byte, error) {
	bytes, err := yaml.Marshal(c.config)
	if err != nil {
		return nil, fmt.Errorf("could not write config to yaml: %v", err)
	}
	return bytes, nil
}

// Ignores what changed for now and recomputes everything
func doOneshot(ctx context.Context, client *storage.Client, opt options, prowConfigAgent *prowConfig.Agent) error {

	// Read Data Sources: Default, YAML configs, Prow Annotations
	var c Config
	if opt.defaultYAML != "" {
		b, err := ioutil.ReadFile(opt.defaultYAML)
		if err != nil {
			return err
		}
		if err := c.UpdateDefaults(b); err != nil {
			return err
		}
	}

	err := readToConfig(&c, opt.inputs)
	if err != nil {
		return fmt.Errorf("could not read config: %v", err)
	}

	if err := applyProwjobAnnotations(&c, prowConfigAgent); err != nil {
		return fmt.Errorf("could not apply prowjob annotations: %v", err)
	}

	// Print proto if requested
	if opt.printText {
		if opt.writeYAML {
			b, err := marshallYAML(&c)
			if err != nil {
				return fmt.Errorf("could not print yaml config: %v", err)
			}
			os.Stdout.Write(b)
		} else if err := c.MarshalText(os.Stdout); err != nil {
			return fmt.Errorf("could not print config: %v", err)
		}
	}

	// Write proto if requested
	if opt.output != "" {
		var b []byte
		var err error
		if opt.writeYAML {
			b, err = marshallYAML(&c)
		} else {
			b, err = c.MarshalBytes()
		}
		if err == nil {
			err = write(ctx, client, opt.output, b, opt.worldReadable)
		}
		if err != nil {
			return fmt.Errorf("could not write config: %v", err)
		}
	}
	return nil
}

func main() {
	// Parse flags
	var opt options
	if err := opt.gatherOptions(flag.CommandLine, os.Args[1:]); err != nil {
		log.Fatalf("Bad flags: %v", err)
	}

	ctx := context.Background()

	prowConfigAgent := &prowConfig.Agent{}
	if opt.prowConfig != "" && opt.prowJobConfig != "" {
		if err := prowConfigAgent.Start(opt.prowConfig, opt.prowJobConfig); err != nil {
			log.Fatalf("FAIL: couldn't load prow config: %v", err)
		}
	}

	// Config file validation only
	if opt.validateConfigFile {
		if err := doOneshot(ctx, nil, opt, prowConfigAgent); err != nil {
			log.Fatalf("FAIL: %v", err)
		}
		log.Println("Config validated successfully")
		return
	}

	// Setup GCS client
	var client *storage.Client
	if opt.output != "" {
		var err error
		client, err = gcs.ClientWithCreds(ctx, opt.creds)
		if err != nil {
			log.Fatalf("Failed to create storage client: %v", err)
		}
	}

	// Oneshot mode, write config and exit
	if opt.oneshot {
		if err := doOneshot(ctx, client, opt, prowConfigAgent); err != nil {
			log.Fatalf("FAIL: %v", err)
		}
		return
	}

	// Service mode, monitor input files for changes
	channel := make(chan []string)
	// Monitor files for changes
	go announceChanges(ctx, opt.inputs, channel)
	go announceProwChanges(ctx, prowConfigAgent, channel)

	// Wait for changed files
	for changes := range channel {
		log.Printf("Changed: %v", changes)
		log.Println("Writing config...")
		if err := doOneshot(ctx, client, opt, prowConfigAgent); err != nil {
			log.Printf("FAIL: %v", err)
			continue
		}
		log.Printf("Wrote config to %s", opt.output)
	}
}
