/*
Copyright 2017 The Kubernetes Authors.

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

// Package main / gke.go provides the Google Container Engine (GKE)
// kubetest deployer via newGKE().
//
// TODO(zmerlynn): Pull this out to a separate package?
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"k8s.io/test-infra/kubetest/util"
)

const (
	defaultPool   = "default"
	e2eAllow      = "tcp:22,tcp:80,tcp:8080,tcp:9090,tcp:30000-32767,udp:30000-32767"
	defaultCreate = "container clusters create --quiet"
)

var (
	gkeAdditionalZones             = flag.String("gke-additional-zones", "", "(gke only) List of additional Google Compute Engine zones to use. Clusters are created symmetrically across zones by default, see --gke-shape for details.")
	gkeNodeLocations               = flag.String("gke-node-locations", "", "(gke only) List of Google Compute Engine zones to use.")
	gkeEnvironment                 = flag.String("gke-environment", "", "(gke only) Container API endpoint to use, one of 'test', 'staging', 'prod', or a custom https:// URL")
	gkeShape                       = flag.String("gke-shape", `{"default":{"Nodes":3,"MachineType":"n1-standard-2"}}`, `(gke only) A JSON description of node pools to create. The node pool 'default' is required and used for initial cluster creation. All node pools are symmetric across zones, so the cluster total node count is {total nodes in --gke-shape} * {1 + (length of --gke-additional-zones)}. Example: '{"default":{"Nodes":999,"MachineType:":"n1-standard-1"},"heapster":{"Nodes":1, "MachineType":"n1-standard-8", "ExtraArgs": []}}`)
	gkeCreateArgs                  = flag.String("gke-create-args", "", "(gke only) (deprecated, use a modified --gke-create-command') Additional arguments passed directly to 'gcloud container clusters create'")
	gkeCommandGroup                = flag.String("gke-command-group", "", "(gke only) Use a different gcloud track (e.g. 'alpha') for all 'gcloud container' commands. Note: This is added to --gke-create-command on create. You should only use --gke-command-group if you need to change the gcloud track for *every* gcloud container command.")
	gkeGcloudCommand               = flag.String("gke-gcloud-command", "gcloud", "(gke only) gcloud command used to create a cluster. Modify if you need to pass custom gcloud to create cluster.")
	gkeGcloudArgs                  = flag.String("gke-gcloud-args", "", "(gke only) Additional arguments to custom gcloud command.")
	gkeCreateCommand               = flag.String("gke-create-command", defaultCreate, "(gke only) gcloud subcommand used to create a cluster. Modify if you need to pass arbitrary arguments to create.")
	gkeCustomSubnet                = flag.String("gke-custom-subnet", "", "(gke only) if specified, we create a custom subnet with the specified options and use it for the gke cluster. The format should be '<subnet-name> --region=<subnet-gcp-region> --range=<subnet-cidr> <any other optional params>'.")
	gkeSubnetMode                  = flag.String("gke-subnet-mode", "auto", "(gke only) subnet creation mode of the GKE cluster network.")
	gkeReleaseChannel              = flag.String("gke-release-channel", "", "(gke only) if specified, bring up GKE clusters from that release channel.")
	gkeSingleZoneNodeInstanceGroup = flag.Bool("gke-single-zone-node-instance-group", true, "(gke only) Add instance groups from a single zone to the NODE_INSTANCE_GROUP env variable.")
	gkeInstanceGroupPrefix         = flag.String("gke-instance-group-prefix", "gke", "(gke only) Use a different instance group prefix.")
	gkeNodePorts                   = flag.String("gke-node-ports", "", "(gke only) List of ports on nodes to open, allowing e.g. master to connect to pods on private nodes. The format should be 'protocol[:port[-port]],[...]' as in gcloud compute firewall-rules create --allow.")
	gkeCreateNat                   = flag.Bool("gke-create-nat", false, "(gke only) Configure Cloud NAT allowing outbound connections in cluster with private nodes.")
	gkeCreateNatBeforeCluster      = flag.Bool("gke-create-nat-before-cluster", false, "(gke only) Create NAT before creating the cluster.")
	gkeNodeTagFromFirewallRules    = flag.Bool("gke-node-tag-from-firewall-rules", false, "(gke only) Get node tag for creating firewall rules from already exisiting firewall rules.")
	gkeNatMinPortsPerVm            = flag.Int("gke-nat-min-ports-per-vm", 64, "(gke only) Specify number of ports per cluster VM for NAT router. Number of ports * number of nodes / 64k = number of auto-allocated IP addresses (there is a hard limit of 100 IPs).")
	gkeDownTimeout                 = flag.Duration("gke-down-timeout", 1*time.Hour, "(gke only) Timeout for gcloud container clusters delete call. Defaults to 1 hour which matches gcloud's default.")
	gkeRemoveNetwork               = flag.Bool("gke-remove-network", true, "(gke only) At the end of the test remove non-default network that was used by cluster.")
	gkeDumpConfigMaps              = flag.String("gke-dump-configmaps", "[]", `(gke-only) A JSON description of ConfigMaps to dump as part of gathering cluster logs. Note: --dump or --dump-pre-test-logs flags must also be set. Example: '[{"Name":"my-map", "Namespace":"default", "DataKey":"my-data-key"}]`)
	gkeDumpAdditionalLogsCmd       = flag.String("gke-dump-additional-logs-cmd", "", "(gke-only) if set, run this command to dump cluster logs.")

	// poolReTemplate matches instance group URLs of the form `https://www.googleapis.com/compute/v1/projects/some-project/zones/a-zone/instanceGroupManagers/gke-some-cluster-some-pool-90fcb815-grp`. Match meaning:
	// m[0]: path starting with zones/
	// m[1]: zone
	// m[2]: pool name (passed to e2es)
	// m[3]: unique hash (used as nonce for firewall rules)
	poolReTemplate = `zones/([^/]+)/instanceGroupManagers/(%s-.*-([0-9a-f]{8})-grp)$`

	urlRe = regexp.MustCompile(`https://.*/`)
)

type gkeNodePool struct {
	Nodes       int
	MachineType string
	ExtraArgs   []string
}

type gkeConfigMap struct {
	Name      string
	Namespace string
	DataKey   string
}

type gkeDeployer struct {
	project                     string
	zone                        string
	region                      string
	locationRaw                 string
	location                    string
	additionalZones             string
	nodeLocations               string
	nodePorts                   string
	cluster                     string
	shape                       map[string]gkeNodePool
	network                     string
	subnetwork                  string
	subnetMode                  string
	subnetworkRegion            string
	createNat                   bool
	natMinPortsPerVm            int
	image                       string
	imageFamily                 string
	imageProject                string
	commandGroup                []string
	createCommand               []string
	singleZoneNodeInstanceGroup bool
	sshProxyInstanceName        string
	poolRe                      *regexp.Regexp
	dumpedConfigMaps            []gkeConfigMap

	setup          bool
	kubecfg        string
	instanceGroups []*ig
}

type ig struct {
	path string
	zone string
	name string
	uniq string
}

var _ deployer = &gkeDeployer{}

func newGKE(provider, project, zone, region, network, image, imageFamily, imageProject, cluster, sshProxyInstanceName string, testArgs *string, upgradeArgs *string) (*gkeDeployer, error) {
	if provider != "gke" {
		return nil, fmt.Errorf("--provider must be 'gke' for GKE deployment, found %q", provider)
	}
	g := &gkeDeployer{}

	if cluster == "" {
		return nil, fmt.Errorf("--cluster must be set for GKE deployment")
	}
	g.cluster = cluster

	if project == "" {
		return nil, fmt.Errorf("--gcp-project must be set for GKE deployment")
	}
	g.project = project

	if zone == "" && region == "" {
		return nil, fmt.Errorf("--gcp-zone or --gcp-region must be set for GKE deployment")
	} else if zone != "" && region != "" {
		return nil, fmt.Errorf("--gcp-zone and --gcp-region cannot both be set")
	}
	if zone != "" {
		g.zone = zone
		g.locationRaw = zone
		g.location = "--zone=" + zone
	} else if region != "" {
		g.region = region
		g.locationRaw = region
		g.location = "--region=" + region
	}

	if network == "" {
		return nil, fmt.Errorf("--gcp-network must be set for GKE deployment")
	}
	g.network = network

	if strings.ToUpper(image) == "CUSTOM" {
		if imageFamily == "" || imageProject == "" {
			return nil, fmt.Errorf("--image-family and --image-project must be set for GKE deployment if --gcp-node-image=CUSTOM")
		}
	}

	poolRe, err := regexp.Compile(fmt.Sprintf(poolReTemplate, *gkeInstanceGroupPrefix))
	if err != nil {
		return nil, fmt.Errorf("Couldn't compile regex %v. prefix: %s", err, *gkeInstanceGroupPrefix)
	}
	g.poolRe = poolRe
	g.imageFamily = imageFamily
	g.imageProject = imageProject
	g.image = image

	g.additionalZones = *gkeAdditionalZones
	g.nodeLocations = *gkeNodeLocations
	g.nodePorts = *gkeNodePorts
	g.createNat = *gkeCreateNat
	g.natMinPortsPerVm = *gkeNatMinPortsPerVm

	err = json.Unmarshal([]byte(*gkeShape), &g.shape)
	if err != nil {
		return nil, fmt.Errorf("--gke-shape must be valid JSON, unmarshal error: %v, JSON: %q", err, *gkeShape)
	}
	if _, ok := g.shape[defaultPool]; !ok {
		return nil, fmt.Errorf("--gke-shape must include a node pool named 'default', found %q", *gkeShape)
	}

	switch subnetMode := *gkeSubnetMode; subnetMode {
	case "auto", "custom":
		g.subnetMode = subnetMode
	default:
		return nil, fmt.Errorf("--gke-subnet-mode must be set either to 'auto' or 'custom', got: %s", subnetMode)
	}

	g.commandGroup = strings.Fields(*gkeCommandGroup)

	g.createCommand = append([]string{}, strings.Fields(*gkeGcloudArgs)...)
	g.createCommand = append(g.createCommand, g.commandGroup...)
	g.createCommand = append(g.createCommand, strings.Fields(*gkeCreateCommand)...)
	createArgs := strings.Fields(*gkeCreateArgs)
	if len(createArgs) > 0 {
		log.Printf("--gke-create-args is deprecated, please use '--gke-create-command=%s %s'", defaultCreate, *gkeCreateArgs)
	}
	g.createCommand = append(g.createCommand, createArgs...)

	if err := util.MigrateOptions([]util.MigratedOption{{
		Env:    "CLOUDSDK_API_ENDPOINT_OVERRIDES_CONTAINER",
		Option: gkeEnvironment,
		Name:   "--gke-environment",
	}}); err != nil {
		return nil, err
	}

	var endpoint string
	switch env := *gkeEnvironment; {
	case env == "test":
		endpoint = "https://test-container.sandbox.googleapis.com/"
	case env == "staging":
		endpoint = "https://staging-container.sandbox.googleapis.com/"
	case env == "staging2":
		endpoint = "https://staging2-container.sandbox.googleapis.com/"
	case env == "prod":
		endpoint = "https://container.googleapis.com/"
	case urlRe.MatchString(env):
		endpoint = env
	default:
		return nil, fmt.Errorf("--gke-environment must be one of {test,staging,prod} or match %v, found %q", urlRe, env)
	}
	if err := os.Setenv("CLOUDSDK_API_ENDPOINT_OVERRIDES_CONTAINER", endpoint); err != nil {
		return nil, err
	}

	// Override kubecfg to a temporary file rather than trashing the user's.
	f, err := os.CreateTemp("", "gke-kubecfg")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	kubecfg := f.Name()
	if err := f.Chmod(0600); err != nil {
		return nil, err
	}
	g.kubecfg = kubecfg

	// We want no KUBERNETES_PROVIDER, but to set
	// KUBERNETES_CONFORMANCE_PROVIDER and
	// KUBERNETES_CONFORMANCE_TEST. This prevents ginkgo-e2e.sh from
	// using the cluster/gke functions.
	//
	// We do this in the deployer constructor so that
	// cluster/gce/list-resources.sh outputs the same provider for the
	// extent of the binary. (It seems like it belongs in TestSetup,
	// but that way leads to madness.)
	//
	// TODO(zmerlynn): This is gross.
	if err := os.Unsetenv("KUBERNETES_PROVIDER"); err != nil {
		return nil, err
	}
	if err := os.Setenv("KUBERNETES_CONFORMANCE_TEST", "yes"); err != nil {
		return nil, err
	}
	if err := os.Setenv("KUBERNETES_CONFORMANCE_PROVIDER", "gke"); err != nil {
		return nil, err
	}

	// TODO(zmerlynn): Another snafu of cluster/gke/list-resources.sh:
	// Set KUBE_GCE_INSTANCE_PREFIX so that we don't accidentally pick
	// up CLUSTER_NAME later.
	if err := os.Setenv("KUBE_GCE_INSTANCE_PREFIX", "gke-"+g.cluster); err != nil {
		return nil, err
	}

	// set --num-nodes flag for ginkgo, since NUM_NODES is not set for gke deployer.
	numNodes := strconv.Itoa(g.shape[defaultPool].Nodes)
	// testArgs can be empty, and we need to support this case
	*testArgs = strings.Join(util.SetFieldDefault(strings.Fields(*testArgs), "--num-nodes", numNodes), " ")

	if *upgradeArgs != "" {
		// --upgrade-target will be passed to e2e upgrade framework to get a valid update version.
		// See usage from https://github.com/kubernetes/kubernetes/blob/master/hack/get-build.sh for supported targets.
		// Here we special case for gke-latest and will extract an actual valid gke version.
		// - gke-latest will be resolved to the latest gke version, and
		// - gke-latest-1.7 will be resolved to the latest 1.7 patch version supported on gke.
		fields, val, exist := util.ExtractField(strings.Fields(*upgradeArgs), "--upgrade-target")
		if exist {
			if strings.HasPrefix(val, "gke-latest") {
				releasePrefix := ""
				if strings.HasPrefix(val, "gke-latest-") {
					releasePrefix = strings.TrimPrefix(val, "gke-latest-")
				}
				if val, err = getLatestGKEVersion(project, zone, region, releasePrefix); err != nil {
					return nil, fmt.Errorf("fail to get latest gke version : %w", err)
				}
			}
			fields = util.SetFieldDefault(fields, "--upgrade-target", val)
		}
		*upgradeArgs = strings.Join(util.SetFieldDefault(fields, "--num-nodes", numNodes), " ")
	}

	g.singleZoneNodeInstanceGroup = *gkeSingleZoneNodeInstanceGroup
	g.sshProxyInstanceName = sshProxyInstanceName

	err = json.Unmarshal([]byte(*gkeDumpConfigMaps), &g.dumpedConfigMaps)
	if err != nil {
		return nil, fmt.Errorf("--gke-dump-configmaps must be valid JSON, unmarshal error: %v, JSON: %q", err, *gkeDumpConfigMaps)
	}

	return g, nil
}

func (g *gkeDeployer) Up() error {
	// Create network if it doesn't exist.
	if control.NoOutput(exec.Command("gcloud", "compute", "networks", "describe", g.network,
		"--project="+g.project,
		"--format=value(name)")) != nil {
		// Assume error implies non-existent.
		log.Printf("Couldn't describe network '%s', assuming it doesn't exist and creating it", g.network)
		if err := control.FinishRunning(exec.Command("gcloud", "compute", "networks", "create", g.network,
			"--project="+g.project,
			"--subnet-mode="+g.subnetMode)); err != nil {
			return err
		}
	}

	// Export network name for gke kubemark clusters.
	if err := os.Setenv("GKE_NETWORK", g.network); err != nil {
		return err
	}

	// Create a custom subnet in that network if it was asked for.
	if *gkeCustomSubnet != "" {
		customSubnetFields := strings.Fields(*gkeCustomSubnet)
		createSubnetCommand := []string{"compute", "networks", "subnets", "create"}
		createSubnetCommand = append(createSubnetCommand, "--project="+g.project, "--network="+g.network)
		createSubnetCommand = append(createSubnetCommand, customSubnetFields...)
		if err := control.FinishRunning(exec.Command("gcloud", createSubnetCommand...)); err != nil {
			return err
		}
		g.subnetwork = customSubnetFields[0]
		g.subnetworkRegion = customSubnetFields[1]
	}

	if *gkeCreateNatBeforeCluster {
		if err := g.ensureNat(); err != nil {
			return fmt.Errorf("error ensuring NAT exists: %v", err)
		}
	}

	def := g.shape[defaultPool]
	args := make([]string, len(g.createCommand))
	for i := range args {
		args[i] = os.ExpandEnv(g.createCommand[i])
	}
	args = append(args,
		"--project="+g.project,
		g.location,
		"--network="+g.network,
	)
	if def.Nodes > 0 {
		args = append(args, "--num-nodes="+strconv.Itoa(def.Nodes))
	}
	if def.MachineType != "" {
		args = append(args, "--machine-type="+def.MachineType)
	}
	if g.image != "" {
		args = append(args, "--image-type="+g.image)
	}
	args = append(args, def.ExtraArgs...)
	if strings.ToUpper(g.image) == "CUSTOM" {
		args = append(args, "--image-family="+g.imageFamily)
		args = append(args, "--image-project="+g.imageProject)
		// gcloud enables node auto-upgrade by default, which doesn't work with CUSTOM image.
		// We disable auto-upgrade explicitly here.
		args = append(args, "--no-enable-autoupgrade")
		// Custom images are not supported with shielded nodes (which is enaled by default) in GKE.
		args = append(args, "--no-enable-shielded-nodes")
	}
	if g.subnetwork != "" {
		args = append(args, "--subnetwork="+g.subnetwork)
	}
	if g.additionalZones != "" {
		args = append(args, "--additional-zones="+g.additionalZones)
		if err := os.Setenv("MULTIZONE", "true"); err != nil {
			return fmt.Errorf("error setting MULTIZONE env variable: %w", err)
		}

	}
	if g.nodeLocations != "" {
		args = append(args, "--node-locations="+g.nodeLocations)
		numNodeLocations := strings.Split(g.nodeLocations, ",")
		if len(numNodeLocations) > 1 {
			if err := os.Setenv("MULTIZONE", "true"); err != nil {
				return fmt.Errorf("error setting MULTIZONE env variable: %w", err)
			}
		}
	}

	if *gkeReleaseChannel != "" {
		args = append(args, "--release-channel="+*gkeReleaseChannel)
	} else {
		// TODO(zmerlynn): The version should be plumbed through Extract
		// or a separate flag rather than magic env variables.
		if v := os.Getenv("CLUSTER_API_VERSION"); v != "" {
			args = append(args, "--cluster-version="+v)
		}
	}

	args = append(args, g.cluster)
	if err := control.FinishRunning(exec.Command(*gkeGcloudCommand, args...)); err != nil {
		return fmt.Errorf("error creating cluster: %w", err)
	}
	for poolName, pool := range g.shape {
		if poolName == defaultPool {
			continue
		}
		poolArgs := []string{"node-pools", "create", poolName,
			"--cluster=" + g.cluster,
			"--project=" + g.project,
			g.location,
			"--num-nodes=" + strconv.Itoa(pool.Nodes)}
		if pool.MachineType != "" {
			poolArgs = append(poolArgs, "--machine-type="+pool.MachineType)
		}
		poolArgs = append(poolArgs, pool.ExtraArgs...)
		if err := control.FinishRunning(exec.Command("gcloud", g.containerArgs(poolArgs...)...)); err != nil {
			return fmt.Errorf("error creating node pool %q: %w", poolName, err)
		}
	}
	return nil
}

func (g *gkeDeployer) IsUp() error {
	return isUp(g)
}

func (g *gkeDeployer) dumpLogs() error {
	cmdLineTokenized := strings.Fields(os.ExpandEnv(*gkeDumpAdditionalLogsCmd))
	return control.FinishRunning(exec.Command(cmdLineTokenized[0], cmdLineTokenized[1:]...))
}

// DumpClusterLogs for GKE generates a small script that wraps
// log-dump.sh with the appropriate shell-fu to get the cluster
// dumped.
//
// TODO(zmerlynn): This whole path is really gross, but this seemed
// the least gross hack to get this done.
func (g *gkeDeployer) DumpClusterLogs(localPath, gcsPath string) error {
	var errs []error
	if *gkeDumpAdditionalLogsCmd != "" {
		if err := g.dumpLogs(); err != nil {
			// Save error for later and carry on
			errs = append(errs, err)
			log.Printf("Failed to dump additional cluster logs: %v", err)
		}
	}

	// gkeLogDumpTemplate is a template of a shell script where
	// - %[1]s is the project
	// - %[2]s is the zone
	// - %[3]s is the OS distribution of nodes
	// - %[4]s is a filter composed of the instance groups
	// - %[5]s is the log-dump.sh command line
	const gkeLogDumpTemplate = `
