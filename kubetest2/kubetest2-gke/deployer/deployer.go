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

// Package deployer implements the kubetest2 GKE deployer
package deployer

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	realexec "os/exec" // Only for ExitError; Use kubetest2/pkg/exec to actually exec stuff
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/pflag"

	"k8s.io/test-infra/kubetest2/pkg/build"
	"k8s.io/test-infra/kubetest2/pkg/exec"
	"k8s.io/test-infra/kubetest2/pkg/metadata"
	"k8s.io/test-infra/kubetest2/pkg/types"
)

// Name is the name of the deployer
const Name = "gke"

const (
	defaultPool   = "default"
	e2eAllow      = "tcp:22,tcp:80,tcp:8080,tcp:30000-32767,udp:30000-32767"
	defaultCreate = "container clusters create --quiet"
	image         = "cos"
)

var (
	// poolRe matches instance group URLs of the form `https://www.googleapis.com/compute/v1/projects/some-project/zones/a-zone/instanceGroupManagers/gke-some-cluster-some-pool-90fcb815-grp`. Match meaning:
	// m[0]: path starting with zones/
	// m[1]: zone
	// m[2]: pool name (passed to e2es)
	// m[3]: unique hash (used as nonce for firewall rules)
	poolRe = regexp.MustCompile(`zones/([^/]+)/instanceGroupManagers/(gke-.*-([0-9a-f]{8})-grp)$`)

	urlRe           = regexp.MustCompile(`https://.*/`)
	defaultNodePool = gkeNodePool{
		Nodes:       3,
		MachineType: "n1-standard-2",
	}
)

type gkeNodePool struct {
	Nodes       int
	MachineType string
}

type ig struct {
	path string
	zone string
	name string
	uniq string
}

type deployer struct {
	// generic parts
	commonOptions types.Options
	// gke specific details
	project           string
	zone              string
	region            string
	cluster           string
	network           string
	environment       string
	createCommandFlag string
	gcpServiceAccount string

	kubecfgPath    string
	gcpPrepared    bool
	testPrepared   bool
	instanceGroups []*ig

	stageLocation string

	localLogsDir string
	gcsLogsDir   string
}

// New implements deployer.New for gke
func New(opts types.Options) (types.Deployer, *pflag.FlagSet) {
	// create a deployer object and set fields that are not flag controlled
	d := &deployer{
		commonOptions: opts,
		localLogsDir:  filepath.Join(opts.ArtifactsDir(), "logs"),
	}

	// register flags and return
	return d, bindFlags(d)
}

// verifyFlags validates that required flags are set, as well as
// ensuring that location() will not return errors.
func (d *deployer) verifyFlags() error {
	if d.cluster == "" {
		return fmt.Errorf("--cluster-name must be set for GKE deployment")
	}
	if d.project == "" {
		return fmt.Errorf("--project must be set for GKE deployment")
	}
	if _, err := d.location(); err != nil {
		return err
	}
	return nil
}

func (d *deployer) location() (string, error) {
	if d.zone == "" && d.region == "" {
		return "", fmt.Errorf("--zone or --region must be set for GKE deployment")
	} else if d.zone != "" && d.region != "" {
		return "", fmt.Errorf("--zone and --region cannot both be set")
	}

	if d.zone != "" {
		return "--zone=" + d.zone, nil
	}
	return "--region=" + d.region, nil
}

func (d *deployer) createCommand() []string {
	return strings.Fields(d.createCommandFlag)
}

// assert that New implements types.NewDeployer
var _ types.NewDeployer = New

func bindFlags(d *deployer) *pflag.FlagSet {
	flags := pflag.NewFlagSet(Name, pflag.ContinueOnError)

	flags.StringVar(&d.cluster, "cluster-name", "", "Cluster name. Must be set.")
	flags.StringVar(&d.createCommandFlag, "create-command", defaultCreate, "gcloud subcommand used to create a cluster. Modify if you need to pass arbitrary arguments to create.")
	flags.StringVar(&d.gcpServiceAccount, "gcp-service-account", "", "Service account to activate before using gcloud")
	flags.StringVar(&d.network, "network", "default", "Cluster network. Defaults to the default network.")
	flags.StringVar(&d.environment, "environment", "staging", "Container API endpoint to use, one of 'test', 'staging', 'prod', or a custom https:// URL")
	flags.StringVar(&d.project, "project", "", "Project to deploy to.")
	flags.StringVar(&d.region, "region", "", "For use with gcloud commands")
	flags.StringVar(&d.zone, "zone", "", "For use with gcloud commands")
	flags.StringVar(&d.stageLocation, "stage", "", "Upload binaries to gs://bucket/ci/job-suffix if set")
	return flags
}

