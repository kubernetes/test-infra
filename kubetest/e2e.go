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
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
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

func appendError(errs []error, err error) []error {
	if err != nil {
		return append(errs, err)
	}
	return errs
}

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

func run(deploy deployer) error {
	if *dump != "" {
		defer writeXML(time.Now())
	}

	if *build {
		if err := xmlWrap("Build", Build); err != nil {
			return fmt.Errorf("error building: %s", err)
		}
	}

	if *checkVersionSkew {
		os.Setenv("KUBECTL", "./cluster/kubectl.sh --match-server-version")
	} else {
		os.Setenv("KUBECTL", "./cluster/kubectl.sh")
	}
	os.Setenv("KUBE_CONFIG_FILE", "config-test.sh")
	// force having batch/v2alpha1 always on for e2e tests
	os.Setenv("KUBE_RUNTIME_CONFIG", "batch/v2alpha1=true")

	if *up {
		if err := xmlWrap("TearDown Previous", deploy.Down); err != nil {
			return fmt.Errorf("error tearing down previous cluster: %s", err)
		}
	}

	var err error
	var errs []error

	var (
		beforeResources []byte
		upResources     []byte
		downResources   []byte
		afterResources  []byte
	)

	if *checkLeakedResources {
		errs = appendError(errs, xmlWrap("ListResources Before", func() error {
			beforeResources, err = ListResources()
			return err
		}))
	}

	if *up {
		// If we tried to bring the cluster up, make a courtesy
		// attempt to bring it down so we're not leaving resources around.
		//
		// TODO: We should try calling deploy.Down exactly once. Though to
		// stop the leaking resources for now, we want to be on the safe side
		// and call it explictly in defer if the other one is not called.
		if *down {
			defer xmlWrap("Deferred TearDown", deploy.Down)
		}
		// Start the cluster using this version.
		if err := xmlWrap("Up", deploy.Up); err != nil {
			if *dump != "" {
				xmlWrap("DumpClusterLogs", func() error {
					return DumpClusterLogs(*dump)
				})
			}
			return fmt.Errorf("starting e2e cluster: %s", err)
		}
		if *dump != "" {
			errs = appendError(errs, xmlWrap("list nodes", listNodes))
		}
	}

	if *checkLeakedResources {
		errs = appendError(errs, xmlWrap("ListResources Up", func() error {
			upResources, err = ListResources()
			return err
		}))
	}

	if *upgradeArgs != "" {
		errs = appendError(errs, xmlWrap("UpgradeTest", func() error {
			return UpgradeTest(*upgradeArgs)
		}))
	}

	if *test {
		errs = appendError(errs, xmlWrap("get kubeconfig", deploy.SetupKubecfg))
		errs = appendError(errs, xmlWrap("kubectl version", func() error {
			return finishRunning(exec.Command("./cluster/kubectl.sh", "version", "--match-server-version=false"))
		}))
		if *skewTests {
			errs = appendError(errs, xmlWrap("SkewTest", SkewTest))
		} else {
			if err := xmlWrap("IsUp", deploy.IsUp); err != nil {
				errs = appendError(errs, err)
			} else {
				errs = appendError(errs, xmlWrap("Test", Test))
			}
		}
	}

	if *kubemark {
		errs = appendError(errs, xmlWrap("Kubemark", KubemarkTest))
	}

	if len(errs) > 0 && *dump != "" {
		errs = appendError(errs, xmlWrap("DumpClusterLogs", func() error {
			return DumpClusterLogs(*dump)
		}))
	}

	if *checkLeakedResources {
		errs = appendError(errs, xmlWrap("ListResources Down", func() error {
			downResources, err = ListResources()
			return err
		}))
	}

	if *down {
		errs = appendError(errs, xmlWrap("TearDown", deploy.Down))
	}

	if *checkLeakedResources {
		log.Print("Sleeping for 30 seconds...") // Wait for eventually consistent listing
		time.Sleep(30 * time.Second)
		if err := xmlWrap("ListResources After", func() error {
			afterResources, err = ListResources()
			return err
		}); err != nil {
			errs = append(errs, err)
		} else {
			errs = appendError(errs, xmlWrap("DiffResources", func() error {
				return DiffResources(beforeResources, upResources, downResources, afterResources, *dump)
			}))
		}
	}
	if len(errs) == 0 && *publish != "" {
		errs = appendError(errs, xmlWrap("Publish version", func() error {
			// Use plaintext version file packaged with kubernetes.tar.gz
			if v, err := ioutil.ReadFile("version"); err != nil {
				return err
			} else {
				log.Printf("Set %s version to %s", *publish, string(v))
			}
			return finishRunning(exec.Command("gsutil", "cp", "version", *publish))
		}))
	}

	if len(errs) != 0 {
		return fmt.Errorf("encountered %d errors: %v", len(errs), errs)
	}
	return nil
}