function log_dump_custom_get_instances() {
  if [[ $1 == "master" ]]; then return 0; fi
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
	if strings.Contains(localPath, "'") || strings.Contains(gcsPath, "'") {
		errs = append(errs, fmt.Errorf("%q or %q contain single quotes - nice try", localPath, gcsPath))
		return wrapErrors("DumpClusterLogs", errs...)
	}

	// Generate a slice of filters to be OR'd together below
	if err := g.getInstanceGroups(); err != nil {
		errs = append(errs, err)
		return wrapErrors("DumpClusterLogs", errs...)
	}
	perZoneFilters := make(map[string][]string)
	for _, ig := range g.instanceGroups {
		filter := fmt.Sprintf("(metadata.created-by ~ %s)", ig.path)
		perZoneFilters[ig.zone] = append(perZoneFilters[ig.zone], filter)
	}

	// Generate the log-dump.sh command-line
	dumpCmd := logDumpPath("gke")
	if gcsPath == "" {
		dumpCmd = fmt.Sprintf("%s '%s'", dumpCmd, localPath)
	} else {
		dumpCmd = fmt.Sprintf("%s '%s' '%s'", dumpCmd, localPath, gcsPath)
	}

	// Try to setup cluster access if it's possible. If credentials are already set, this will be no-op. Access to
	// GKE cluster is required for log-exporter.
	if err := g.getKubeConfig(); err != nil {
		errs = append(errs, err)
		log.Printf("error while setting up kubeconfig: %v", err)
	}

	// Make sure the firewall rule is created. It's needed so the log-dump.sh can ssh into nodes.
	// If cluster-up operation failed for some reasons (e.g. some nodes didn't register) the
	// firewall rule isn't automatically created as the TestSetup is not being executed. If firewall
	// rule was successfully created, the ensureFirewall call will be no-op.
	if err := g.ensureFirewall(); err != nil {
		log.Printf("error while ensuring firewall rule: %v", err)
	}

	var errorMessages []string
	for zone, filters := range perZoneFilters {
		err := control.FinishRunning(exec.Command("bash", "-c", fmt.Sprintf(gkeLogDumpTemplate,
			g.project,
			zone,
			os.Getenv("NODE_OS_DISTRIBUTION"),
			strings.Join(filters, " OR "),
			dumpCmd)))
		if err != nil {
			errorMessages = append(errorMessages, err.Error())
		}
	}
	if len(errorMessages) > 0 {
		errs = append(errs, fmt.Errorf("errors while dumping logs: %s", strings.Join(errorMessages, ", ")))
		return wrapErrors("DumpClusterLogs", errs...)
	}

	// Fetch any ConfigMap data fields that were requested to be dumped
	errorMessages = nil
	dumpValues := make(map[string]string)
	for _, cm := range g.dumpedConfigMaps {
		cmd := exec.Command("kubectl", "get", fmt.Sprintf("ConfigMaps/%s", cm.Name), "-n", cm.Namespace, "-o", fmt.Sprintf("jsonpath={.data.%s}", cm.DataKey))
		log.Printf("Running: %s", cmd)
		out, err := cmd.Output()
		if err != nil {
			errorMessages = append(errorMessages, util.ExecError(err))
			continue
		}
		jsonKey := strings.Join([]string{cm.Namespace, cm.Name, cm.DataKey}, ".")
		dumpValues[jsonKey] = string(out)
	}
	if len(errorMessages) > 0 {
		errs = append(errs, fmt.Errorf("errors while dumping ConfigMaps: %s", strings.Join(errorMessages, ", ")))
		return wrapErrors("DumpClusterLogs", errs...)
	}

	jsonDump, err := json.Marshal(dumpValues)
	if err != nil {
		errs = append(errs, err)
		return wrapErrors("DumpClusterLogs", errs...)
	}

	if err := os.WriteFile(filepath.Join(localPath, "gke-configmap.json"), jsonDump, 0644); err != nil {
		errs = append(errs, err)
		return wrapErrors("DumpClusterLogs", errs...)
	}

	if len(errs) > 0 {
		return wrapErrors("DumpClusterLogs", errs...)
	}

	return nil
}

