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

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"sigs.k8s.io/boskos/client"

	"k8s.io/test-infra/kubetest/conformance"
	"k8s.io/test-infra/kubetest/kind"
	"k8s.io/test-infra/kubetest/process"
	"k8s.io/test-infra/kubetest/util"
)

// Hardcoded in ginkgo-e2e.sh
const defaultGinkgoParallel = 25

var (
	artifacts = filepath.Join(os.Getenv("WORKSPACE"), "_artifacts")
	interrupt = time.NewTimer(time.Duration(0)) // interrupt testing at this time.
	terminate = time.NewTimer(time.Duration(0)) // terminate testing at this time.
	verbose   = false
	timeout   = time.Duration(0)
	boskos, _ = client.NewClient(os.Getenv("JOB_NAME"), "http://boskos.test-pods.svc.cluster.local.", "", "")
	control   = process.NewControl(timeout, interrupt, terminate, verbose)
)

type options struct {
	build                buildStrategy
	charts               bool
	checkLeaks           bool
	checkSkew            bool
	cluster              string
	clusterIPRange       string
	deployment           string
	down                 bool
	dump                 string
	dumpPreTestLogs      string
	extract              extractStrategies
	extractCIBucket      string
	extractReleaseBucket string
	extractSource        bool
	flushMemAfterBuild   bool
	focusRegex           string
	gcpCloudSdk          string
	gcpMasterImage       string
	gcpMasterSize        string
	gcpNetwork           string
	gcpNodeImage         string
	gcpImageFamily       string
	gcpImageProject      string
	gcpNodes             string
	gcpNodeSize          string
	gcpProject           string
	gcpProjectType       string
	gcpServiceAccount    string
	// gcpSSHProxyInstanceName is the name of the vm instance which ip address will be used to set the
	// KUBE_SSH_BASTION env. If set, it will result in proxying ssh connections in tests through the
	// "bastion". It's useful for clusters with nodes without public ssh access, e.g. nodes without
	// public ip addresses. Works only for gcp providers (gce, gke).
	gcpSSHProxyInstanceName string
	gcpRegion               string
	gcpZone                 string
	ginkgoParallel          ginkgoParallelValue
	kubecfg                 string
	kubemark                bool
	kubemarkMasterSize      string
	kubemarkNodes           string // TODO(fejta): switch to int after migration
	logexporterGCSPath      string
	metadataSources         string
	noAllowDup              bool
	nodeArgs                string
	nodeTestArgs            string
	nodeTests               bool
	outputDir               string
	provider                string
	publish                 string
	runtimeConfig           string
	save                    string
	skew                    bool
	skipRegex               string
	soak                    bool
	soakDuration            time.Duration
	sshUser                 string
	stage                   stageStrategy
	test                    bool
	testArgs                string
	testCmd                 string
	testCmdName             string
	testCmdArgs             []string
	up                      bool
	upgradeArgs             string
	boskosWaitDuration      time.Duration
}

