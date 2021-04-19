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
	"strings"
	"time"

	tgCfgUtil "github.com/GoogleCloudPlatform/testgrid/config"
	"github.com/GoogleCloudPlatform/testgrid/config/yamlcfg"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	prowConfig "k8s.io/test-infra/prow/config"
	configflagutil "k8s.io/test-infra/prow/flagutil/config"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/yaml"
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
	prowConfig         configflagutil.ConfigOptions
	defaultYAML        string
	updateDescription  bool
	prowJobURLPrefix   string
	strictUnmarshal    bool
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
	o.prowConfig.ConfigPathFlagName = "prow-config"
	o.prowConfig.JobConfigPathFlagName = "prow-job-config"
	o.prowConfig.AddFlags(fs)
	fs.StringVar(&o.defaultYAML, "default", "", "path to default settings; required for proto outputs")
	fs.BoolVar(&o.updateDescription, "update-description", false, "add prowjob info to description even if non-empty")
	fs.StringVar(&o.prowJobURLPrefix, "prowjob-url-prefix", "", "for prowjob_config_url in descriptions: {prowjob-url-prefix}/{prowjob.sourcepath}")
	fs.BoolVar(&o.strictUnmarshal, "strict-unmarshal", false, "whether or not we want to be strict when unmarshalling configs")

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
	if err := o.prowConfig.ValidateConfigOptional(); err != nil {
		return err
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
		err := yamlcfg.SeekYAMLFiles(paths, func(path string, info os.FileInfo) error {
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

func write(ctx context.Context, client *storage.Client, path string, bytes []byte, worldReadable bool, cacheControl string) error {

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
	return gcs.Upload(ctx, client, p, bytes, worldReadable, cacheControl)
}

// Ignores what changed for now and recomputes everything
func doOneshot(ctx context.Context, client *storage.Client, opt options, prowConfigAgent *prowConfig.Agent) error {

	// Read Data Sources: Default, YAML configs, Prow Annotations
	c, err := yamlcfg.ReadConfig(opt.inputs, opt.defaultYAML, opt.strictUnmarshal)
	if err != nil {
		return fmt.Errorf("could not read testgrid config: %v", err)
	}

	// Remains nil if no default YAML
	var d *yamlcfg.DefaultConfiguration
	if opt.defaultYAML != "" {
		b, err := ioutil.ReadFile(opt.defaultYAML)
		if err != nil {
			return err
		}
		val, err := yamlcfg.LoadDefaults(b)
		if err != nil {
			return err
		}
		d = &val

	}

	pac := prowAwareConfigurator{
		defaultTestgridConfig: d,
		prowConfig:            prowConfigAgent.Config(),
		updateDescription:     opt.updateDescription,
		prowJobConfigPath:     opt.prowConfig.JobConfigPath,
		prowJobURLPrefix:      opt.prowJobURLPrefix,
	}

	if err := pac.applyProwjobAnnotations(&c); err != nil {
		return fmt.Errorf("could not apply prowjob annotations: %v", err)
	}

	// Print proto if requested
	if opt.printText {
		if opt.writeYAML {
			b, err := yaml.Marshal(&c)
			if err != nil {
				return fmt.Errorf("could not print yaml config: %v", err)
			}
			os.Stdout.Write(b)
		} else if err := tgCfgUtil.MarshalText(&c, os.Stdout); err != nil {
			return fmt.Errorf("could not print config: %v", err)
		}
	}

	// Write proto if requested
	if opt.output != "" {
		var b []byte
		var err error
		if opt.writeYAML {
			b, err = yaml.Marshal(&c)
		} else {
			b, err = tgCfgUtil.MarshalBytes(&c)
		}
		if err == nil {
			err = write(ctx, client, opt.output, b, opt.worldReadable, "")
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

	var prowConfigAgent *prowConfig.Agent
	if opt.prowConfig.ConfigPath != "" && opt.prowConfig.JobConfigPath != "" {
		agent, err := opt.prowConfig.ConfigAgent()
		if err != nil {
			log.Fatalf("FAIL: couldn't load prow config: %v", err)
		}
		prowConfigAgent = agent
	}

	// Config file validation only
	if opt.validateConfigFile {
		if err := doOneshot(ctx, nil, opt, prowConfigAgent); err != nil {
			log.Fatalf("FAIL: %v", err)
		}
		log.Println("Config validated successfully")
		return
	}

	// Set up GCS client if output is to GCS
	var client *storage.Client
	if strings.HasPrefix(opt.output, "gs://") {
		var err error
		var creds []string
		if opt.creds != "" {
			creds = append(creds, opt.creds)
		}
		client, err = gcs.ClientWithCreds(ctx, creds...)
		if err != nil {
			log.Fatalf("failed to create gcs client: %v", err)
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