// assert that deployer implements types.Deployer
var _ types.Deployer = &deployer{}

func (d *deployer) Provider() string {
	return "gke"
}

func (d *deployer) Build() error {
	if err := build.Build(); err != nil {
		return err
	}

	if d.stageLocation != "" {
		if err := build.Stage(d.stageLocation); err != nil {
			return fmt.Errorf("error staging build: %v", err)
		}
	}
	return nil
}

// Deployer implementation methods below
func (d *deployer) Up() error {
	if err := d.verifyFlags(); err != nil {
		return err
	}
	if err := d.prepareGcpIfNeeded(); err != nil {
		return err
	}

	// Create network if it doesn't exist.
	if runWithNoOutput(exec.Command("gcloud", "compute", "networks", "describe", d.network,
		"--project="+d.project,
		"--format=value(name)")) != nil {
		// Assume error implies non-existent.
		log.Printf("Couldn't describe network '%s', assuming it doesn't exist and creating it", d.network)
		if err := runWithOutput(exec.Command("gcloud", "compute", "networks", "create", d.network,
			"--project="+d.project,
			"--subnet-mode=auto")); err != nil {
			return err
		}
	}

	loc, err := d.location()
	if err != nil {
		return err
	}
	args := make([]string, len(d.createCommand()))
	copy(args, d.createCommand())
	args = append(args,
		"--project="+d.project,
		loc,
		"--machine-type="+defaultNodePool.MachineType,
		"--image-type="+image,
		"--num-nodes="+strconv.Itoa(defaultNodePool.Nodes),
		"--network="+d.network,
	)
	fmt.Printf("Environment: %v", os.Environ())

	args = append(args, d.cluster)
	fmt.Printf("Gcloud command: gcloud %+v", args)
	if err := runWithOutput(exec.Command("gcloud", args...)); err != nil {
		return fmt.Errorf("error creating cluster: %v", err)
	}
	return nil
}

func (d *deployer) IsUp() (up bool, err error) {
	// naively assume that if the api server reports nodes, the cluster is up
	lines, err := exec.CombinedOutputLines(
		exec.Command("kubectl", "get", "nodes", "-o=name"),
	)
	if err != nil {
		return false, metadata.NewJUnitError(err, strings.Join(lines, "\n"))
	}
	return len(lines) > 0, nil
}

// DumpClusterLogs for GKE generates a small script that wraps
// log-dump.sh with the appropriate shell-fu to get the cluster
// dumped.
//
// TODO(RonWeber): This whole path is really gross, but this seemed
// the least gross hack to get this done.
//
// TODO(RonWeber): Make this work with multizonal and regional clusters.
func (d *deployer) DumpClusterLogs() error {
	// gkeLogDumpTemplate is a template of a shell script where
	// - %[1]s is the project
	// - %[2]s is the zone
	// - %[3]s is a filter composed of the instance groups
	// - %[4]s is the log-dump.sh command line
	const gkeLogDumpTemplate = `
function log_dump_custom_get_instances() {
  if [[ $1 == "master" ]]; then
    return 0
  fi

  gcloud compute instances list '--project=%[1]s' '--filter=%[4]s' '--format=get(name)'
}
export -f log_dump_custom_get_instances
# Set below vars that log-dump.sh expects in order to use scp with gcloud.
export PROJECT=%[1]s
export ZONE='%[2]s'
export KUBERNETES_PROVIDER=gke
export KUBE_NODE_OS_DISTRIBUTION='%[3]s'
%[5]s
`
	// Prevent an obvious injection.
	if strings.Contains(d.localLogsDir, "'") || strings.Contains(d.gcsLogsDir, "'") {
		return fmt.Errorf("%q or %q contain single quotes - nice try", d.localLogsDir, d.gcsLogsDir)
	}

	// Generate a slice of filters to be OR'd together below
	if err := d.getInstanceGroups(); err != nil {
		return err
	}
	var filters []string
	for _, ig := range d.instanceGroups {
		filters = append(filters, fmt.Sprintf("(metadata.created-by:*%s)", ig.path))
	}

	// Generate the log-dump.sh command-line
	dumpCmd := fmt.Sprintf("./cluster/log-dump/log-dump.sh '%s'", d.localLogsDir)
	if d.gcsLogsDir != "" {
		dumpCmd += " " + d.gcsLogsDir
	}

	return runWithOutput(exec.Command("bash", "-c", fmt.Sprintf(gkeLogDumpTemplate,
		d.project,
		d.zone,
		os.Getenv("NODE_OS_DISTRIBUTION"),
		strings.Join(filters, " OR "),
		dumpCmd)))
}