func (g *gkeDeployer) TestSetup() error {
	if g.setup {
		// Ensure setup is a singleton.
		return nil
	}
	if err := g.getKubeConfig(); err != nil {
		return err
	}
	if err := g.getInstanceGroups(); err != nil {
		return err
	}
	if err := g.ensureFirewall(); err != nil {
		return err
	}
	if err := g.ensureNat(); err != nil {
		return err
	}
	if err := g.setupBastion(); err != nil {
		return err
	}
	if err := g.setupEnv(); err != nil {
		return err
	}
	g.setup = true
	return nil
}

// setupBastion prepares KUBE_SSH_BASTION env variable with the hostname of some public
// node of the cluster that could be sshed into. Some Kubernetes e2e tests need it.
func (g *gkeDeployer) setupBastion() error {
	if g.sshProxyInstanceName == "" {
		return nil
	}
	var filtersToTry []string
	// Use exact name first, VM does not have to belong to the cluster
	exactFilter := "name=" + g.sshProxyInstanceName
	filtersToTry = append(filtersToTry, exactFilter)
	// As a fallback - use proxy instance name as a regex but check only cluster nodes
	var igFilters []string
	// Filter out VMs not belonging to the GKE cluster
	for _, ig := range g.instanceGroups {
		igFilters = append(igFilters, fmt.Sprintf("(metadata.created-by ~ %s)", ig.path))
	}
	// Match VM name or wildcard passed by kubetest parameters
	fuzzyFilter := fmt.Sprintf("(name ~ %s) AND (%s)",
		g.sshProxyInstanceName,
		strings.Join(igFilters, " OR "))
	filtersToTry = append(filtersToTry, fuzzyFilter)

	// Find hostname of VM that matches criteria
	var bastion, zone string
	for _, filter := range filtersToTry {
		log.Printf("Checking for proxy instance with filter: %q", filter)
		output, err := exec.Command("gcloud", "compute", "instances", "list",
			"--filter="+filter,
			"--format=value(name,zone)",
			"--project="+g.project).Output()
		if err != nil {
			return fmt.Errorf("listing instances failed: %s", util.ExecError(err))
		}
		if len(output) == 0 {
			continue
		}
		instances := strings.Split(string(output), "\n")
		// Proxy instance found
		fields := strings.Split(strings.TrimSpace(string(instances[0])), "\t")
		if len(fields) != 2 {
			return fmt.Errorf("error parsing instances list output %q", output)
		}
		bastion, zone = fields[0], fields[1]
		break
	}
	if bastion == "" {
		return fmt.Errorf("proxy instance %q not found", g.sshProxyInstanceName)
	}
	log.Printf("Found proxy instance %q", bastion)

	log.Printf("Adding NAT access config if not present")
	control.NoOutput(exec.Command("gcloud", "compute", "instances", "add-access-config", bastion,
		"--zone="+zone,
		"--project="+g.project))

	// Set KUBE_SSH_BASTION env parameter
	err := setKubeShhBastionEnv(g.project, zone, bastion)
	if err != nil {
		return fmt.Errorf("setting KUBE_SSH_BASTION variable failed: %s", util.ExecError(err))
	}
	return nil
}