func listNodes() error {
	cmd := exec.Command("./cluster/kubectl.sh", "--match-server-version=false", "get", "nodes", "-oyaml")
	b, err := cmd.CombinedOutput()
	if *verbose {
		log.Printf("kubectl get nodes:\n%s", string(b))
	}
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filepath.Join(*dump, "nodes.yaml"), b, 0644)
}

func DiffResources(before, clusterUp, clusterDown, after []byte, location string) error {
	if location == "" {
		var err error
		location, err = ioutil.TempDir("", "e2e-check-resources")
		if err != nil {
			return fmt.Errorf("Could not create e2e-check-resources temp dir: %s", err)
		}
	}

	var mode os.FileMode = 0664
	bp := filepath.Join(location, "gcp-resources-before.txt")
	up := filepath.Join(location, "gcp-resources-cluster-up.txt")
	cdp := filepath.Join(location, "gcp-resources-cluster-down.txt")
	ap := filepath.Join(location, "gcp-resources-after.txt")
	dp := filepath.Join(location, "gcp-resources-diff.txt")

	if err := ioutil.WriteFile(bp, before, mode); err != nil {
		return err
	}
	if err := ioutil.WriteFile(up, clusterUp, mode); err != nil {
		return err
	}
	if err := ioutil.WriteFile(cdp, clusterDown, mode); err != nil {
		return err
	}
	if err := ioutil.WriteFile(ap, after, mode); err != nil {
		return err
	}

	cmd := exec.Command("diff", "-sw", "-U0", "-F^\\[.*\\]$", bp, ap)
	if *verbose {
		cmd.Stderr = os.Stderr
	}
	stdout, cerr := cmd.Output()
	if err := ioutil.WriteFile(dp, stdout, mode); err != nil {
		return err
	}
	if cerr == nil { // No diffs
		return nil
	}
	lines := strings.Split(string(stdout), "\n")
	if len(lines) < 3 { // Ignore the +++ and --- header lines
		return nil
	}
	lines = lines[2:]

	var added, report []string
	resourceTypeRE := regexp.MustCompile(`^@@.+\s(\[\s\S+\s\])$`)
	for _, l := range lines {
		if matches := resourceTypeRE.FindStringSubmatch(l); matches != nil {
			report = append(report, matches[1])
		}
		if strings.HasPrefix(l, "+") && len(strings.TrimPrefix(l, "+")) > 0 {
			added = append(added, l)
			report = append(report, l)
		}
	}
	if len(added) > 0 {
		return fmt.Errorf("Error: %d leaked resources\n%v", len(added), strings.Join(report, "\n"))
	}
	return nil
}

func ListResources() ([]byte, error) {
	log.Printf("Listing resources...")
	cmd := exec.Command("./cluster/gce/list-resources.sh")
	if *verbose {
		cmd.Stderr = os.Stderr
	}
	stdout, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("Failed to list resources (%s):\n%s", err, string(stdout))
	}
	return stdout, nil
}

func Build() error {
	// The build-release script needs stdin to ask the user whether
	// it's OK to download the docker image.
	cmd := exec.Command("make", "quick-release")
	cmd.Stdin = os.Stdin
	if err := finishRunning(cmd); err != nil {
		return fmt.Errorf("error building kubernetes: %v", err)
	}
	return nil
}

type deployer interface {
	Up() error
	IsUp() error
	SetupKubecfg() error
	Down() error
}

func getDeployer() (deployer, error) {
	switch *deployment {
	case "bash":
		return bash{}, nil
	case "kops":
		return NewKops()
	case "kubernetes-anywhere":
		return NewKubernetesAnywhere()
	default:
		return nil, fmt.Errorf("Unknown deployment strategy %q", *deployment)
	}
}

func clusterSize(deploy deployer) (int, error) {
	if err := deploy.SetupKubecfg(); err != nil {
		return -1, err
	}
	o, err := exec.Command("kubectl", "get", "nodes", "--no-headers").Output()
	if err != nil {
		log.Printf("kubectl get nodes failed: %s\n%s", WrapError(err).Error(), string(o))
		return -1, err
	}
	stdout := strings.TrimSpace(string(o))
	log.Printf("Cluster nodes:\n%s", stdout)
	return len(strings.Split(stdout, "\n")), nil
}

// CommandError will provide stderr output (if available) from structured
// exit errors
type CommandError struct {
	err error
}

func WrapError(err error) *CommandError {
	if err == nil {
		return nil
	}
	return &CommandError{err: err}
}

func (e *CommandError) Error() string {
	if e == nil {
		return ""
	}
	exitErr, ok := e.err.(*exec.ExitError)
	if !ok {
		return e.err.Error()
	}

	stderr := ""
	if exitErr.Stderr != nil {
		stderr = string(stderr)
	}
	return fmt.Sprintf("%q: %q", exitErr.Error(), stderr)
}