func (d *deployer) TestSetup() error {
	if d.testPrepared {
		// Ensure setup is a singleton.
		return nil
	}
	if _, err := d.Kubeconfig(); err != nil {
		return err
	}
	if err := d.getInstanceGroups(); err != nil {
		return err
	}
	if err := d.ensureFirewall(); err != nil {
		return err
	}
	d.testPrepared = true
	return nil
}

// Kubeconfig returns a path to a kubeconfig file for the cluster in
// a temp directory, creating one if one does not exist.
// It also sets the KUBECONFIG environment variable appropriately.
func (d *deployer) Kubeconfig() (string, error) {
	if err := d.prepareGcpIfNeeded(); err != nil {
		return "", err
	}

	if d.kubecfgPath != "" {
		return d.kubecfgPath, nil
	}

	tmpdir, err := ioutil.TempDir("", "kubetest2-gke")
	if err != nil {
		return "", err
	}

	// Get gcloud to create the file.
	loc, err := d.location()
	if err != nil {
		return "", err
	}

	filename := filepath.Join(tmpdir, "kubecfg")
	if err := os.Setenv("KUBECONFIG", filename); err != nil {
		return "", err
	}
	if err := runWithOutput(exec.Command("gcloud", d.containerArgs("clusters", "get-credentials", d.cluster, "--project="+d.project, loc)...)); err != nil {
		return "", fmt.Errorf("error executing get-credentials: %v", err)
	}
	d.kubecfgPath = filename
	return d.kubecfgPath, nil
}

func (d *deployer) ensureFirewall() error {
	if d.network == "default" {
		return nil
	}
	firewall, err := d.getClusterFirewall()
	if err != nil {
		return fmt.Errorf("error getting unique firewall: %v", err)
	}
	if runWithNoOutput(exec.Command("gcloud", "compute", "firewall-rules", "describe", firewall,
		"--project="+d.project,
		"--format=value(name)")) == nil {
		// Assume that if this unique firewall exists, it's good to go.
		return nil
	}
	log.Printf("Couldn't describe firewall '%s', assuming it doesn't exist and creating it", firewall)

	tagOut, err := exec.Output(exec.Command("gcloud", "compute", "instances", "list",
		"--project="+d.project,
		"--filter=metadata.created-by:*"+d.instanceGroups[0].path,
		"--limit=1",
		"--format=get(tags.items)"))
	if err != nil {
		return fmt.Errorf("instances list failed: %s", execError(err))
	}
	tag := strings.TrimSpace(string(tagOut))
	if tag == "" {
		return fmt.Errorf("instances list returned no instances (or instance has no tags)")
	}

	if err := runWithOutput(exec.Command("gcloud", "compute", "firewall-rules", "create", firewall,
		"--project="+d.project,
		"--network="+d.network,
		"--allow="+e2eAllow,
		"--target-tags="+tag)); err != nil {
		return fmt.Errorf("error creating e2e firewall: %v", err)
	}
	return nil
}

func (d *deployer) getInstanceGroups() error {
	if len(d.instanceGroups) > 0 {
		return nil
	}
	location, err := d.location()
	if err != nil {
		return err
	}
	igs, err := exec.Output(exec.Command("gcloud", d.containerArgs("clusters", "describe", d.cluster,
		"--format=value(instanceGroupUrls)",
		"--project="+d.project,
		location)...))
	if err != nil {
		return fmt.Errorf("instance group URL fetch failed: %s", execError(err))
	}
	igURLs := strings.Split(strings.TrimSpace(string(igs)), ";")
	if len(igURLs) == 0 {
		return fmt.Errorf("no instance group URLs returned by gcloud, output %q", string(igs))
	}
	sort.Strings(igURLs)
	for _, igURL := range igURLs {
		m := poolRe.FindStringSubmatch(igURL)
		if len(m) == 0 {
			return fmt.Errorf("instanceGroupUrl %q did not match regex %v", igURL, poolRe)
		}
		d.instanceGroups = append(d.instanceGroups, &ig{path: m[0], zone: m[1], name: m[2], uniq: m[3]})
	}
	return nil
}