func (g *gkeDeployer) getKubeConfig() error {
	info, err := os.Stat(g.kubecfg)
	if err != nil {
		return err
	}
	if info.Size() > 0 {
		// Assume that if we already have it, it's good.
		return nil
	}
	if err := os.Setenv("KUBECONFIG", g.kubecfg); err != nil {
		return err
	}
	if err := control.FinishRunning(exec.Command("gcloud", g.containerArgs("clusters", "get-credentials", g.cluster,
		"--project="+g.project,
		g.location)...)); err != nil {
		return fmt.Errorf("error executing get-credentials: %w", err)
	}
	return nil
}

// setupEnv is to appease ginkgo-e2e.sh and other pieces of the e2e infrastructure. It
// would be nice to handle this elsewhere, and not with env
// variables. c.f. kubernetes/test-infra#3330.
func (g *gkeDeployer) setupEnv() error {
	// If we don't have instance groups we won't be able to set env variables anyway
	if len(g.instanceGroups) == 0 {
		return nil
	}

	// If singleZoneNodeInstanceGroup is true, set NODE_INSTANCE_GROUP to the
	// names of instance groups that are in the same zone as the lexically first
	// instance group. Otherwise set NODE_INSTANCE_GROUP to the names of all
	// instance groups.
	var filt []string
	zone := g.instanceGroups[0].zone
	for _, ig := range g.instanceGroups {
		if !g.singleZoneNodeInstanceGroup || ig.zone == zone {
			filt = append(filt, ig.name)
		}
	}
	if err := os.Setenv("NODE_INSTANCE_GROUP", strings.Join(filt, ",")); err != nil {
		return fmt.Errorf("error setting NODE_INSTANCE_GROUP: %w", err)
	}
	return nil
}

