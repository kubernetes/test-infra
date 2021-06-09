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
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"k8s.io/test-infra/kubetest/e2e"
	"k8s.io/test-infra/kubetest/process"
	"k8s.io/test-infra/kubetest/util"
)

// Add more default --test_args as we migrate them
func argFields(args, dump, ipRange string) []string {
	f := strings.Fields(args)
	if dump != "" {
		f = util.SetFieldDefault(f, "--report-dir", dump)
		// Disable logdump within ginkgo as it'll be done in kubetest anyway now.
		f = util.SetFieldDefault(f, "--disable-log-dump", "true")
	}
	if ipRange != "" {
		f = util.SetFieldDefault(f, "--cluster-ip-range", ipRange)
	}
	return f
}

func run(deploy deployer, o options) error {
	cmd, err := deploy.KubectlCommand()
	if err != nil {
		return err
	}
	if cmd == nil {
		cmd = exec.Command("./cluster/kubectl.sh")
	}
	if o.checkSkew {
		cmd.Args = append(cmd.Args, "--match-server-version")
	}
	os.Setenv("KUBECTL", strings.Join(cmd.Args, " "))

	os.Setenv("KUBE_CONFIG_FILE", "config-test.sh")
	os.Setenv("KUBE_RUNTIME_CONFIG", o.runtimeConfig)

	var errs []error

	dump, err := util.OptionalAbsPath(o.dump)
	if err != nil {
		return fmt.Errorf("failed handling --dump path: %v", err)
	}

	dumpPreTestLogs, err := util.OptionalAbsPath(o.dumpPreTestLogs)
	if err != nil {
		return fmt.Errorf("failed handling --dump-pre-test-logs path: %v", err)
	}

	if o.up {
		if err := control.XMLWrap(&suite, "TearDown Previous", deploy.Down); err != nil {
			return fmt.Errorf("error tearing down previous cluster: %s", err)
		}
	}

	// Ensures that the cleanup/down action is performed exactly once.
	var (
		downDone = false
	)

	var (
		beforeResources []byte
		upResources     []byte
		downResources   []byte
		afterResources  []byte
	)

	if o.checkLeaks {
		errs = util.AppendError(errs, control.XMLWrap(&suite, "listResources Before", func() error {
			beforeResources, err = listResources()
			return err
		}))
	}

	if o.up {
		// If we tried to bring the cluster up, make a courtesy
		// attempt to bring it down so we're not leaving resources around.
		if o.down {
			defer control.XMLWrap(&suite, "Deferred TearDown", func() error {
				if !downDone {
					return deploy.Down()
				}
				return nil
			})
		}
		// Start the cluster using this version.
		if err := control.XMLWrap(&suite, "Up", deploy.Up); err != nil {
			if dump != "" {
				control.XMLWrap(&suite, "DumpClusterLogs (--up failed)", func() error {
					// This frequently means the cluster does not exist.
					// Thus DumpClusterLogs() typically fails.
					// Therefore always return null for this scenarios.
					// TODO(fejta): report a green E in testgrid if it errors.
					deploy.DumpClusterLogs(dump, o.logexporterGCSPath)
					return nil
				})
			}
			return fmt.Errorf("starting e2e cluster: %s", err)
		}
		// If node testing is enabled, check that the api is reachable before
		// proceeding with further steps. This is accomplished by listing the nodes.
		if !o.nodeTests && !strings.EqualFold(string(o.build), "none") {
			errs = util.AppendError(errs, control.XMLWrap(&suite, "Check APIReachability", func() error { return getKubectlVersion(deploy) }))
			if dump != "" {
				errs = util.AppendError(errs, control.XMLWrap(&suite, "list nodes", func() error {
					return listNodes(deploy, dump)
				}))
			}
		}
	}

	if o.checkLeaks {
		errs = util.AppendError(errs, control.XMLWrap(&suite, "listResources Up", func() error {
			upResources, err = listResources()
			return err
		}))
	}

	if o.upgradeArgs != "" {
		if err := control.XMLWrap(&suite, "test setup", deploy.TestSetup); err != nil {
			errs = util.AppendError(errs, err)
		} else {
			errs = util.AppendError(errs, control.XMLWrap(&suite, "UpgradeTest", func() error {
				// upgrade tests really only run one spec
				var env []string
				for _, v := range os.Environ() {
					if !strings.HasPrefix(v, "GINKGO_PARALLEL") {
						env = append(env, v)
					}
				}
				return skewTestEnv(env, argFields(o.upgradeArgs, dump, o.clusterIPRange), "upgrade", o.checkSkew)
			}))
		}
	}

	if dumpPreTestLogs != "" {
		errs = append(errs, dumpRemoteLogs(deploy, o, dumpPreTestLogs, "pre-test")...)
	}

	testArgs := argFields(o.testArgs, dump, o.clusterIPRange)
	if o.test {
		if err := control.XMLWrap(&suite, "test setup", deploy.TestSetup); err != nil {
			errs = util.AppendError(errs, err)
		} else if o.nodeTests {
			nodeArgs := strings.Fields(o.nodeArgs)
			errs = util.AppendError(errs, control.XMLWrap(&suite, "Node Tests", func() error {
				return nodeTest(nodeArgs, o.testArgs, o.nodeTestArgs, o.gcpProject, o.gcpZone, o.runtimeConfig)
			}))
		} else if err := control.XMLWrap(&suite, "IsUp", deploy.IsUp); err != nil {
			errs = util.AppendError(errs, err)
		} else {
			if o.deployment != "conformance" {
				errs = util.AppendError(errs, control.XMLWrap(&suite, "kubectl version", func() error { return getKubectlVersion(deploy) }))
			}

			if o.skew {
				errs = util.AppendError(errs, control.XMLWrap(&suite, "SkewTest", func() error {
					return skewTest(testArgs, "skew", o.checkSkew)
				}))
			} else {
				var tester e2e.Tester
				tester = &GinkgoScriptTester{}
				if testBuilder, ok := deploy.(e2e.TestBuilder); ok {
					tester, err = testBuilder.BuildTester(toBuildTesterOptions(&o))
					errs = util.AppendError(errs, err)
				}
				if tester != nil {
					errs = util.AppendError(errs, control.XMLWrap(&suite, "Test", func() error {
						return tester.Run(control, testArgs)
					}))
				}
			}
		}
	}

	var kubemarkUpErr error
	if o.kubemark {
		errs = util.AppendError(errs, control.XMLWrap(&suite, "Kubemark Overall", func() error {
			if kubemarkUpErr = kubemarkUp(dump, o, deploy); kubemarkUpErr != nil {
				return kubemarkUpErr
			}
			// running test in clusterloader, or other custom commands, skip the ginkgo call
			if o.testCmd != "" {
				return nil
			}
			return kubemarkGinkgoTest(testArgs, dump)
		}))
	}

	if kubemarkUpErr == nil && o.testCmd != "" {
		if err := control.XMLWrap(&suite, "test setup", deploy.TestSetup); err != nil {
			errs = util.AppendError(errs, err)
		} else {
			errs = util.AppendError(errs, control.XMLWrap(&suite, o.testCmdName, func() error {
				cmdLine := os.ExpandEnv(o.testCmd)
				return control.FinishRunning(exec.Command(cmdLine, o.testCmdArgs...))
			}))
		}
	}

	// TODO: consider remapping charts, etc to testCmd

	var kubemarkWg sync.WaitGroup
	var kubemarkDownErr error
	if o.down && o.kubemark {
		kubemarkWg.Add(1)
		go kubemarkDown(&kubemarkDownErr, &kubemarkWg, o.provider, dump)
	}

	if o.charts {
		errs = util.AppendError(errs, control.XMLWrap(&suite, "Helm Charts", chartsTest))
	}

	if dump != "" {
		errs = append(errs, dumpRemoteLogs(deploy, o, dump, "")...)
	}

	if o.checkLeaks {
		errs = util.AppendError(errs, control.XMLWrap(&suite, "listResources Down", func() error {
			downResources, err = listResources()
			return err
		}))
	}

	if o.down {
		errs = util.AppendError(errs, control.XMLWrap(&suite, "TearDown", func() error {
			if !downDone {
				err := deploy.Down()
				if err != nil {
					return err
				}
				downDone = true
			}
			return nil
		}))
	}

	// Wait for kubemarkDown step to finish before going further.
	kubemarkWg.Wait()
	errs = util.AppendError(errs, kubemarkDownErr)

	// Save the state if we upped a new cluster without downing it
	if o.save != "" && ((!o.down && o.up) || (o.up && o.deployment != "none")) {
		errs = util.AppendError(errs, control.XMLWrap(&suite, "Save Cluster State", func() error {
			return saveState(o.save)
		}))
	}

	if o.checkLeaks {
		log.Print("Sleeping for 30 seconds...") // Wait for eventually consistent listing
		time.Sleep(30 * time.Second)
		if err := control.XMLWrap(&suite, "listResources After", func() error {
			afterResources, err = listResources()
			return err
		}); err != nil {
			errs = append(errs, err)
		} else {
			errs = util.AppendError(errs, control.XMLWrap(&suite, "diffResources", func() error {
				return diffResources(beforeResources, upResources, downResources, afterResources, dump)
			}))
		}
	}
	if len(errs) == 0 {
		if pub, ok := deploy.(publisher); ok {
			errs = util.AppendError(errs, pub.Publish())
		}
	}
	if len(errs) == 0 && o.publish != "" {
		errs = util.AppendError(errs, control.XMLWrap(&suite, "Publish version", func() error {
			// Use plaintext version file packaged with kubernetes.tar.gz
			v, err := ioutil.ReadFile("version")
			if err != nil {
				return err
			}
			log.Printf("Set %s version to %s", o.publish, string(v))
			return gcsWrite(o.publish, v)
		}))
	}

	if len(errs) != 0 {
		return fmt.Errorf("encountered %d errors: %v", len(errs), errs)
	}
	return nil
}