func (d *deployer) getClusterFirewall() (string, error) {
	if err := d.getInstanceGroups(); err != nil {
		return "", err
	}
	// We want to ensure that there's an e2e-ports-* firewall rule
	// that maps to the cluster nodes, but the target tag for the
	// nodes can be slow to get. Use the hash from the lexically first
	// node pool instead.
	return "e2e-ports-" + d.instanceGroups[0].uniq, nil
}

// This function ensures that all firewall-rules are deleted from specific network.
// We also want to keep in logs that there were some resources leaking.
func (d *deployer) cleanupNetworkFirewalls() (int, error) {
	fws, err := exec.Output(exec.Command("gcloud", "compute", "firewall-rules", "list",
		"--format=value(name)",
		"--project="+d.project,
		"--filter=network:"+d.network))
	if err != nil {
		return 0, fmt.Errorf("firewall rules list failed: %s", execError(err))
	}
	if len(fws) > 0 {
		fwList := strings.Split(strings.TrimSpace(string(fws)), "\n")
		log.Printf("Network %s has %v undeleted firewall rules %v", d.network, len(fwList), fwList)
		commandArgs := []string{"compute", "firewall-rules", "delete", "-q"}
		commandArgs = append(commandArgs, fwList...)
		commandArgs = append(commandArgs, "--project="+d.project)
		errFirewall := runWithOutput(exec.Command("gcloud", commandArgs...))
		if errFirewall != nil {
			return 0, fmt.Errorf("error deleting firewall: %v", errFirewall)
		}
		return len(fwList), nil
	}
	return 0, nil
}

func (d *deployer) Down() error {
	if err := d.verifyFlags(); err != nil {
		return err
	}
	if err := d.prepareGcpIfNeeded(); err != nil {
		return err
	}
	firewall, err := d.getClusterFirewall()
	if err != nil {
		// This is expected if the cluster doesn't exist.
		return nil
	}
	d.instanceGroups = nil

	loc, err := d.location()
	if err != nil {
		return err
	}

	// We best-effort try all of these and report errors as appropriate.
	errCluster := runWithOutput(exec.Command(
		"gcloud", d.containerArgs("clusters", "delete", "-q", d.cluster,
			"--project="+d.project,
			loc)...))

	// don't delete default network
	if d.network == "default" {
		if errCluster != nil {
			log.Printf("Error deleting cluster using default network, allow the error for now %s", errCluster)
		}
		return nil
	}

	var errFirewall error
	if runWithNoOutput(exec.Command("gcloud", "compute", "firewall-rules", "describe", firewall,
		"--project="+d.project,
		"--format=value(name)")) == nil {
		log.Printf("Found rules for firewall '%s', deleting them", firewall)
		errFirewall = exec.Command("gcloud", "compute", "firewall-rules", "delete", "-q", firewall,
			"--project="+d.project).Run()
	} else {
		log.Printf("Found no rules for firewall '%s', assuming resources are clean", firewall)
	}
	numLeakedFWRules, errCleanFirewalls := d.cleanupNetworkFirewalls()
	var errSubnet error
	errNetwork := runWithOutput(exec.Command("gcloud", "compute", "networks", "delete", "-q", d.network,
		"--project="+d.project))
	if errCluster != nil {
		return fmt.Errorf("error deleting cluster: %v", errCluster)
	}
	if errFirewall != nil {
		return fmt.Errorf("error deleting firewall: %v", errFirewall)
	}
	if errCleanFirewalls != nil {
		return fmt.Errorf("error cleaning-up firewalls: %v", errCleanFirewalls)
	}
	if errSubnet != nil {
		return fmt.Errorf("error deleting subnetwork: %v", errSubnet)
	}
	if errNetwork != nil {
		return fmt.Errorf("error deleting network: %v", errNetwork)
	}
	if numLeakedFWRules > 0 {
		return fmt.Errorf("leaked firewall rules")
	}
	return nil
}

func (d *deployer) containerArgs(args ...string) []string {
	return append(append([]string{}, "container"), args...)
}

func runWithNoOutput(cmd exec.Cmd) error {
	exec.NoOutput(cmd)
	return cmd.Run()
}

func runWithOutput(cmd exec.Cmd) error {
	exec.InheritOutput(cmd)
	return cmd.Run()
}

// execError returns a string format of err including stderr if the
// err is an ExitError, useful for errors from e.g. exec.Cmd.Output().
func execError(err error) string {
	if ee, ok := err.(*realexec.ExitError); ok {
		return fmt.Sprintf("%v (output: %q)", err, string(ee.Stderr))
	}
	return err.Error()
}
