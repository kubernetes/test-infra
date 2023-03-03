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

// fakepubsub wraps around the official gcloud Pub/Sub emulator.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/sirupsen/logrus"

	configflagutil "k8s.io/test-infra/prow/flagutil/config"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/test/integration/internal/fakepubsub"
)

type options struct {
	// config is the Prow configuration. We need this to read in the
	// 'pubsub_subscriptions' field set in the integration test's Prow
	// configuration, because we have to initialize (create) these subscriptions
	// before sub can start listening to them.
	config           configflagutil.ConfigOptions
	emulatorHostPort string
}

func (o *options) validate() error {
	return nil
}

func flagOptions() *options {
	// When the KIND cluster starts, the Prow configs get loaded into a
	// Kubernetes ConfigMap object. This object is then mounted as a volume into
	// the fakepubsub container at the path "/etc/config/config.yaml".
	o := &options{config: configflagutil.ConfigOptions{ConfigPath: "/etc/config/config.yaml"}}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	fs.StringVar(&o.emulatorHostPort, "emulator-host-port", "0.0.0.0:8085", "Host and port of the running Pub/Sub emulator.")
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

	if err := startPubSubEmulator(o); err != nil {
		logrus.WithError(err).Fatal("could not start Pub/Sub emulator")
	}

	if err := initEmulatorState(o); err != nil {
		logrus.WithError(err).Fatal("Could not initialize emulator state")
	}
}

// startPubSubEmulator starts the Pub/Sub Emulator. It's a Java server, so the
// host system needs the JRE installed as well as gcloud cli (this the
// recommended way to start the emulator).
func startPubSubEmulator(o *options) error {
	logrus.Info("Starting Pub/Sub emulator...")

	args := []string{"beta", "emulators", "pubsub", "start",
		fmt.Sprintf("--host-port=%s", o.emulatorHostPort)}
	cmd := exec.Command("gcloud", args...)

	// Unfortunately the emulator does not really give useful messages about
	// what type of gRPC request is being served. Still, this is better than
	// nothing.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Could not start process: %v", err)
	}
	logrus.Info("Started Pub/Sub emulator")

	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		return fmt.Errorf("Could not get pid: %v", err)
	}

	// Cleanup. Kill child processes (in our case, the emulator) if we detect
	// that we're getting shut down. See
	// https://stackoverflow.com/a/29552044/437583.
	interrupts.Run(func(ctx context.Context) {
		for {
			if _, ok := <-ctx.Done(); ok {
				syscall.Kill(-pgid, syscall.SIGTERM)
				cmd.Wait()
				logrus.Info("Pub/Sub emulator exited.")
				return
			}
		}
	})

	return nil
}

// initEmulatorState creates Pub/Sub topics and subscriptions, because
// every time the emulator starts, it starts off from a clean slate (no topics
// or subscriptions).
func initEmulatorState(o *options) error {
	configAgent, err := o.config.ConfigAgent()
	if err != nil {
		return fmt.Errorf("Error starting config agent: %v", err)
	}

	subs := configAgent.Config().PubSubSubscriptions

	logrus.Info("Initializing Pub/Sub emulator state...")

	ctx := context.Background()

	for projectID, subscriptionIDs := range subs {
		client, err := fakepubsub.NewClient(projectID, o.emulatorHostPort)
		if err != nil {
			return err
		}
		for _, subscriptionID := range subscriptionIDs {
			// Extract the number part from the subscriptionID. The pattern we use
			// for tests is "subscriptionN" where the trailing N is a number.
			// Example: For "subscription1", we create "topic1".
			numberPart := strings.TrimPrefix(subscriptionID, "subscription")
			topicID := "topic" + numberPart
			if err := client.CreateSubscription(ctx, projectID, topicID, subscriptionID); err != nil {
				return err
			}
		}
	}

	return nil
}