func (g *gkeDeployer) ensureFirewall() error {
	if g.network == "default" {
		return nil
	}
	firewall := g.getClusterFirewall()

	if control.NoOutput(exec.Command("gcloud", "compute", "firewall-rules", "describe", firewall,
		"--project="+g.project,
		"--format=value(name)")) == nil {
		// Assume that if this unique firewall exists, it's good to go.
		return nil
	}
	log.Printf("Couldn't describe firewall '%s', assuming it doesn't exist and creating it", firewall)

	var err error
	var tagOut []byte
	if *gkeNodeTagFromFirewallRules {
		tagOut, err = exec.Command("gcloud", "compute", "firewall-rules", "list",
			"--project="+g.project,
			"--filter=name ~ "+g.cluster,
			"--limit=1",
			"--format=get(targetTags)").Output()
		if err != nil {
			return fmt.Errorf("firewall-rules list failed: %s", util.ExecError(err))
		}
	} else if len(g.instanceGroups) > 0 {
		tagOut, err = exec.Command("gcloud", "compute", "instances", "list",
			"--project="+g.project,
			"--filter=metadata.created-by ~ "+g.instanceGroups[0].path,
			"--limit=1",
			"--format=get(tags.items)").Output()
		if err != nil {
			return fmt.Errorf("instances list failed: %s", util.ExecError(err))
		}
	} else {
		return fmt.Errorf("either node tag or instance group must be present to create firewall")
	}
	tag := strings.TrimSpace(string(tagOut))
	if tag == "" {
		return fmt.Errorf("instances list returned no instances (or instance has no tags)")
	}

	allowPorts := e2eAllow
	if g.nodePorts != "" {
		allowPorts += "," + g.nodePorts
	}
	if err := control.FinishRunning(exec.Command("gcloud", "compute", "firewall-rules", "create", firewall,
		"--project="+g.project,
		"--network="+g.network,
		"--allow="+allowPorts,
		"--target-tags="+tag)); err != nil {
		return fmt.Errorf("error creating e2e firewall: %w", err)
	}
	return nil
}