func getKubectlVersion(dp deployer) error {
	cmd, err := dp.KubectlCommand()
	if err != nil {
		return err
	}
	if cmd == nil {
		cmd = exec.Command("./cluster/kubectl.sh")
	}
	cmd.Args = append(cmd.Args, "--match-server-version=false", "version")
	copied := *cmd
	retries := 5
	for {
		_, err := control.Output(&copied)
		if err == nil {
			return nil
		}
		retries--
		if retries == 0 {
			return err
		}
		log.Printf("Failed to reach api. Sleeping for 10 seconds before retrying... (%v)", copied.Args)
		time.Sleep(10 * time.Second)
	}
}

func dumpRemoteLogs(deploy deployer, o options, path, reason string) []error {
	if o.kubemark {
		// For dumping kubemark logs with logexporter, we should use
		// root cluster kubeconfig.
		kubeconfigKubemark := os.Getenv("KUBECONFIG")
		kubeconfigRoot := os.Getenv("KUBEMARK_ROOT_KUBECONFIG")
		if err := os.Setenv("KUBECONFIG", kubeconfigRoot); err != nil {
			return []error{err}
		}
		defer os.Setenv("KUBECONFIG", kubeconfigKubemark)
	}

	if reason != "" {
		reason += " "
	}

	var errs []error

	errs = util.AppendError(errs, control.XMLWrap(&suite, reason+"DumpClusterLogs", func() error {
		return deploy.DumpClusterLogs(path, o.logexporterGCSPath)
	}))

	return errs
}

