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

// fakegcsserver runs the open source GCS emulator from
// https://github.com/fsouza/fake-gcs-server.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/fsouza/fake-gcs-server/fakestorage"
	"github.com/sirupsen/logrus"

	configflagutil "k8s.io/test-infra/prow/flagutil/config"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pjutil"
)

type options struct {
	// config is the Prow configuration. We need this to read in GCS
	// configurations under plank's default_decoration_config_entries field set
	// in the integration test's Prow configuration, because we have to
	// initialize (create) these buckets before we upload into them with
	// initupload.
	config              configflagutil.ConfigOptions
	emulatorPort        uint
	emulatorPublicHost  string
	emulatorStorageRoot string
}

func (o *options) validate() error {
	if o.emulatorPort > 65535 {
		return fmt.Errorf("-emulator-port range (got %d) must be 0 - 65535", o.emulatorPort)
	}

	return nil
}

func flagOptions() *options {
	o := &options{config: configflagutil.ConfigOptions{ConfigPath: "/etc/config/config.yaml"}}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	fs.UintVar(&o.emulatorPort, "emulator-port", 8888, "Port to use for the GCS emulator.")
	fs.StringVar(&o.emulatorPublicHost, "emulator-public-host", "fakegcsserver.default:80", "Address of the GCS emulator as seen by other Prow components in the test cluster.")
	fs.StringVar(&o.emulatorStorageRoot, "emulator-storage-root", "/gcs", "Folder to store GCS buckets and objects.")
	o.config.AddFlags(fs)

	fs.Parse(os.Args[1:])

	return o
}

func main() {
	logrusutil.ComponentInit()

	o := flagOptions()
	if err := o.validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid arguments.")
	}

	health := pjutil.NewHealth()
	health.ServeReady()

	defer interrupts.WaitForGracefulShutdown()

	initialObjects, err := getInitialObjects(o)
	if err != nil {
		logrus.WithError(err).Fatal("Could not initialize emulator state")
	}

	logrus.Info("Starting server...")

	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{
		Scheme:         "http",
		Host:           "0.0.0.0",
		Port:           uint16(o.emulatorPort),
		PublicHost:     o.emulatorPublicHost,
		Writer:         logrus.New().Writer(),
		StorageRoot:    o.emulatorStorageRoot,
		InitialObjects: *initialObjects,
	})
	if err != nil {
		panic(err)
	}

	logrus.Infof("Server started at %s", server.URL())
	logrus.Infof("PublicURL: %s", server.PublicURL())
}

// getInitialObjects creates GCS buckets, because every time the emulator
// starts, it starts off from a clean slate.
func getInitialObjects(o *options) (*[]fakestorage.Object, error) {
	initialObjects := []fakestorage.Object{}
	configAgent, err := o.config.ConfigAgent()
	if err != nil {
		return &initialObjects, fmt.Errorf("Error starting config agent: %v", err)
	}

	ddcs := configAgent.Config().Plank.DefaultDecorationConfigs

	for _, ddc := range ddcs {
		logrus.Infof("detected bucket %q from configuration", ddc.Config.GCSConfiguration.Bucket)
		initialObjects = append(initialObjects, fakestorage.Object{
			BucketName: ddc.Config.GCSConfiguration.Bucket,
			Name:       "placeholder",
			Content:    []byte("This file is here so that we can create the parent directory."),
		})
	}

	return &initialObjects, nil
}