func (g *gkeDeployer) getInstanceGroups() error {
	if len(g.instanceGroups) > 0 {
		return nil
	}
	igs, err := g.getInstanceGroupsFromGcloud()
	if err != nil {
		return err
	}

	instanceGroups, err := g.parseInstanceGroupsFromGcloud(igs)
	if err != nil {
		return err
	}
	g.instanceGroups = instanceGroups
	return nil
}

func (g *gkeDeployer) getInstanceGroupsFromGcloud() (string, error) {
	igs, err := exec.Command("gcloud", g.containerArgs("clusters", "describe", g.cluster,
		"--format=value(instanceGroupUrls)",
		"--project="+g.project,
		g.location)...).Output()
	if err != nil {
		return "", fmt.Errorf("instance group URL fetch failed: %s", util.ExecError(err))
	}
	return string(igs), nil
}

func (g *gkeDeployer) parseInstanceGroupsFromGcloud(igs string) ([]*ig, error) {
	igURLs := strings.Split(strings.TrimSpace(igs), ";")
	if len(igURLs) == 0 || len(strings.TrimSpace(igs)) == 0 {
		fmt.Printf("warning: no instance group URLs returned by gcloud, output %q", string(igs))
		return nil, nil
	}
	sort.Strings(igURLs)
	var instanceGroups []*ig
	for _, igURL := range igURLs {
		m := g.poolRe.FindStringSubmatch(igURL)
		if len(m) == 0 {
			return nil, fmt.Errorf("instanceGroupUrl %q did not match regex %v", igURL, g.poolRe)
		}
		instanceGroups = append(instanceGroups, &ig{path: m[0], zone: m[1], name: m[2], uniq: m[3]})
	}
	return instanceGroups, nil
}

