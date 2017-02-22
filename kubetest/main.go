/*
Copyright 2014 The Kubernetes Authors.

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
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

var (
	interrupt = time.NewTimer(time.Duration(0)) // interrupt testing at this time.
	terminate = time.NewTimer(time.Duration(0)) // terminate testing at this time.
	// TODO(fejta): change all these _ flags to -
	build            = flag.Bool("build", false, "If true, build a new release. Otherwise, use whatever is there.")
	checkVersionSkew = flag.Bool("check_version_skew", true, ""+
		"By default, verify that client and server have exact version match. "+
		"You can explicitly set to false if you're, e.g., testing client changes "+
		"for which the server version doesn't make a difference.")
	checkLeakedResources = flag.Bool("check_leaked_resources", false, "Ensure project ends with the same resources")
	deployment           = flag.String("deployment", "bash", "up/down mechanism (defaults to cluster/kube-{up,down}.sh) (choices: bash/kops/kubernetes-anywhere)")
	down                 = flag.Bool("down", false, "If true, tear down the cluster before exiting.")
	dump                 = flag.String("dump", "", "If set, dump cluster logs to this location on test or cluster-up failure")
	kubemark             = flag.Bool("kubemark", false, "If true, run kubemark tests.")
	publish              = flag.String("publish", "", "Publish version to the specified gs:// path on success")
	skewTests            = flag.Bool("skew", false, "If true, run tests in another version at ../kubernetes/hack/e2e.go")
	testArgs             = flag.String("test_args", "", "Space-separated list of arguments to pass to Ginkgo test runner.")
	test                 = flag.Bool("test", false, "Run Ginkgo tests.")
	timeout              = flag.Duration("timeout", time.Duration(0), "Terminate testing after the timeout duration (s/m/h)")
	up                   = flag.Bool("up", false, "If true, start the the e2e cluster. If cluster is already up, recreate it.")
	upgradeArgs          = flag.String("upgrade_args", "", "If set, run upgrade tests before other tests")
	verbose              = flag.Bool("v", false, "If true, print all command output.")

	// Deprecated flags.
	deprecatedPush   = flag.Bool("push", false, "Deprecated. Does nothing.")
	deprecatedPushup = flag.Bool("pushup", false, "Deprecated. Does nothing.")
	deprecatedCtlCmd = flag.String("ctl", "", "Deprecated. Does nothing.")
)

func validWorkingDirectory() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("could not get pwd: %v", err)
	}
	acwd, err := filepath.Abs(cwd)
	if err != nil {
		return fmt.Errorf("failed to convert %s to an absolute path: %v", cwd, err)
	}
	// This also matches "kubernetes_skew" for upgrades.
	if !strings.Contains(filepath.Base(acwd), "kubernetes") {
		return fmt.Errorf("must run from kubernetes directory root: %v", acwd)
	}
	return nil
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	flag.Parse()

	if !terminate.Stop() {
		<-terminate.C // Drain the value if necessary.
	}
	if !interrupt.Stop() {
		<-interrupt.C // Drain value
	}

	if *timeout > 0 {
		log.Printf("Limiting testing to %s", *timeout)
		interrupt.Reset(*timeout)
	}

	if err := validWorkingDirectory(); err != nil {
		log.Fatalf("Called from invalid working directory: %v", err)
	}

	deploy, err := getDeployer()
	if err != nil {
		log.Fatalf("Error creating deployer: %v", err)
	}

	if *down {
		// listen for signals such as ^C and gracefully attempt to clean up
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		go func() {
			for range c {
				log.Print("Captured ^C, gracefully attempting to cleanup resources..")
				if err := deploy.Down(); err != nil {
					log.Printf("Tearing down deployment failed: %v", err)
					os.Exit(1)
				}
			}
		}()
	}

	if err := run(deploy); err != nil {
		log.Fatalf("Something went wrong: %s", err)
	}
}