func listNodes(dp deployer, dump string) error {
	cmd, err := dp.KubectlCommand()
	if err != nil {
		return err
	}
	if cmd == nil {
		cmd = exec.Command("./cluster/kubectl.sh")
	}
	cmd.Args = append(cmd.Args, "--match-server-version=false", "get", "nodes", "-oyaml")
	b, err := control.Output(cmd)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filepath.Join(dump, "nodes.yaml"), b, 0644)
}

func listKubemarkNodes(dp deployer, dump string) error {
	cmd, err := dp.KubectlCommand()
	if err != nil {
		return err
	}
	if cmd == nil {
		cmd = exec.Command("./cluster/kubectl.sh")
	}
	cmd.Args = append(cmd.Args, "--match-server-version=false", "--kubeconfig=./test/kubemark/resources/kubeconfig.kubemark", "get", "nodes", "-oyaml")
	b, err := control.Output(cmd)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filepath.Join(dump, "kubemark_nodes.yaml"), b, 0644)
}

func diffResources(before, clusterUp, clusterDown, after []byte, location string) error {
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

	stdout, cerr := control.Output(exec.Command("diff", "-sw", "-U0", "-F^\\[.*\\]$", bp, ap))
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

func listResources() ([]byte, error) {
	log.Printf("Listing resources...")
	stdout, err := control.Output(exec.Command("./cluster/gce/list-resources.sh"))
	if err != nil {
		return stdout, fmt.Errorf("Failed to list resources (%s):\n%s", err, string(stdout))
	}
	return stdout, err
}

func clusterSize(deploy deployer) (int, error) {
	if err := deploy.TestSetup(); err != nil {
		return -1, err
	}
	o, err := control.Output(exec.Command("kubectl", "get", "nodes", "--no-headers"))
	if err != nil {
		log.Printf("kubectl get nodes failed: %s\n%s", wrapError(err).Error(), string(o))
		return -1, err
	}
	stdout := strings.TrimSpace(string(o))
	log.Printf("Cluster nodes:\n%s", stdout)
	return len(strings.Split(stdout, "\n")), nil
}

// commandError will provide stderr output (if available) from structured
// exit errors
type commandError struct {
	err error
}

func wrapError(err error) *commandError {
	if err == nil {
		return nil
	}
	return &commandError{err: err}
}

func (e *commandError) Error() string {
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

func logDumpPath(provider string) string {
	// Use the log dumping script outside of kubernetes/kubernetes repo.
	// Guarding against K8s provider as the script is tested only for gce
	// and gke cases at the moment.
	if os.Getenv("USE_TEST_INFRA_LOG_DUMPING") == "true" && (provider == "gce" || provider == "gke") {
		if logDumpPath := os.Getenv("LOG_DUMP_SCRIPT_PATH"); logDumpPath != "" {
			return logDumpPath
		}
	}
	return "./cluster/log-dump/log-dump.sh"
}

func defaultDumpClusterLogs(localArtifactsDir, logexporterGCSPath, provider string) error {
	logDumpPath := logDumpPath(provider)
	if _, err := os.Stat(logDumpPath); err != nil {
		log.Printf("Could not find %s.", logDumpPath)
		if cwd, err := os.Getwd(); err == nil {
			log.Printf("CWD: %v", cwd)
		}
		return nil
	}
	var cmd *exec.Cmd
	if logexporterGCSPath != "" {
		log.Printf("Dumping logs from nodes to GCS directly at path: %v", logexporterGCSPath)
		cmd = exec.Command(logDumpPath, localArtifactsDir, logexporterGCSPath)
	} else {
		log.Printf("Dumping logs locally to: %v", localArtifactsDir)
		cmd = exec.Command(logDumpPath, localArtifactsDir)
	}
	return control.FinishRunning(cmd)
}

func chartsTest() error {
	// Run helm tests.
	cmdline := util.K8s("charts", "test", "helm-test-e2e.sh")
	return control.FinishRunning(exec.Command(cmdline))
}

func nodeTest(nodeArgs []string, testArgs, nodeTestArgs, project, zone, runtimeConfig string) error {
	// Run node e2e tests.
	// TODO(krzyzacy): remove once nodeTest is stable
	if wd, err := os.Getwd(); err == nil {
		log.Printf("cwd : %s", wd)
	}

	sshKeyPath := os.Getenv("JENKINS_GCE_SSH_PRIVATE_KEY_FILE")
	if _, err := os.Stat(sshKeyPath); err != nil {
		return fmt.Errorf("Cannot find ssh key from: %v, err : %v", sshKeyPath, err)
	}

	artifactsDir, ok := os.LookupEnv("ARTIFACTS")
	if !ok {
		// TODO(krzyzacy): old behavior, consider deprecate
		artifactsDir = filepath.Join(os.Getenv("WORKSPACE"), "_artifacts")
	}

	var sshUser string
	// Use the KUBE_SSH_USER environment variable if it is set. This is particularly
	// required for Fedora CoreOS hosts that only have the user 'core`. Tests
	// using Fedora CoreOS as a host for node tests must set KUBE_SSH_USER
	// environment variable so that test infrastructure can communicate with the host
	// successfully using ssh.
	if os.Getenv("KUBE_SSH_USER") != "" {
		sshUser = os.Getenv("KUBE_SSH_USER")
	} else {
		sshUser = os.Getenv("USER")
	}

	// prep node args
	runner := []string{
		"run",
		util.K8s("kubernetes", "test", "e2e_node", "runner", "remote", "run_remote.go"),
		"--cleanup",
		"--logtostderr",
		"--vmodule=*=4",
		"--ssh-env=gce",
		fmt.Sprintf("--results-dir=%s", artifactsDir),
		fmt.Sprintf("--project=%s", project),
		fmt.Sprintf("--zone=%s", zone),
		fmt.Sprintf("--ssh-user=%s", sshUser),
		fmt.Sprintf("--ssh-key=%s", sshKeyPath),
		fmt.Sprintf("--ginkgo-flags=%s", testArgs),
		fmt.Sprintf("--test_args=%s", nodeTestArgs),
		fmt.Sprintf("--test-timeout=%s", timeout.String()),
	}

	if runtimeConfig != "" {
		runner = append(runner, fmt.Sprintf("--runtime-config=%s", runtimeConfig))
	}

	runner = append(runner, nodeArgs...)

	return control.FinishRunning(exec.Command("go", runner...))
}

func kubemarkUp(dump string, o options, deploy deployer) error {
	// Stop previously running kubemark cluster (if any).
	if err := control.XMLWrap(&suite, "Kubemark TearDown Previous", func() error {
		if err := control.FinishRunning(exec.Command("./test/kubemark/stop-kubemark.sh")); err != nil {
			return fmt.Errorf("failed to stop kubemark cluster, err: %v", err)
		}
		return nil
	}); err != nil {
		return err
	}

	log.Printf("finished tearing down kubemark")

	if err := control.XMLWrap(&suite, "IsUp", deploy.IsUp); err != nil {
		return err
	}

	// Start kubemark cluster.
	if err := control.XMLWrap(&suite, "Kubemark Up", func() error {
		return control.FinishRunning(exec.Command("./test/kubemark/start-kubemark.sh"))
	}); err != nil {
		return err
	}

	// Check kubemark apiserver reachability by listing all nodes.
	if dump != "" {
		control.XMLWrap(&suite, "list kubemark nodes", func() error {
			return listKubemarkNodes(deploy, dump)
		})
	}

	// detect master IP
	if err := os.Setenv("MASTER_NAME", os.Getenv("INSTANCE_PREFIX")+"-kubemark-master"); err != nil {
		return err
	}

	var masterIP, masterInternalIP []byte

	if o.deployment == "bash" && o.provider == "gce" {
		var err error
		masterIP, err = control.Output(exec.Command(
			"gcloud", "compute", "addresses", "describe",
			os.Getenv("MASTER_NAME")+"-ip",
			"--project="+o.gcpProject,
			"--region="+o.gcpZone[:len(o.gcpZone)-2],
			"--format=value(address)"))
		if err != nil {
			return fmt.Errorf("failed to get masterIP: %v", err)
		}

		masterInternalIP, err = control.Output(exec.Command(
			"gcloud", "compute", "instances", "describe",
			os.Getenv("MASTER_NAME"),
			"--project="+o.gcpProject,
			"--zone="+o.gcpZone,
			"--format=value(networkInterfaces[0].networkIP)"))
		if err != nil {
			return fmt.Errorf("failed to get masterInternalIP: %v", err)
		}
	} else if o.deployment == "aks" {
		var err error
		masterIP, err = control.Output(exec.Command(
			"az", "aks", "show",
			"-g", *aksResourceGroupName,
			"-n", *aksResourceName,
			"--query", "fqdn", "-o", "tsv"))
		if err != nil {
			return fmt.Errorf("failed to get masterIP: %v", err)
		}
		masterInternalIP = masterIP
	}

	if err := os.Setenv("KUBE_MASTER_IP", strings.TrimSpace(string(masterIP))); err != nil {
		return err
	}

	// MASTER_IP variable is required by the clusterloader. It requires to have master ip provided,
	// due to master being unregistered.
	if err := os.Setenv("MASTER_IP", strings.TrimSpace(string(masterIP))); err != nil {
		return err
	}

	// MASTER_INTERNAL_IP variable is needed by the clusterloader2 when running on kubemark clusters.
	if err := os.Setenv("MASTER_INTERNAL_IP", strings.TrimSpace(string(masterInternalIP))); err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// Remember root cluster kubeconfig, this nescessary for dumping logs with logexporter.
	if err := os.Setenv("KUBEMARK_ROOT_KUBECONFIG", os.Getenv("KUBECONFIG")); err != nil {
		return err
	}

	if err := os.Setenv("KUBECONFIG", fmt.Sprintf("%s/test/kubemark/resources/kubeconfig.kubemark", cwd)); err != nil {
		return err
	}

	// 'Stop kubemark cluster' step has now been moved outside this function
	// to make it asynchronous with other steps (to speed test execution).
	return nil
}

func kubemarkGinkgoTest(testArgs []string, dump string) error {
	if os.Getenv("ENABLE_KUBEMARK_CLUSTER_AUTOSCALER") == "true" {
		testArgs = append(testArgs, "--kubemark-external-kubeconfig="+os.Getenv("DEFAULT_KUBECONFIG"))
	}

	// Run tests on the kubemark cluster.
	return control.XMLWrap(&suite, "Kubemark Test", func() error {
		testArgs = util.SetFieldDefault(testArgs, "--ginkgo.focus", "starting\\s30\\spods")
		// TODO(krzyzacy): unsure if the envs in kubemark/util.sh makes a difference to e2e tests
		//                 will verify and remove (or uncomment) next
		//util := os.Getenv("WORKSPACE") + "/kubernetes/cluster/kubemark/util.sh"
		//testArgs = append([]string{"-c", "source", util, " ; ./hack/ginkgo-e2e.sh"}, testArgs...)
		cmd := exec.Command("./hack/ginkgo-e2e.sh", testArgs...)
		cmd.Env = append(
			os.Environ(),
			"KUBERNETES_PROVIDER=kubemark",
			"KUBE_CONFIG_FILE=config-default.sh",
			"KUBE_MASTER_URL=https://"+os.Getenv("KUBE_MASTER_IP"),
		)

		return control.FinishRunning(cmd)
	})
}

// Brings down the kubemark cluster.
func kubemarkDown(err *error, wg *sync.WaitGroup, provider, dump string) {
	defer wg.Done()
	control.XMLWrap(&suite, "Kubemark MasterLogDump", func() error {
		logDumpPath := logDumpPath(provider)
		cmd := exec.Command(logDumpPath, dump)
		masterName := os.Getenv("MASTER_NAME")
		cmd.Env = append(
			os.Environ(),
			"KUBEMARK_MASTER_NAME="+masterName,
			"DUMP_ONLY_MASTER_LOGS=true",
		)
		log.Printf("Dumping logs for kubemark master: %s", masterName)
		return control.FinishRunning(cmd)
	})
	*err = control.XMLWrap(&suite, "Kubemark TearDown", func() error {
		return control.FinishRunning(exec.Command("./test/kubemark/stop-kubemark.sh"))
	})
}

// Runs tests in the kubernetes_skew directory, appending --report-prefix flag to the run
func skewTest(args []string, prefix string, checkSkew bool) error {
	return skewTestEnv(nil, args, prefix, checkSkew)
}

// Runs tests in the kubernetes_skew directory, appending --report-prefix flag to the run
func skewTestEnv(env, args []string, prefix string, checkSkew bool) error {
	// TODO(fejta): run this inside this kubetest process, do not spawn a new one.
	popS, err := util.Pushd("../kubernetes_skew")
	if err != nil {
		return err
	}
	defer popS()
	args = util.AppendField(args, "--report-prefix", prefix)
	cmd := exec.Command(
		"kubetest",
		"--test",
		"--test_args="+strings.Join(args, " "),
		fmt.Sprintf("--check-version-skew=%t", checkSkew),
	)
	cmd.Env = env
	return control.FinishRunning(cmd)
}

// GinkgoScriptTester implements Tester by calling the hack/ginkgo-e2e.sh script
type GinkgoScriptTester struct {
}

// Run executes ./hack/ginkgo-e2e.sh
func (t *GinkgoScriptTester) Run(control *process.Control, testArgs []string) error {
	return control.FinishRunning(exec.Command("./hack/ginkgo-e2e.sh", testArgs...))
}

// toBuildTesterOptions builds the BuildTesterOptions data structure for passing to BuildTester
func toBuildTesterOptions(o *options) *e2e.BuildTesterOptions {
	return &e2e.BuildTesterOptions{
		FocusRegex:            o.focusRegex,
		SkipRegex:             o.skipRegex,
		Parallelism:           o.ginkgoParallel.Get(),
		StorageTestDriverPath: o.storageTestDriverPath,
	}
}