func (g *gkeDeployer) getClusterFirewall() string {
	// We want to ensure that there's an e2e-ports-* firewall rule
	// that maps to the cluster nodes,
	return fmt.Sprintf("e2e-ports-%s-%s", g.cluster, g.locationRaw)
}

// This function ensures that all firewall-rules are deleted from specific network.
// We also want to keep in logs that there were some resources leaking.
func (g *gkeDeployer) cleanupNetworkFirewalls() (int, error) {
	fws, err := exec.Command("gcloud", "compute", "firewall-rules", "list",
		"--format=value(name)",
		"--project="+g.project,
		"--filter=network:"+g.network).Output()
	if err != nil {
		return 0, fmt.Errorf("firewall rules list failed: %s", util.ExecError(err))
	}
	if len(fws) > 0 {
		fwList := strings.Split(strings.TrimSpace(string(fws)), "\n")
		log.Printf("Network %s has %v undeleted firewall rules %v", g.network, len(fwList), fwList)
		commandArgs := []string{"compute", "firewall-rules", "delete", "-q"}
		commandArgs = append(commandArgs, fwList...)
		commandArgs = append(commandArgs, "--project="+g.project)
		errFirewall := control.FinishRunning(exec.Command("gcloud", commandArgs...))
		if errFirewall != nil {
			return 0, fmt.Errorf("error deleting firewall: %w", errFirewall)
		}
		return len(fwList), nil
	}
	return 0, nil
}

func (g *gkeDeployer) ensureNat() error {
	if !g.createNat {
		return nil
	}
	if g.network == "default" {
		return fmt.Errorf("NAT router should be set manually for the default network")
	}
	region, err := g.getRegion(g.region, g.zone)
	if err != nil {
		return fmt.Errorf("error finding region for NAT router: %w", err)
	}
	nat := g.getNatName()

	// Create this unique router only if it does not exist yet.
	if control.NoOutput(exec.Command("gcloud", "compute", "routers", "describe", nat,
		"--project="+g.project,
		"--region="+region,
		"--format=value(name)")) != nil {
		log.Printf("Couldn't describe router '%s', assuming it doesn't exist and creating it", nat)
		if err := control.FinishRunning(exec.Command("gcloud", "compute", "routers", "create", nat,
			"--project="+g.project,
			"--network="+g.network,
			"--region="+region)); err != nil {
			return fmt.Errorf("error creating NAT router: %w", err)
		}
	}
	// Create this unique NAT configuration only if it does not exist yet.
	if control.NoOutput(exec.Command("gcloud", "compute", "routers", "nats", "describe", nat,
		"--project="+g.project,
		"--router="+nat,
		"--router-region="+region,
		"--format=value(name)")) != nil {
		log.Printf("Couldn't describe NAT '%s', assuming it doesn't exist and creating it", nat)
		if err := control.FinishRunning(exec.Command("gcloud", "compute", "routers", "nats", "create", nat,
			"--project="+g.project,
			"--router="+nat,
			"--router-region="+region,
			"--auto-allocate-nat-external-ips",
			"--min-ports-per-vm="+strconv.Itoa(g.natMinPortsPerVm),
			"--nat-primary-subnet-ip-ranges")); err != nil {
			return fmt.Errorf("error adding NAT to a router: %w", err)
		}
	}

	return nil
}

func (g *gkeDeployer) getRegion(region, zone string) (string, error) {
	if region != "" {
		return region, nil
	}
	result, err := exec.Command("gcloud", "compute", "zones", "list",
		"--filter=name="+zone,
		"--format=value(region)",
		"--project="+g.project).Output()
	if err != nil {
		return "", fmt.Errorf("error resolving region of %s zone: %w", zone, err)
	}
	return strings.TrimSuffix(string(result), "\n"), nil
}

func (g *gkeDeployer) getNatName() string {
	return "nat-router-" + g.cluster
}