func isUp(d deployer) error {
	n, err := clusterSize(d)
	if err != nil {
		return err
	}
	if n <= 0 {
		return fmt.Errorf("cluster found, but %d nodes reported", n)
	}
	return nil
}

func waitForNodes(d deployer, nodes int, timeout time.Duration) error {
	for stop := time.Now().Add(timeout); time.Now().Before(stop); time.Sleep(30 * time.Second) {
		n, err := clusterSize(d)
		if err != nil {
			log.Printf("Can't get cluster size, sleeping: %v", err)
			continue
		}
		if n < nodes {
			log.Printf("%d (current nodes) < %d (requested instances), sleeping", n, nodes)
			continue
		}
		return nil
	}
	return fmt.Errorf("waiting for nodes timed out")
}

func DumpClusterLogs(location string) error {
	log.Printf("Dumping cluster logs to: %v", location)
	return finishRunning(exec.Command("./cluster/log-dump.sh", location))
}

func KubemarkTest() error {
	// Stop previous run
	err := finishRunning(exec.Command("./test/kubemark/stop-kubemark.sh"))
	if err != nil {
		return err
	}
	// If we tried to bring the Kubemark cluster up, make a courtesy
	// attempt to bring it down so we're not leaving resources around.
	//
	// TODO: We should try calling stop-kubemark exactly once. Though to
	// stop the leaking resources for now, we want to be on the safe side
	// and call it explictly in defer if the other one is not called.
	defer xmlWrap("Deferred Stop kubemark", func() error {
		return finishRunning(exec.Command("./test/kubemark/stop-kubemark.sh"))
	})

	// Start new run
	backups := []string{"NUM_NODES", "MASTER_SIZE"}
	for _, item := range backups {
		old, present := os.LookupEnv(item)
		if present {
			defer os.Setenv(item, old)
		} else {
			defer os.Unsetenv(item)
		}
	}
	os.Setenv("NUM_NODES", os.Getenv("KUBEMARK_NUM_NODES"))
	os.Setenv("MASTER_SIZE", os.Getenv("KUBEMARK_MASTER_SIZE"))
	err = xmlWrap("Start kubemark", func() error {
		return finishRunning(exec.Command("./test/kubemark/start-kubemark.sh"))
	})
	if err != nil {
		return err
	}

	// Run kubemark tests
	focus, present := os.LookupEnv("KUBEMARK_TESTS")
	if !present {
		focus = "starting\\s30\\pods"
	}
	test_args := os.Getenv("KUBEMARK_TEST_ARGS")

	err = finishRunning(exec.Command("./test/kubemark/run-e2e-tests.sh", "--ginkgo.focus="+focus, test_args))
	if err != nil {
		return err
	}

	err = xmlWrap("Stop kubemark", func() error {
		return finishRunning(exec.Command("./test/kubemark/stop-kubemark.sh"))
	})
	if err != nil {
		return err
	}
	return nil
}

func chdirSkew() (string, error) {
	old, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to os.Getwd(): %v", err)
	}
	err = os.Chdir("../kubernetes_skew")
	if err != nil {
		return "", fmt.Errorf("failed to cd ../kubernetes_skew: %v", err)
	}
	return old, nil
}

func UpgradeTest(args string) error {
	old, err := chdirSkew()
	if err != nil {
		return err
	}
	defer os.Chdir(old)
	previous, present := os.LookupEnv("E2E_REPORT_PREFIX")
	if present {
		defer os.Setenv("E2E_REPORT_PREFIX", previous)
	} else {
		defer os.Unsetenv("E2E_REPORT_PREFIX")
	}
	os.Setenv("E2E_REPORT_PREFIX", "upgrade")
	return finishRunning(exec.Command(
		"go", "run", "./hack/e2e.go",
		"--test",
		"--test_args="+args,
		fmt.Sprintf("--v=%t", *verbose),
		fmt.Sprintf("--check_version_skew=%t", *checkVersionSkew)))
}

func SkewTest() error {
	old, err := chdirSkew()
	if err != nil {
		return err
	}
	defer os.Chdir(old)
	return finishRunning(exec.Command(
		"go", "run", "./hack/e2e.go",
		"--test",
		"--test_args="+*testArgs,
		fmt.Sprintf("--v=%t", *verbose),
		fmt.Sprintf("--check_version_skew=%t", *checkVersionSkew)))
}

func Test() error {
	// TODO(fejta): add a --federated or something similar
	if os.Getenv("FEDERATION") != "true" {
		return finishRunning(exec.Command("./hack/ginkgo-e2e.sh", strings.Fields(*testArgs)...))
	}

	if *testArgs == "" {
		*testArgs = "--ginkgo.focus=\\[Feature:Federation\\]"
	}
	return finishRunning(exec.Command("./hack/federated-ginkgo-e2e.sh", strings.Fields(*testArgs)...))
}