func defineFlags() *options {
	o := options{}
	flag.Var(&o.build, "build", "Rebuild k8s binaries, optionally forcing (release|quick|bazel) strategy")
	flag.BoolVar(&o.charts, "charts", false, "If true, run charts tests")
	flag.BoolVar(&o.checkSkew, "check-version-skew", true, "Verify client and server versions match")
	flag.BoolVar(&o.checkLeaks, "check-leaked-resources", false, "Ensure project ends with the same resources")
	flag.StringVar(&o.cluster, "cluster", "", "Cluster name. Must be set for --deployment=gke (TODO: other deployments).")
	flag.StringVar(&o.clusterIPRange, "cluster-ip-range", "", "Specifies CLUSTER_IP_RANGE value during --up and --test (only relevant for --deployment=bash). Auto-calculated if empty.")
	flag.StringVar(&o.deployment, "deployment", "bash", "Choices: none/bash/conformance/gke/kind/kops/node/local")
	flag.BoolVar(&o.down, "down", false, "If true, tear down the cluster before exiting.")
	flag.StringVar(&o.dump, "dump", "", "If set, dump bring-up and cluster logs to this location on test or cluster-up failure")
	flag.StringVar(&o.dumpPreTestLogs, "dump-pre-test-logs", "", "If set, dump cluster logs to this location before running tests")
	flag.Var(&o.extract, "extract", "Extract k8s binaries from the specified release location")
	flag.StringVar(&o.extractCIBucket, "extract-ci-bucket", "kubernetes-release-dev", "Extract k8s CI binaries from the specified GCS bucket")
	flag.StringVar(&o.extractReleaseBucket, "extract-release-bucket", "kubernetes-release", "Extract k8s release binaries from the specified GCS bucket")
	flag.BoolVar(&o.extractSource, "extract-source", false, "Extract k8s src together with other tarballs")
	flag.BoolVar(&o.flushMemAfterBuild, "flush-mem-after-build", false, "If true, try to flush container memory after building")
	flag.Var(&o.ginkgoParallel, "ginkgo-parallel", fmt.Sprintf("Run Ginkgo tests in parallel, default %d runners. Use --ginkgo-parallel=N to specify an exact count.", defaultGinkgoParallel))
	flag.StringVar(&o.gcpCloudSdk, "gcp-cloud-sdk", "", "Install/upgrade google-cloud-sdk to the gs:// path if set")
	flag.StringVar(&o.gcpProject, "gcp-project", "", "For use with gcloud commands")
	flag.StringVar(&o.gcpProjectType, "gcp-project-type", "", "Explicitly indicate which project type to select from boskos")
	flag.StringVar(&o.gcpServiceAccount, "gcp-service-account", "", "Service account to activate before using gcloud")
	flag.StringVar(&o.gcpZone, "gcp-zone", "", "For use with gcloud commands")
	flag.StringVar(&o.gcpRegion, "gcp-region", "", "For use with gcloud commands")
	flag.StringVar(&o.gcpNetwork, "gcp-network", "", "Cluster network. Must be set for --deployment=gke (TODO: other deployments).")
	flag.StringVar(&o.gcpMasterImage, "gcp-master-image", "", "Master image type (cos|debian on GCE, n/a on GKE)")
	flag.StringVar(&o.gcpMasterSize, "gcp-master-size", "", "(--provider=gce only) Size of master to create (e.g n1-standard-1). Auto-calculated if left empty.")
	flag.StringVar(&o.gcpNodeImage, "gcp-node-image", "", "Node image type (cos|container_vm on GKE, cos|debian on GCE)")
	flag.StringVar(&o.gcpImageFamily, "image-family", "", "Node image family from which to use the latest image, required when --gcp-node-image=CUSTOM")
	flag.StringVar(&o.gcpImageProject, "image-project", "", "Project containing node image family, required when --gcp-node-image=CUSTOM")
	flag.StringVar(&o.gcpNodes, "gcp-nodes", "", "(--provider=gce only) Number of nodes to create.")
	flag.StringVar(&o.gcpNodeSize, "gcp-node-size", "", "(--provider=gce only) Size of nodes to create (e.g n1-standard-1).")
	flag.StringVar(&o.gcpSSHProxyInstanceName, "gcp-ssh-proxy-instance-name", "", "(--provider=gce|gke only) If set, will result in proxing the ssh connections via the provided instance name while running tests")
	flag.StringVar(&o.kubecfg, "kubeconfig", "", "The location of a kubeconfig file.")
	flag.StringVar(&o.focusRegex, "ginkgo-focus", "", "The ginkgo regex to focus. Currently only respected for (dind).")
	flag.StringVar(&o.skipRegex, "ginkgo-skip", "", "The ginkgo regex to skip. Currently only respected for (dind).")
	flag.BoolVar(&o.kubemark, "kubemark", false, "If true, run kubemark tests.")
	flag.StringVar(&o.kubemarkMasterSize, "kubemark-master-size", "", "Kubemark master size (only relevant if --kubemark=true). Auto-calculated based on '--kubemark-nodes' if left empty.")
	flag.StringVar(&o.kubemarkNodes, "kubemark-nodes", "5", "Number of kubemark nodes to start (only relevant if --kubemark=true).")
	flag.StringVar(&o.logexporterGCSPath, "logexporter-gcs-path", "", "Path to the GCS artifacts directory to dump logs from nodes. Logexporter gets enabled if this is non-empty")
	flag.StringVar(&o.metadataSources, "metadata-sources", "images.json", "Comma-separated list of files inside ./artifacts to merge into metadata.json")
	flag.StringVar(&o.nodeArgs, "node-args", "", "Args for node e2e tests.")
	flag.StringVar(&o.nodeTestArgs, "node-test-args", "", "Test args specifically for node e2e tests.")
	flag.BoolVar(&o.noAllowDup, "no-allow-dup", false, "if set --allow-dup will not be passed to push-build and --stage will error if the build already exists on the gcs path")
	flag.BoolVar(&o.nodeTests, "node-tests", false, "If true, run node-e2e tests.")
	flag.StringVar(&o.provider, "provider", "", "Kubernetes provider such as gce, gke, aws, etc")
	flag.StringVar(&o.publish, "publish", "", "Publish version to the specified gs:// path on success")
	flag.StringVar(&o.runtimeConfig, "runtime-config", "batch/v2alpha1=true", "If set, API versions can be turned on or off while bringing up the API server.")
	flag.StringVar(&o.stage.dockerRegistry, "registry", "", "Push images to the specified docker registry (e.g. gcr.io/a-test-project)")
	flag.StringVar(&o.save, "save", "", "Save credentials to gs:// path on --up if set (or load from there if not --up)")
	flag.BoolVar(&o.skew, "skew", false, "If true, run tests in another version at ../kubernetes/kubernetes_skew")
	flag.BoolVar(&o.soak, "soak", false, "If true, job runs in soak mode")
	flag.DurationVar(&o.soakDuration, "soak-duration", 7*24*time.Hour, "Maximum age of a soak cluster before it gets recycled")
	flag.Var(&o.stage, "stage", "Upload binaries to gs://bucket/devel/job-suffix if set")
	flag.StringVar(&o.stage.versionSuffix, "stage-suffix", "", "Append suffix to staged version when set")
	flag.BoolVar(&o.test, "test", false, "Run Ginkgo tests.")
	flag.StringVar(&o.testArgs, "test_args", "", "Space-separated list of arguments to pass to Ginkgo test runner.")
	flag.StringVar(&o.testCmd, "test-cmd", "", "command to run against the cluster instead of Ginkgo e2e tests")
	flag.StringVar(&o.testCmdName, "test-cmd-name", "", "name to log the test command as in xml results")
	flag.DurationVar(&timeout, "timeout", time.Duration(0), "Terminate testing after the timeout duration (s/m/h)")
	flag.BoolVar(&o.up, "up", false, "If true, start the e2e cluster. If cluster is already up, recreate it.")
	flag.StringVar(&o.upgradeArgs, "upgrade_args", "", "If set, run upgrade tests before other tests")
	flag.DurationVar(&o.boskosWaitDuration, "boskos-wait-duration", 5*time.Minute, "Defines how long it waits until quit getting Boskos resoure, default 5 minutes")

	// The "-v" flag was also used by glog, which is used by k8s.io/client-go. Duplicate flags cause panics.
	// 1. Even if we could convince glog to change, they have too many consumers to ever do so.
	// 2. The glog lib parses flags during init. It is impossible to dynamically rewrite the args before they're parsed by glog.
	// 3. The glog lib takes an int value, so "-v false" is an error.
	// 4. It's possible, but unlikely, we could convince k8s.io/client-go to use a logging shim, because a library shouldn't force a logging implementation. This would take a major version release for the lib.
	//
	// The most reasonable solution is to accept that we shouldn't have made a single-letter global, and rename all references to this variable.
	flag.BoolVar(&verbose, "verbose-commands", true, "If true, print all command output.")

	// go flag does not support StringArrayVar
	pflag.StringArrayVar(&o.testCmdArgs, "test-cmd-args", []string{}, "args for test-cmd")
	return &o
}