func (g *gkeDeployer) cleanupNat() error {
	if !g.createNat {
		return nil
	}
	region, err := g.getRegion(g.region, g.zone)
	if err != nil {
		return fmt.Errorf("error finding region for NAT router: %w", err)
	}
	nat := g.getNatName()

	// Delete NAT router. That will remove NAT configuration as well.
	if control.NoOutput(exec.Command("gcloud", "compute", "routers", "describe", nat,
		"--project="+g.project,
		"--region="+region,
		"--format=value(name)")) == nil {
		log.Printf("Found NAT router '%s', deleting", nat)
		err = control.FinishRunning(exec.Command("gcloud", "compute", "routers", "delete", "-q", nat,
			"--project="+g.project,
			"--region="+region))
		if err != nil {
			return fmt.Errorf("error deleting NAT router: %w", err)
		}
	} else {
		log.Printf("Found no NAT router '%s', assuming resources are clean", nat)
	}

	return nil
}

func (g *gkeDeployer) Down() error {
	g.instanceGroups = nil

	clusterExistsBytes, err := control.Output(exec.Command("gcloud", g.containerArgs("clusters",
		"list", "--project="+g.project, fmt.Sprintf("--filter=(name=%s AND location=%s)", g.cluster, g.locationRaw))...))
	if strings.TrimSpace(string(clusterExistsBytes)) == "" {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to list clusters in project %s with filter (name=%s AND location=%s): %w", g.project, g.cluster, g.locationRaw, err)
	}

	operationNameBytes, err := control.Output(exec.Command(
		"gcloud", g.containerArgs("operations", "list", "--project="+g.project,
			g.location, "--format=value(name)", fmt.Sprintf("--filter=(status!=DONE AND (targetLink ~ /clusters/%s$ OR targetLink ~ /clusters/%s/))", g.cluster, g.cluster))...))
	if err != nil {
		return fmt.Errorf("failed to list not DONE operations for cluster %s: %w", g.cluster, err)
	}

	operationName := strings.TrimSpace(string(operationNameBytes))
	if operationName != "" {
		log.Printf("Found RUNNING operation %q blocking cluster deletion. Will wait for its completion.", operationName)
		err := control.FinishRunning(exec.Command(
			"gcloud", g.containerArgs("operations", "wait", "--project="+g.project,
				g.location, operationName)...))
		if err != nil {
			return fmt.Errorf("error waiting for operation %s to finish: %w", operationName, err)
		}
	}

	// We best-effort try all of these and report errors as appropriate.
	errCluster := control.FinishRunning(exec.Command(
		"gcloud", g.containerArgs("clusters", "delete", "-q", g.cluster,
			"--project="+g.project,
			g.location, fmt.Sprintf("--timeout=%d", int(gkeDownTimeout.Seconds())))...))

	// don't delete default network
	if g.network == "default" {
		if errCluster != nil {
			log.Printf("Error deleting cluster using default network, allow the error for now %s", errCluster)
		}
		return nil
	}

	firewall := g.getClusterFirewall()
	var errFirewall error
	if control.NoOutput(exec.Command("gcloud", "compute", "firewall-rules", "describe", firewall,
		"--project="+g.project,
		"--format=value(name)")) == nil {
		log.Printf("Found rules for firewall '%s', deleting them", firewall)
		errFirewall = control.FinishRunning(exec.Command("gcloud", "compute", "firewall-rules", "delete", "-q", firewall,
			"--project="+g.project))
	} else {
		log.Printf("Found no rules for firewall '%s', assuming resources are clean", firewall)
	}
	numLeakedFWRules, errCleanFirewalls := g.cleanupNetworkFirewalls()

	errNat := g.cleanupNat()

	var errSubnet error
	if g.subnetwork != "" {
		errSubnet = control.FinishRunning(exec.Command("gcloud", "compute", "networks", "subnets", "delete", "-q", g.subnetwork,
			g.subnetworkRegion, "--project="+g.project))
	}
	var errNetwork error
	if *gkeRemoveNetwork {
		errNetwork = control.FinishRunning(exec.Command("gcloud", "compute", "networks", "delete", "-q", g.network,
			"--project="+g.project))
	}
	if errCluster != nil {
		return fmt.Errorf("error deleting cluster: %w", errCluster)
	}
	if errFirewall != nil {
		return fmt.Errorf("error deleting firewall: %w", errFirewall)
	}
	if errCleanFirewalls != nil {
		return fmt.Errorf("error cleaning-up firewalls: %w", errCleanFirewalls)
	}
	if errNat != nil {
		return fmt.Errorf("error cleaning-up NAT: %w", errNat)
	}
	if errSubnet != nil {
		return fmt.Errorf("error deleting subnetwork: %w", errSubnet)
	}
	if errNetwork != nil {
		return fmt.Errorf("error deleting network: %w", errNetwork)
	}
	if numLeakedFWRules > 0 {
		// Leaked firewall rules are cleaned up already, print a warning instead of failing hard
		log.Println("Warning: leaked firewall rules")
	}
	return nil
}

func (g *gkeDeployer) containerArgs(args ...string) []string {
	return append(append(append([]string{}, g.commandGroup...), "container"), args...)
}

func (g *gkeDeployer) GetClusterCreated(gcpProject string) (time.Time, error) {
	res, err := control.Output(exec.Command(
		"gcloud",
		"compute",
		"instance-groups",
		"list",
		"--project="+gcpProject,
		"--format=json(name,creationTimestamp)"))
	if err != nil {
		return time.Time{}, fmt.Errorf("list instance-group failed : %w", err)
	}

	created, err := getLatestClusterUpTime(string(res))
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time failed : got gcloud res %s, err %w", string(res), err)
	}
	return created, nil
}

func (g *gkeDeployer) KubectlCommand() (*exec.Cmd, error) { return nil, nil }

func wrapErrors(stage string, errs ...error) error {
	return fmt.Errorf("%s encountered %d errors: %v", stage, len(errs), errs)
}
