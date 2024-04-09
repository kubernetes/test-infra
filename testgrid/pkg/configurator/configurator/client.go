/*
Copyright 2021 The Kubernetes Authors.

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

package configurator

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	tgCfgUtil "github.com/GoogleCloudPlatform/testgrid/config"
	"github.com/GoogleCloudPlatform/testgrid/config/yamlcfg"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	prowConfig "sigs.k8s.io/prow/prow/config"
	"k8s.io/test-infra/testgrid/pkg/configurator/options"
	"k8s.io/test-infra/testgrid/pkg/configurator/prow"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/yaml"
)

// How long Configurator waits between file checks in polling mode
const pollingTime = time.Second

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
		return fmt.Errorf("invalid url %s: %w", path, err)
	}
	if u.Scheme != "gs" {
		return os.WriteFile(path, bytes, 0644)
	}
	var p gcs.Path
	if err = p.SetURL(u); err != nil {
		return err
	}
	_, err = gcs.Upload(ctx, client, p, bytes, worldReadable, cacheControl)
	if err != nil {
		return err
	}
	return nil
}

// Ignores what changed for now and recomputes everything
func doOneshot(ctx context.Context, opt *options.Options, prowConfigAgent *prowConfig.Agent) error {
	// Set up GCS client if output is to GCS
	var client *storage.Client

	// Read Data Sources: Default, YAML configs, Prow Annotations
	c, err := yamlcfg.ReadConfig(opt.Inputs, opt.DefaultYAML, opt.StrictUnmarshal)
	if err != nil {
		return fmt.Errorf("could not read testgrid config: %w", err)
	}

	// Remains nil if no default YAML
	var d *yamlcfg.DefaultConfiguration
	if opt.DefaultYAML != "" {
		b, err := os.ReadFile(opt.DefaultYAML)
		if err != nil {
			return err
		}
		val, err := yamlcfg.LoadDefaults(b)
		if err != nil {
			return err
		}
		d = &val

	}

	pac := prow.ProwAwareConfigurator{
		DefaultTestgridConfig: d,
		UpdateDescription:     opt.UpdateDescription,
		ProwJobConfigPath:     opt.ProwConfig.JobConfigPath,
		ProwJobURLPrefix:      opt.ProwJobURLPrefix,
	}

	if prowConfigAgent != nil {
		pac.ProwConfig = prowConfigAgent.Config()
		if err := pac.ApplyProwjobAnnotations(&c); err != nil {
			return fmt.Errorf("could not apply prowjob annotations: %w", err)
		}
	}

	if opt.ValidateConfigFile {
		return tgCfgUtil.Validate(&c)
	}

	// Print proto if requested
	if opt.PrintText {
		if opt.WriteYAML {
			b, err := yaml.Marshal(&c)
			if err != nil {
				return fmt.Errorf("could not print yaml config: %w", err)
			}
			os.Stdout.Write(b)
		} else if err := tgCfgUtil.MarshalText(&c, os.Stdout); err != nil {
			return fmt.Errorf("could not print config: %w", err)
		}
	}

	// Write proto if requested
	if len(opt.Output.Strings()) > 0 {
		var b []byte
		var err error
		if opt.WriteYAML {
			b, err = yaml.Marshal(&c)
		} else {
			b, err = tgCfgUtil.MarshalBytes(&c)
		}
		if err != nil {
			return fmt.Errorf("could not write config: %w", err)
		}

		for _, output := range opt.Output.Strings() {
			if client == nil && strings.HasPrefix(output, "gs://") {
				var err error
				var creds []string
				if opt.Creds != "" {
					creds = append(creds, opt.Creds)
				}
				client, err = gcs.ClientWithCreds(ctx, creds...)
				if err != nil {
					return fmt.Errorf("failed to create gcs client: %w", err)
				}
			}

			if err = write(ctx, client, output, b, opt.WorldReadable, ""); err != nil {
				return fmt.Errorf("could not write config: %w", err)
			}
		}
		return nil
	}
	return nil
}

func RealMain(opt *options.Options) error {
	ctx := context.Background()

	var prowConfigAgent *prowConfig.Agent
	if opt.ProwConfig.ConfigPath != "" && opt.ProwConfig.JobConfigPath != "" {
		agent, err := opt.ProwConfig.ConfigAgent()
		if err != nil {
			log.Fatalf("FAIL: couldn't load prow config: %v", err)
		}
		prowConfigAgent = agent
	}

	// Config file validation only
	if opt.ValidateConfigFile {
		err := doOneshot(ctx, opt, prowConfigAgent)
		if err == nil {
			log.Println("Config validated successfully")
		}
		return err
	}

	// Oneshot mode, write config and exit
	if opt.Oneshot {
		return doOneshot(ctx, opt, prowConfigAgent)
	}

	// Service mode, monitor input files for changes
	channel := make(chan []string)
	// Monitor files for changes
	go announceChanges(ctx, opt.Inputs, channel)
	if prowConfigAgent != nil {
		go announceProwChanges(ctx, prowConfigAgent, channel)
	}

	// Wait for changed files
	for changes := range channel {
		log.Printf("Changed: %v", changes)
		log.Println("Writing config...")
		if err := doOneshot(ctx, opt, prowConfigAgent); err != nil {
			log.Printf("FAIL: %v", err)
			continue
		}
		log.Printf("Wrote config to %v", opt.Output)
	}
	return nil
}