var suite util.TestSuite = util.TestSuite{Name: "kubetest"}

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
		return fmt.Errorf("must run from kubernetes directory root. current: %v", acwd)
	}
	return nil
}

type deployer interface {
	Up() error
	IsUp() error
	DumpClusterLogs(localPath, gcsPath string) error
	TestSetup() error
	Down() error
	GetClusterCreated(gcpProject string) (time.Time, error)
	KubectlCommand() (*exec.Cmd, error)
}

// publisher is implemented by deployers that want to publish status on success
type publisher interface {
	// Publish is called when the tests were successful; the deployer should publish a success file
	Publish() error
}

func getDeployer(o *options) (deployer, error) {
	switch o.deployment {
	case "bash":
		return newBash(&o.clusterIPRange, o.gcpProject, o.gcpZone, o.gcpSSHProxyInstanceName, o.provider), nil
	case "conformance":
		return conformance.NewDeployer(o.kubecfg)
	case "gke":
		return newGKE(o.provider, o.gcpProject, o.gcpZone, o.gcpRegion, o.gcpNetwork, o.gcpNodeImage, o.gcpImageFamily, o.gcpImageProject, o.cluster, o.gcpSSHProxyInstanceName, &o.testArgs, &o.upgradeArgs)
	case "kind":
		return kind.NewDeployer(control, string(o.build))
	case "kops":
		return newKops(o.provider, o.gcpProject, o.cluster)
	case "node":
		return nodeDeploy{provider: o.provider}, nil
	case "none":
		return noneDeploy{}, nil
	case "local":
		return newLocalCluster(), nil
	case "aksengine":
		return newAKSEngine()
	case "aks":
		return newAksDeployer()
	default:
		return nil, fmt.Errorf("unknown deployment strategy %q", o.deployment)
	}
}

func validateFlags(o *options) error {
	if !o.extract.Enabled() && o.extractSource {
		return errors.New("--extract-source flag cannot be passed without --extract")
	}
	return nil
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Initialize global pseudo random generator. Initializing it to select random AWS Zones.
	rand.Seed(time.Now().UnixNano())

	pflag.CommandLine = pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)
	o := defineFlags()
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	if err := pflag.CommandLine.Parse(os.Args[1:]); err != nil {
		log.Fatalf("Flag parse failed: %v", err)
	}

	if err := validateFlags(o); err != nil {
		log.Fatalf("Flags validation failed. err: %v", err)
	}

	control = process.NewControl(timeout, interrupt, terminate, verbose)

	// do things when we know we are running in the kubetest image
	if os.Getenv("KUBETEST_IN_DOCKER") == "true" {
		o.flushMemAfterBuild = true
	}
	// sanity fix for kind deployer, not set for other deployers to avoid
	// breaking changes...
	if o.deployment == "kind" {
		// always default --dump for kind, in CI use $ARTIFACTS
		artifacts := os.Getenv("ARTIFACTS")
		if artifacts == "" {
			artifacts = "./_artifacts"
		}
		o.dump = artifacts
	}

	err := complete(o)

	if boskos.HasResource() {
		if berr := boskos.ReleaseAll("dirty"); berr != nil {
			log.Fatalf("[Boskos] Fail To Release: %v, kubetest err: %v", berr, err)
		}
	}

	if err != nil {
		log.Fatalf("Something went wrong: %v", err)
	}
}

func complete(o *options) error {
	if !terminate.Stop() {
		<-terminate.C // Drain the value if necessary.
	}
	if !interrupt.Stop() {
		<-interrupt.C // Drain value
	}

	if timeout > 0 {
		log.Printf("Limiting testing to %s", timeout)
		interrupt.Reset(timeout)
	}

	if o.dump != "" {
		defer writeMetadata(o.dump, o.metadataSources)
		defer control.WriteXML(&suite, o.dump, time.Now())
	}
	if o.logexporterGCSPath != "" {
		o.testArgs += fmt.Sprintf(" --logexporter-gcs-path=%s", o.logexporterGCSPath)
	}
	if err := prepare(o); err != nil {
		return fmt.Errorf("failed to prepare test environment: %v", err)
	}
	// Get the deployer before we acquire k8s so any additional flag
	// verifications happen early.
	deploy, err := getDeployer(o)
	if err != nil {
		return fmt.Errorf("error creating deployer: %v", err)
	}

	// Check soaking before run tests
	if o.soak {
		if created, err := deploy.GetClusterCreated(o.gcpProject); err != nil {
			// continue, but log the error
			log.Printf("deploy %v, GetClusterCreated failed: %v", o.deployment, err)
		} else {
			if time.Now().After(created.Add(o.soakDuration)) {
				// flip up on - which will tear down previous cluster and start a new one
				log.Printf("Previous soak cluster created at %v, will recreate the cluster", created)
				o.up = true
			}
		}
	}

	if err := acquireKubernetes(o, deploy); err != nil {
		return fmt.Errorf("failed to acquire k8s binaries: %v", err)
	}
	if o.extract.Enabled() {
		// If we specified `--extract-source` we will already be in the correct directory
		if !o.extractSource {
			if err := os.Chdir("kubernetes"); err != nil {
				return fmt.Errorf("failed to chdir to kubernetes dir: %v", err)
			}
		}
	}
	if err := validWorkingDirectory(); err != nil {
		return fmt.Errorf("called from invalid working directory: %v", err)
	}

	if o.down {
		// listen for signals such as ^C and gracefully attempt to clean up
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		go func() {
			for range c {
				log.Print("Captured ^C, gracefully attempting to cleanup resources..")
				if err = deploy.Down(); err != nil {
					log.Printf("Tearing down deployment failed: %v", err)
				}
				if err != nil {
					os.Exit(1)
				}

				os.Exit(2)
			}
		}()
	}

	if err := run(deploy, *o); err != nil {
		return err
	}

	// Publish the successfully tested version when requested
	if o.publish != "" {
		if err := publish(o.publish); err != nil {
			return err
		}
	}
	return nil
}

func acquireKubernetes(o *options, d deployer) error {
	// Potentially build kubernetes
	if o.build.Enabled() {
		var err error
		// kind deployer manages build
		if k, ok := d.(*kind.Deployer); ok {
			err = control.XMLWrap(&suite, "Build", k.Build)
		} else if c, ok := d.(*aksEngineDeployer); ok { // Azure deployer
			err = control.XMLWrap(&suite, "Build", func() error {
				return c.Build(o.build)
			})
		} else {
			err = control.XMLWrap(&suite, "Build", o.build.Build)
		}
		if o.flushMemAfterBuild {
			util.FlushMem()
		}
		if err != nil {
			return err
		}
	}

	// Potentially stage build binaries somewhere on GCS
	if o.stage.Enabled() {
		if err := control.XMLWrap(&suite, "Stage", func() error {
			return o.stage.Stage(o.noAllowDup)
		}); err != nil {
			return err
		}
	}

	// Potentially download existing binaries and extract them.
	if o.extract.Enabled() {
		err := control.XMLWrap(&suite, "Extract", func() error {
			// Should we restore a previous state?
			// Restore if we are not upping the cluster
			if o.save != "" {
				if !o.up {
					// Restore version and .kube/config from --up
					log.Printf("Overwriting extract strategy to load kubeconfig and version from %s", o.save)
					o.extract = extractStrategies{
						extractStrategy{
							mode:   load,
							option: o.save,
						},
					}
				}
			}

			// New deployment, extract new version
			return o.extract.Extract(o.gcpProject, o.gcpZone, o.gcpRegion, o.extractCIBucket, o.extractReleaseBucket, o.extractSource)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// Returns the k8s version name
func findVersion() string {
	// The version may be in a version file
	if _, err := os.Stat("version"); err == nil {
		b, err := ioutil.ReadFile("version")
		if err == nil {
			return strings.TrimSpace(string(b))
		}
		log.Printf("Failed to read version: %v", err)
	}

	// We can also get it from the git repo.
	if _, err := os.Stat("hack/lib/version.sh"); err == nil {
		// TODO(fejta): do this in go. At least we removed the upload-to-gcs.sh dep.
		gross := `. hack/lib/version.sh && KUBE_ROOT=. kube::version::get_version_vars && echo "${KUBE_GIT_VERSION-}"`
		b, err := control.Output(exec.Command("bash", "-c", gross))
		if err == nil {
			return strings.TrimSpace(string(b))
		}
		log.Printf("Failed to get_version_vars: %v", err)
	}

	return "unknown" // Sad trombone
}

// maybeMergeMetadata will add new keyvals into the map; quietly eats errors.
func maybeMergeJSON(meta map[string]string, path string) {
	if data, err := ioutil.ReadFile(path); err == nil {
		json.Unmarshal(data, &meta)
	}
}

// Write metadata.json, including version and env arg data.
func writeMetadata(path, metadataSources string) error {
	m := make(map[string]string)

	// Look for any sources of metadata and load 'em
	for _, f := range strings.Split(metadataSources, ",") {
		maybeMergeJSON(m, filepath.Join(path, f))
	}

	ver := findVersion()
	m["job-version"] = ver // TODO(krzyzacy): retire
	m["revision"] = ver
	re := regexp.MustCompile(`^BUILD_METADATA_(.+)$`)
	for _, e := range os.Environ() {
		p := strings.SplitN(e, "=", 2)
		r := re.FindStringSubmatch(p[0])
		if r == nil {
			continue
		}
		k, v := strings.ToLower(r[1]), p[1]
		m[k] = v
	}
	f, err := os.Create(filepath.Join(path, "metadata.json"))
	if err != nil {
		return err
	}
	defer f.Close()
	e := json.NewEncoder(f)
	return e.Encode(m)
}

// Install cloudsdk tarball to location, updating PATH
func installGcloud(tarball string, location string) error {

	if err := os.MkdirAll(location, 0775); err != nil {
		return err
	}

	if err := control.FinishRunning(exec.Command("tar", "xzf", tarball, "-C", location)); err != nil {
		return err
	}

	if err := control.FinishRunning(exec.Command(filepath.Join(location, "google-cloud-sdk", "install.sh"), "--disable-installation-options", "--bash-completion=false", "--path-update=false", "--usage-reporting=false")); err != nil {
		return err
	}

	if err := util.InsertPath(filepath.Join(location, "google-cloud-sdk", "bin")); err != nil {
		return err
	}

	if err := control.FinishRunning(exec.Command("gcloud", "components", "install", "alpha")); err != nil {
		return err
	}

	if err := control.FinishRunning(exec.Command("gcloud", "components", "install", "beta")); err != nil {
		return err
	}

	if err := control.FinishRunning(exec.Command("gcloud", "info")); err != nil {
		return err
	}
	return nil
}

func migrateGcpEnvAndOptions(o *options) error {
	var network string
	var zone string
	switch o.provider {
	case "gke":
		network = "KUBE_GKE_NETWORK"
		zone = "ZONE"
	default:
		network = "KUBE_GCE_NETWORK"
		zone = "KUBE_GCE_ZONE"
	}
	return util.MigrateOptions([]util.MigratedOption{
		{
			Env:    "PROJECT",
			Option: &o.gcpProject,
			Name:   "--gcp-project",
		},
		{
			Env:    zone,
			Option: &o.gcpZone,
			Name:   "--gcp-zone",
		},
		{
			Env:    "REGION",
			Option: &o.gcpRegion,
			Name:   "--gcp-region",
		},
		{
			Env:    "GOOGLE_APPLICATION_CREDENTIALS",
			Option: &o.gcpServiceAccount,
			Name:   "--gcp-service-account",
		},
		{
			Env:    network,
			Option: &o.gcpNetwork,
			Name:   "--gcp-network",
		},
		{
			Env:    "KUBE_NODE_OS_DISTRIBUTION",
			Option: &o.gcpNodeImage,
			Name:   "--gcp-node-image",
		},
		{
			Env:    "KUBE_MASTER_OS_DISTRIBUTION",
			Option: &o.gcpMasterImage,
			Name:   "--gcp-master-image",
		},
		{
			Env:    "NUM_NODES",
			Option: &o.gcpNodes,
			Name:   "--gcp-nodes",
		},
		{
			Env:    "NODE_SIZE",
			Option: &o.gcpNodeSize,
			Name:   "--gcp-node-size",
		},
		{
			Env:    "MASTER_SIZE",
			Option: &o.gcpMasterSize,
			Name:   "--gcp-master-size",
		},
		{
			Env:      "CLOUDSDK_BUCKET",
			Option:   &o.gcpCloudSdk,
			Name:     "--gcp-cloud-sdk",
			SkipPush: true,
		},
	})
}

func prepareGcp(o *options) error {
	if err := migrateGcpEnvAndOptions(o); err != nil {
		return err
	}
	// Must happen before any gcloud commands
	if err := activateServiceAccount(o.gcpServiceAccount); err != nil {
		return err
	}

	if o.provider == "gce" {
		if distro := os.Getenv("KUBE_OS_DISTRIBUTION"); distro != "" {
			log.Printf("Please use --gcp-master-image=%s --gcp-node-image=%s (instead of deprecated KUBE_OS_DISTRIBUTION)",
				distro, distro)
			// Note: KUBE_OS_DISTRIBUTION takes precedence over
			// KUBE_{MASTER,NODE}_OS_DISTRIBUTION, so override here
			// after the migration above.
			o.gcpNodeImage = distro
			o.gcpMasterImage = distro
			if err := os.Setenv("KUBE_NODE_OS_DISTRIBUTION", distro); err != nil {
				return fmt.Errorf("could not set KUBE_NODE_OS_DISTRIBUTION=%s: %v", distro, err)
			}
			if err := os.Setenv("KUBE_MASTER_OS_DISTRIBUTION", distro); err != nil {
				return fmt.Errorf("could not set KUBE_MASTER_OS_DISTRIBUTION=%s: %v", distro, err)
			}
		}

		hasGCPImageFamily, hasGCPImageProject := len(o.gcpImageFamily) != 0, len(o.gcpImageProject) != 0
		if hasGCPImageFamily != hasGCPImageProject {
			return fmt.Errorf("--image-family and --image-project must be both set or unset")
		}
		if hasGCPImageFamily && hasGCPImageProject {
			out, err := control.Output(exec.Command("gcloud", "compute", "images", "describe-from-family", o.gcpImageFamily, "--project", o.gcpImageProject))
			if err != nil {
				return fmt.Errorf("failed to get latest image from family %q in project %q: %s", o.gcpImageFamily, o.gcpImageProject, err)
			}
			latestImage := ""
			latestImageRegexp := regexp.MustCompile("^name: *(\\S+)")
			for _, line := range strings.Split(string(out), "\n") {
				matches := latestImageRegexp.FindStringSubmatch(line)
				if len(matches) == 2 {
					latestImage = matches[1]
					break
				}
			}
			if len(latestImage) == 0 {
				return fmt.Errorf("failed to get latest image from family %q in project %q", o.gcpImageFamily, o.gcpImageProject)
			}
			if o.deployment == "node" {
				o.nodeArgs += fmt.Sprintf(" --images=%s --image-project=%s", latestImage, o.gcpImageProject)
			} else {
				os.Setenv("KUBE_GCE_NODE_IMAGE", latestImage)
				os.Setenv("KUBE_GCE_NODE_PROJECT", o.gcpImageProject)
			}
		}
	} else if o.provider == "gke" {
		if o.deployment == "" {
			o.deployment = "gke"
		}
		if o.deployment != "gke" {
			return fmt.Errorf("expected --deployment=gke for --provider=gke, found --deployment=%s", o.deployment)
		}
		if o.gcpMasterImage != "" {
			return fmt.Errorf("expected --gcp-master-image to be empty for --provider=gke, found --gcp-master-image=%s", o.gcpMasterImage)
		}
		if o.gcpNodes != "" {
			return fmt.Errorf("--gcp-nodes cannot be set on GKE, use --gke-shape instead")
		}
		if o.gcpNodeSize != "" {
			return fmt.Errorf("--gcp-node-size cannot be set on GKE, use --gke-shape instead")
		}
		if o.gcpMasterSize != "" {
			return fmt.Errorf("--gcp-master-size cannot be set on GKE, where it's auto-computed")
		}

		// TODO(kubernetes/test-infra#3536): This is used by the
		// ginkgo-e2e.sh wrapper.
		nod := o.gcpNodeImage
		if nod == "container_vm" {
			// gcloud container clusters create understands
			// "container_vm", e2es understand "debian".
			nod = "debian"
		}
		if nod == "cos_containerd" {
			// gcloud container clusters create understands
			// "cos_containerd", e2es only understand
			// "gci"/"cos",
			nod = "gci"
		}
		os.Setenv("NODE_OS_DISTRIBUTION", nod)
	}
	if o.gcpProject == "" {
		log.Print("--gcp-project is missing, trying to fetch a project from boskos.\n" +
			"(for local runs please set --gcp-project to your dev project)")

		var resType string
		if o.gcpProjectType != "" {
			resType = o.gcpProjectType
		} else if o.provider == "gke" {
			resType = "gke-project"
		} else {
			resType = "gce-project"
		}

		log.Printf("provider %v, will acquire project type %v from boskos", o.provider, resType)

		// let's retry 5min to get next available resource
		ctx, cancel := context.WithTimeout(context.Background(), o.boskosWaitDuration)
		defer cancel()
		p, err := boskos.AcquireWait(ctx, resType, "free", "busy")
		if err != nil {
			return fmt.Errorf("--provider=%s boskos failed to acquire project: %v", o.provider, err)
		}

		if p == nil {
			return fmt.Errorf("boskos does not have a free %s at the moment", resType)
		}

		go func(c *client.Client, proj string) {
			for range time.Tick(time.Minute * 5) {
				if err := c.UpdateOne(p.Name, "busy", nil); err != nil {
					log.Printf("[Boskos] Update of %s failed with %v", p.Name, err)
				}
			}
		}(boskos, p.Name)
		o.gcpProject = p.Name
	}

	if err := os.Setenv("CLOUDSDK_CORE_PRINT_UNHANDLED_TRACEBACKS", "1"); err != nil {
		return fmt.Errorf("could not set CLOUDSDK_CORE_PRINT_UNHANDLED_TRACEBACKS=1: %v", err)
	}

	if err := control.FinishRunning(exec.Command("gcloud", "config", "set", "project", o.gcpProject)); err != nil {
		return fmt.Errorf("fail to set project %s : err %v", o.gcpProject, err)
	}

	// TODO(krzyzacy):Remove this when we retire migrateGcpEnvAndOptions
	// Note that a lot of scripts are still depend on this env in k/k repo.
	if err := os.Setenv("PROJECT", o.gcpProject); err != nil {
		return fmt.Errorf("fail to set env var PROJECT %s : err %v", o.gcpProject, err)
	}

	// Ensure ssh keys exist
	log.Print("Checking existing of GCP ssh keys...")
	k := filepath.Join(util.Home(".ssh"), "google_compute_engine")
	if _, err := os.Stat(k); err != nil {
		return err
	}
	pk := k + ".pub"
	if _, err := os.Stat(pk); err != nil {
		return err
	}

	log.Printf("Checking presence of public key in %s", o.gcpProject)
	if out, err := control.Output(exec.Command("gcloud", "compute", "--project="+o.gcpProject, "project-info", "describe")); err != nil {
		return err
	} else if b, err := ioutil.ReadFile(pk); err != nil {
		return err
	} else if !strings.Contains(string(out), string(b)) {
		log.Print("Uploading public ssh key to project metadata...")
		if err = control.FinishRunning(exec.Command("gcloud", "compute", "--project="+o.gcpProject, "config-ssh")); err != nil {
			return err
		}
	}

	// Install custom gcloud version if necessary
	if o.gcpCloudSdk != "" {
		for i := 0; i < 3; i++ {
			if err := control.FinishRunning(exec.Command("gsutil", "-mq", "cp", "-r", o.gcpCloudSdk, util.Home())); err == nil {
				break // Success!
			}
			time.Sleep(1 << uint(i) * time.Second)
		}
		for _, f := range []string{util.Home(".gsutil"), util.Home("repo"), util.Home("cloudsdk")} {
			if _, err := os.Stat(f); err == nil || !os.IsNotExist(err) {
				if err = os.RemoveAll(f); err != nil {
					return err
				}
			}
		}

		install := util.Home("repo", "google-cloud-sdk.tar.gz")
		if strings.HasSuffix(o.gcpCloudSdk, ".tar.gz") {
			install = util.Home(filepath.Base(o.gcpCloudSdk))
		} else {
			if err := os.Rename(util.Home(filepath.Base(o.gcpCloudSdk)), util.Home("repo")); err != nil {
				return err
			}

			// Controls which gcloud components to install.
			pop, err := util.PushEnv("CLOUDSDK_COMPONENT_MANAGER_SNAPSHOT_URL", "file://"+util.Home("repo", "components-2.json"))
			if err != nil {
				return err
			}
			defer pop()
		}

		if err := installGcloud(install, util.Home("cloudsdk")); err != nil {
			return err
		}
		// gcloud creds may have changed
		if err := activateServiceAccount(o.gcpServiceAccount); err != nil {
			return err
		}
	}

	if o.kubemark {
		if p := os.Getenv("KUBEMARK_BAZEL_BUILD"); strings.ToLower(p) == "y" {
			// we need docker-credential-gcr to get authed properly
			// https://github.com/bazelbuild/rules_docker#authorization
			if err := control.FinishRunning(exec.Command("gcloud", "components", "install", "docker-credential-gcr")); err != nil {
				return err
			}
			if err := control.FinishRunning(exec.Command("docker-credential-gcr", "configure-docker")); err != nil {
				return err
			}
		}
	}

	return nil
}

func prepareAws(o *options) error {
	// gcloud creds may have changed
	if err := activateServiceAccount(o.gcpServiceAccount); err != nil {
		return err
	}
	return control.FinishRunning(exec.Command("pip", "install", "awscli"))
}

// Activate GOOGLE_APPLICATION_CREDENTIALS if set or do nothing.
func activateServiceAccount(path string) error {
	if path == "" {
		return nil
	}
	return control.FinishRunning(exec.Command("gcloud", "auth", "activate-service-account", "--key-file="+path))
}

// Make all artifacts world readable.
// The root user winds up owning the files when the container exists.
// Ensure that other users can read these files at that time.
func chmodArtifacts() error {
	return control.FinishRunning(exec.Command("chmod", "-R", "o+r", artifacts))
}

func prepare(o *options) error {
	if err := util.MigrateOptions([]util.MigratedOption{
		{
			Env:    "KUBERNETES_PROVIDER",
			Option: &o.provider,
			Name:   "--provider",
		},
		{
			Env:    "CLUSTER_NAME",
			Option: &o.cluster,
			Name:   "--cluster",
		},
	}); err != nil {
		return err
	}
	if err := prepareGinkgoParallel(&o.ginkgoParallel); err != nil {
		return err
	}

	switch o.provider {
	case "gce", "gke", "node":
		if err := prepareGcp(o); err != nil {
			return err
		}
	case "aws":
		if err := prepareAws(o); err != nil {
			return err
		}
	}

	if o.kubemark {
		if err := util.MigrateOptions([]util.MigratedOption{
			{
				Env:    "KUBEMARK_NUM_NODES",
				Option: &o.kubemarkNodes,
				Name:   "--kubemark-nodes",
			},
			{
				Env:    "KUBEMARK_MASTER_SIZE",
				Option: &o.kubemarkMasterSize,
				Name:   "--kubemark-master-size",
			},
		}); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(artifacts, 0777); err != nil { // Create artifacts
		return err
	}

	return nil
}

type ginkgoParallelValue struct {
	v int // 0 == not set (defaults to 1)
}

func (v *ginkgoParallelValue) IsBoolFlag() bool {
	return true
}

func (v *ginkgoParallelValue) String() string {
	if v.v == 0 {
		return "1"
	}
	return strconv.Itoa(v.v)
}

func (v *ginkgoParallelValue) Set(s string) error {
	if s == "" {
		v.v = 0
		return nil
	}
	if s == "true" {
		v.v = defaultGinkgoParallel
		return nil
	}
	p, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("--ginkgo-parallel must be an integer, found %q", s)
	}
	if p < 1 {
		return fmt.Errorf("--ginkgo-parallel must be >= 1, found %d", p)
	}
	v.v = p
	return nil
}

func (v *ginkgoParallelValue) Type() string {
	return "ginkgoParallelValue"
}

func (v *ginkgoParallelValue) Get() int {
	if v.v == 0 {
		return 1
	}
	return v.v
}

var _ flag.Value = &ginkgoParallelValue{}

// Hand migrate this option. GINKGO_PARALLEL => GINKGO_PARALLEL_NODES=25
func prepareGinkgoParallel(v *ginkgoParallelValue) error {
	if p := os.Getenv("GINKGO_PARALLEL"); strings.ToLower(p) == "y" {
		log.Printf("Please use kubetest --ginkgo-parallel (instead of deprecated GINKGO_PARALLEL=y)")
		if err := v.Set("true"); err != nil {
			return err
		}
		os.Unsetenv("GINKGO_PARALLEL")
	}
	if p := os.Getenv("GINKGO_PARALLEL_NODES"); p != "" {
		log.Printf("Please use kubetest --ginkgo-parallel=%s (instead of deprecated GINKGO_PARALLEL_NODES=%s)", p, p)
		if err := v.Set(p); err != nil {
			return err
		}
	}
	os.Setenv("GINKGO_PARALLEL_NODES", v.String())
	return nil
}

func publish(pub string) error {
	v, err := ioutil.ReadFile("version")
	if err != nil {
		return err
	}
	log.Printf("Set %s version to %s", pub, string(v))
	return gcsWrite(pub, v)
}
