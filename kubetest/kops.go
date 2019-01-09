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
	"bytes"
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
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"golang.org/x/crypto/ssh"
	"k8s.io/test-infra/kubetest/e2e"
	"k8s.io/test-infra/kubetest/util"
)

// kopsAWSMasterSize is the default ec2 instance type for kops on aws
const kopsAWSMasterSize = "c4.large"

var (

	// kops specific flags.
	kopsPath         = flag.String("kops", "", "(kops only) Path to the kops binary. kops will be downloaded from kops-base-url if not set.")
	kopsCluster      = flag.String("kops-cluster", "", "(kops only) Deprecated. Cluster name for kops; if not set defaults to --cluster.")
	kopsState        = flag.String("kops-state", "", "(kops only) s3:// path to kops state store. Must be set.")
	kopsSSHUser      = flag.String("kops-ssh-user", os.Getenv("USER"), "(kops only) Username for SSH connections to nodes.")
	kopsSSHKey       = flag.String("kops-ssh-key", "", "(kops only) Path to ssh key-pair for each node (defaults '~/.ssh/kube_aws_rsa' if unset.)")
	kopsKubeVersion  = flag.String("kops-kubernetes-version", "", "(kops only) If set, the version of Kubernetes to deploy (can be a URL to a GCS path where the release is stored) (Defaults to kops default, latest stable release.).")
	kopsZones        = flag.String("kops-zones", "", "(kops only) zones for kops deployment, comma delimited.")
	kopsNodes        = flag.Int("kops-nodes", 2, "(kops only) Number of nodes to create.")
	kopsUpTimeout    = flag.Duration("kops-up-timeout", 20*time.Minute, "(kops only) Time limit between 'kops config / kops update' and a response from the Kubernetes API.")
	kopsAdminAccess  = flag.String("kops-admin-access", "", "(kops only) If set, restrict apiserver access to this CIDR range.")
	kopsImage        = flag.String("kops-image", "", "(kops only) Image (AMI) for nodes to use. (Defaults to kops default, a Debian image with a custom kubernetes kernel.)")
	kopsArgs         = flag.String("kops-args", "", "(kops only) Additional space-separated args to pass unvalidated to 'kops create cluster', e.g. '--kops-args=\"--dns private --node-size t2.micro\"'")
	kopsPriorityPath = flag.String("kops-priority-path", "", "(kops only) Insert into PATH if set")
	kopsBaseURL      = flag.String("kops-base-url", "", "(kops only) Base URL for a prebuilt version of kops")
	kopsVersion      = flag.String("kops-version", "", "(kops only) URL to a file containing a valid kops-base-url")
	kopsDiskSize     = flag.Int("kops-disk-size", 48, "(kops only) Disk size to use for nodes and masters")
	kopsPublish      = flag.String("kops-publish", "", "(kops only) Publish kops version to the specified gs:// path on success")
	kopsMasterSize   = flag.String("kops-master-size", kopsAWSMasterSize, "(kops only) master instance type")
	kopsMasterCount  = flag.Int("kops-master-count", 1, "(kops only) Number of masters to run")
	kopsEtcdVersion  = flag.String("kops-etcd-version", "", "(kops only) Etcd Version")

	kopsMultipleZones = flag.Bool("kops-multiple-zones", false, "(kops only) run tests in multiple zones")

	awsRegions = []string{
		"ap-south-1",
		"eu-west-2",
		"eu-west-1",
		"ap-northeast-2",
		"ap-northeast-1",
		"sa-east-1",
		"ca-central-1",
		// not supporting Singapore since they do not seem to have capacity for c4.large
		//"ap-southeast-1",
		"ap-southeast-2",
		"eu-central-1",
		"us-east-1",
		"us-east-2",
		"us-west-1",
		"us-west-2",
		// not supporting Paris yet as AWS does not have all instance types available
		//"eu-west-3",
	}
)

type kops struct {
	path        string
	kubeVersion string
	zones       []string
	nodes       int
	adminAccess string
	cluster     string
	image       string
	args        string
	kubecfg     string
	diskSize    int

	// sshUser is the username to use when SSHing to nodes (for example for log capture)
	sshUser string
	// sshPublicKey is the path to the SSH public key matching sshPrivateKey
	sshPublicKey string
	// sshPrivateKey is the path to the SSH private key matching sshPublicKey
	sshPrivateKey string

	// GCP project we should use
	gcpProject string

	// Cloud provider in use (gce, aws)
	provider string

	// kopsVersion is the version of kops we are running (used for publishing)
	kopsVersion string

	// kopsPublish is the path where we will publish kopsVersion, after a successful test
	kopsPublish string

	// masterCount denotes how many masters to start
	masterCount int

	// etcdVersion is the etcd version to run
	etcdVersion string

	// masterSize is the EC2 instance type for the master
	masterSize string

	// multipleZones denotes using more than one zone
	multipleZones bool
}

var _ deployer = kops{}

func migrateKopsEnv() error {
	return util.MigrateOptions([]util.MigratedOption{
		{
			Env:      "KOPS_STATE_STORE",
			Option:   kopsState,
			Name:     "--kops-state",
			SkipPush: true,
		},
		{
			Env:      "AWS_SSH_KEY",
			Option:   kopsSSHKey,
			Name:     "--kops-ssh-key",
			SkipPush: true,
		},
		{
			Env:      "PRIORITY_PATH",
			Option:   kopsPriorityPath,
			Name:     "--kops-priority-path",
			SkipPush: true,
		},
	})
}

func newKops(provider, gcpProject, cluster string) (*kops, error) {
	tmpdir, err := ioutil.TempDir("", "kops")
	if err != nil {
		return nil, err
	}

	if err := migrateKopsEnv(); err != nil {
		return nil, err
	}

	if *kopsCluster != "" {
		cluster = *kopsCluster
	}
	if cluster == "" {
		return nil, fmt.Errorf("--cluster or --kops-cluster must be set to a valid cluster name for kops deployment")
	}
	if *kopsState == "" {
		return nil, fmt.Errorf("--kops-state must be set to a valid S3 path for kops deployment")
	}
	if *kopsPriorityPath != "" {
		if err := util.InsertPath(*kopsPriorityPath); err != nil {
			return nil, err
		}
	}

	// TODO(fejta): consider explicitly passing these env items where needed.
	sshKey := *kopsSSHKey
	if sshKey == "" {
		usr, err := user.Current()
		if err != nil {
			return nil, err
		}
		sshKey = filepath.Join(usr.HomeDir, ".ssh/kube_aws_rsa")
	}
	if err := os.Setenv("KOPS_STATE_STORE", *kopsState); err != nil {
		return nil, err
	}

	sshUser := *kopsSSHUser
	if sshUser != "" {
		if err := os.Setenv("KUBE_SSH_USER", sshUser); err != nil {
			return nil, err
		}
	}

	// Repoint KUBECONFIG to an isolated kubeconfig in our temp directory
	kubecfg := filepath.Join(tmpdir, "kubeconfig")
	f, err := os.Create(kubecfg)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := f.Chmod(0600); err != nil {
		return nil, err
	}
	if err := os.Setenv("KUBECONFIG", kubecfg); err != nil {
		return nil, err
	}

	// Set KUBERNETES_CONFORMANCE_TEST so the auth info is picked up
	// from kubectl instead of bash inference.
	if err := os.Setenv("KUBERNETES_CONFORMANCE_TEST", "yes"); err != nil {
		return nil, err
	}
	// Set KUBERNETES_CONFORMANCE_PROVIDER to override the
	// cloudprovider for KUBERNETES_CONFORMANCE_TEST.
	// This value is set by the provider flag that is passed into kubetest.
	// HACK: until we merge #7408, there's a bug in the ginkgo-e2e.sh script we have to work around
	// TODO(justinsb): remove this hack once #7408 merges
	// if err := os.Setenv("KUBERNETES_CONFORMANCE_PROVIDER", provider); err != nil {
	if err := os.Setenv("KUBERNETES_CONFORMANCE_PROVIDER", "aws"); err != nil {
		return nil, err
	}
	// AWS_SSH_KEY is required by the AWS e2e tests.
	if err := os.Setenv("AWS_SSH_KEY", sshKey); err != nil {
		return nil, err
	}

	// zones are required by the kops e2e tests.
	var zones []string

	// if zones is set to zero and gcp project is not set then pick random aws zone
	if *kopsZones == "" && provider == "aws" {
		zones, err = getRandomAWSZones(*kopsMasterCount, *kopsMultipleZones)
		if err != nil {
			return nil, err
		}
	} else {
		zones = strings.Split(*kopsZones, ",")
	}

	// set ZONES for e2e.go
	if err := os.Setenv("ZONE", zones[0]); err != nil {
		return nil, err
	}

	if len(zones) == 0 {
		return nil, errors.New("no zones found")
	} else if zones[0] == "" {
		return nil, errors.New("zone cannot be a empty string")
	}

	log.Printf("executing kops with zones: %q", zones)

	// Set kops-base-url from kops-version
	if *kopsVersion != "" {
		if *kopsBaseURL != "" {
			return nil, fmt.Errorf("cannot set --kops-version and --kops-base-url")
		}

		var b bytes.Buffer
		if err := httpRead(*kopsVersion, &b); err != nil {
			return nil, err
		}
		latest := strings.TrimSpace(b.String())

		log.Printf("Got latest kops version from %v: %v", *kopsVersion, latest)
		if latest == "" {
			return nil, fmt.Errorf("version URL %v was empty", *kopsVersion)
		}
		*kopsBaseURL = latest
	}

	// kops looks at KOPS_BASE_URL env var, so export it here
	if *kopsBaseURL != "" {
		if err := os.Setenv("KOPS_BASE_URL", *kopsBaseURL); err != nil {
			return nil, err
		}
	}

	// Download kops from kopsBaseURL if kopsPath is not set
	if *kopsPath == "" {
		if *kopsBaseURL == "" {
			return nil, errors.New("--kops or --kops-base-url must be set")
		}

		kopsBinURL := *kopsBaseURL + "/linux/amd64/kops"
		log.Printf("Download kops binary from %s", kopsBinURL)
		kopsBin := filepath.Join(tmpdir, "kops")
		f, err := os.Create(kopsBin)
		if err != nil {
			return nil, fmt.Errorf("error creating file %q: %v", kopsBin, err)
		}
		defer f.Close()
		if err := httpRead(kopsBinURL, f); err != nil {
			return nil, err
		}
		if err := util.EnsureExecutable(kopsBin); err != nil {
			return nil, err
		}
		*kopsPath = kopsBin
	}

	return &kops{
		path:          *kopsPath,
		kubeVersion:   *kopsKubeVersion,
		sshPrivateKey: sshKey,
		sshPublicKey:  sshKey + ".pub",
		sshUser:       sshUser,
		zones:         zones,
		nodes:         *kopsNodes,
		adminAccess:   *kopsAdminAccess,
		cluster:       cluster,
		image:         *kopsImage,
		args:          *kopsArgs,
		kubecfg:       kubecfg,
		provider:      provider,
		gcpProject:    gcpProject,
		diskSize:      *kopsDiskSize,
		kopsVersion:   *kopsBaseURL,
		kopsPublish:   *kopsPublish,
		masterCount:   *kopsMasterCount,
		etcdVersion:   *kopsEtcdVersion,
		masterSize:    *kopsMasterSize,
	}, nil
}

func (k kops) isGoogleCloud() bool {
	return k.provider == "gce"
}

func (k kops) Up() error {
	// If we downloaded kubernetes, pass that version to kops
	if k.kubeVersion == "" {
		// TODO(justinsb): figure out a refactor that allows us to get this from acquireKubernetes cleanly
		kubeReleaseURL := os.Getenv("KUBERNETES_RELEASE_URL")
		kubeRelease := os.Getenv("KUBERNETES_RELEASE")
		if kubeReleaseURL != "" && kubeRelease != "" {
			if !strings.HasSuffix(kubeReleaseURL, "/") {
				kubeReleaseURL += "/"
			}
			k.kubeVersion = kubeReleaseURL + kubeRelease
		}
	}

	var featureFlags []string
	var overrides []string

	createArgs := []string{
		"create", "cluster",
		"--name", k.cluster,
		"--ssh-public-key", k.sshPublicKey,
		"--node-count", strconv.Itoa(k.nodes),
		"--node-volume-size", strconv.Itoa(k.diskSize),
		"--master-volume-size", strconv.Itoa(k.diskSize),
		"--master-count", strconv.Itoa(k.masterCount),
		"--zones", strings.Join(k.zones, ","),
	}

	// We are defaulting the master size to c4.large on AWS because m3.larges are getting less previlent.
	// When we are using GCE, then we need to handle the flag differently.
	// If we are not using gce then add the masters size flag, or if we are using gce, and the
	// master size is not set to the aws default, then add the master size flag.
	if !k.isGoogleCloud() || (k.isGoogleCloud() && k.masterSize != kopsAWSMasterSize) {
		createArgs = append(createArgs, "--master-size", k.masterSize)
	}

	if k.kubeVersion != "" {
		createArgs = append(createArgs, "--kubernetes-version", k.kubeVersion)
	}
	if k.adminAccess != "" {
		createArgs = append(createArgs, "--admin-access", k.adminAccess)
		// Enable nodeport access from the same IP (we expect it to be the test IPs)
		overrides = append(overrides, "cluster.spec.nodePortAccess="+k.adminAccess)
	}
	if k.image != "" {
		createArgs = append(createArgs, "--image", k.image)
	}
	if k.gcpProject != "" {
		createArgs = append(createArgs, "--project", k.gcpProject)
	}
	if k.isGoogleCloud() {
		featureFlags = append(featureFlags, "AlphaAllowGCE")
		createArgs = append(createArgs, "--cloud", "gce")
	} else {
		// append cloud type to allow for use of new regions without updates
		createArgs = append(createArgs, "--cloud", "aws")
	}
	if k.args != "" {
		createArgs = append(createArgs, strings.Split(k.args, " ")...)
	}
	if k.etcdVersion != "" {
		overrides = append(overrides, "cluster.spec.etcdClusters[*].version="+k.etcdVersion)
	}
	if len(overrides) != 0 {
		featureFlags = append(featureFlags, "SpecOverrideFlag")
		createArgs = append(createArgs, "--override", strings.Join(overrides, ","))
	}
	if len(featureFlags) != 0 {
		os.Setenv("KOPS_FEATURE_FLAGS", strings.Join(featureFlags, ","))
	}
	if err := control.FinishRunning(exec.Command(k.path, createArgs...)); err != nil {
		return fmt.Errorf("kops configuration failed: %v", err)
	}
	if err := control.FinishRunning(exec.Command(k.path, "update", "cluster", k.cluster, "--yes")); err != nil {
		return fmt.Errorf("kops bringup failed: %v", err)
	}

	// We require repeated successes, so we know that the cluster is stable
	// (e.g. in HA scenarios, or where we're using multiple DNS servers)
	// We use a relatively high number as DNS can take a while to
	// propagate across multiple servers / caches
	requiredConsecutiveSuccesses := 10

	// TODO(zmerlynn): More cluster validation. This should perhaps be
	// added to kops and not here, but this is a fine place to loop
	// for now.
	return waitForReadyNodes(k.nodes+1, *kopsUpTimeout, requiredConsecutiveSuccesses)
}

func (k kops) IsUp() error {
	return isUp(k)
}

func (k kops) DumpClusterLogs(localPath, gcsPath string) error {
	privateKeyPath := k.sshPrivateKey
	if strings.HasPrefix(privateKeyPath, "~/") {
		privateKeyPath = filepath.Join(os.Getenv("HOME"), privateKeyPath[2:])
	}
	key, err := ioutil.ReadFile(privateKeyPath)
	if err != nil {
		return fmt.Errorf("error reading private key %q: %v", k.sshPrivateKey, err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return fmt.Errorf("error parsing private key %q: %v", k.sshPrivateKey, err)
	}

	sshConfig := &ssh.ClientConfig{
		User: k.sshUser,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	sshClientFactory := &sshClientFactoryImplementation{
		sshConfig: sshConfig,
	}
	logDumper, err := newLogDumper(sshClientFactory, localPath)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	finished := make(chan error)
	go func() {
		finished <- k.dumpAllNodes(ctx, logDumper)
	}()

	for {
		select {
		case <-interrupt.C:
			cancel()
		case err := <-finished:
			return err
		}
	}
}

// dumpAllNodes connects to every node and dumps the logs
func (k *kops) dumpAllNodes(ctx context.Context, d *logDumper) error {
	// Make sure kubeconfig is set, in particular before calling DumpAllNodes, which calls kubectlGetNodes
	if err := k.TestSetup(); err != nil {
		return fmt.Errorf("error setting up kubeconfig: %v", err)
	}

	var additionalIPs []string
	dump, err := k.runKopsDump()
	if err != nil {
		log.Printf("unable to get cluster status from kops: %v", err)
	} else {
		for _, instance := range dump.Instances {
			name := instance.Name

			if len(instance.PublicAddresses) == 0 {
				log.Printf("ignoring instance in kops status with no public address: %v", name)
				continue
			}

			additionalIPs = append(additionalIPs, instance.PublicAddresses[0])
		}
	}

	if err := d.DumpAllNodes(ctx, additionalIPs); err != nil {
		return err
	}

	return nil
}

func (k kops) TestSetup() error {
	info, err := os.Stat(k.kubecfg)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("kubeconfig file %s not found", k.kubecfg)
		} else {
			return err
		}
	} else if info.Size() > 0 {
		// Assume that if we already have it, it's good.
		return nil
	}

	if err := control.FinishRunning(exec.Command(k.path, "export", "kubecfg", k.cluster)); err != nil {
		return fmt.Errorf("failure from 'kops export kubecfg %s': %v", k.cluster, err)
	}

	// Double-check that the file was exported
	info, err = os.Stat(k.kubecfg)
	if err != nil {
		return fmt.Errorf("kubeconfig file %s was not exported", k.kubecfg)
	}
	if info.Size() == 0 {
		return fmt.Errorf("exported kubeconfig file %s was empty", k.kubecfg)
	}

	return nil
}

// BuildTester returns a standard ginkgo-script tester, except for GCE where we build an e2e.Tester
func (k kops) BuildTester(o *e2e.BuildTesterOptions) (e2e.Tester, error) {
	// Start by only enabling this on GCE
	if !k.isGoogleCloud() {
		return &GinkgoScriptTester{}, nil
	}

	log.Printf("running ginkgo tests directly")

	t := e2e.NewGinkgoTester(o)
	t.KubeRoot = "."

	t.Kubeconfig = k.kubecfg
	t.Provider = k.provider

	if k.provider == "gce" {
		t.GCEProject = k.gcpProject
		if len(k.zones) > 0 {
			zone := k.zones[0]
			t.GCEZone = zone

			// us-central1-a => us-central1
			lastDash := strings.LastIndex(zone, "-")
			if lastDash == -1 {
				return nil, fmt.Errorf("unexpected format for GCE zone: %q", zone)
			}
			t.GCERegion = zone[0:lastDash]
		}
	}

	return t, nil
}

func (k kops) Down() error {
	// We do a "kops get" first so the exit status of "kops delete" is
	// more sensical in the case of a non-existent cluster. ("kops
	// delete" will exit with status 1 on a non-existent cluster)
	err := control.FinishRunning(exec.Command(k.path, "get", "clusters", k.cluster))
	if err != nil {
		// This is expected if the cluster doesn't exist.
		return nil
	}
	return control.FinishRunning(exec.Command(k.path, "delete", "cluster", k.cluster, "--yes"))
}

func (k kops) GetClusterCreated(gcpProject string) (time.Time, error) {
	return time.Time{}, errors.New("not implemented")
}

// kopsDump is the format of data as dumped by `kops toolbox dump -ojson`
type kopsDump struct {
	Instances []*kopsDumpInstance `json:"instances"`
}

// String implements fmt.Stringer
func (o *kopsDump) String() string {
	return util.JSONForDebug(o)
}

// kopsDumpInstance is the format of an instance (machine) in a kops dump
type kopsDumpInstance struct {
	Name            string   `json:"name"`
	PublicAddresses []string `json:"publicAddresses"`
}

// String implements fmt.Stringer
func (o *kopsDumpInstance) String() string {
	return util.JSONForDebug(o)
}

// runKopsDump runs a kops toolbox dump to dump the status of the cluster
func (k *kops) runKopsDump() (*kopsDump, error) {
	o, err := control.Output(exec.Command(k.path, "toolbox", "dump", "--name", k.cluster, "-ojson"))
	if err != nil {
		log.Printf("error running kops toolbox dump: %s\n%s", wrapError(err).Error(), string(o))
		return nil, err
	}

	dump := &kopsDump{}
	if err := json.Unmarshal(o, dump); err != nil {
		return nil, fmt.Errorf("error parsing kops toolbox dump output: %v", err)
	}

	return dump, nil
}

// kops deployer implements publisher
var _ publisher = &kops{}

// kops deployer implements e2e.TestBuilder
var _ e2e.TestBuilder = &kops{}

// Publish will publish a success file, it is called if the tests were successful
func (k kops) Publish() error {
	if k.kopsPublish == "" {
		// No publish destination set
		return nil
	}

	if k.kopsVersion == "" {
		return errors.New("kops-version not set; cannot publish")
	}

	return control.XMLWrap(&suite, "Publish kops version", func() error {
		log.Printf("Set %s version to %s", k.kopsPublish, k.kopsVersion)
		return gcsWrite(k.kopsPublish, []byte(k.kopsVersion))
	})
}

func (_ kops) KubectlCommand() (*exec.Cmd, error) { return nil, nil }

// getRandomAWSZones looks up all regions, and the availability zones for those regions.  A random
// region is then chosen and the AZ's for that region is returned. At least masterCount zones will be
// returned, all in the same region.
func getRandomAWSZones(masterCount int, multipleZones bool) ([]string, error) {

	// TODO(chrislovecnm): get the number of ec2 instances in the region and ensure that there are not too many running
	for _, i := range rand.Perm(len(awsRegions)) {
		ec2Session, err := getAWSEC2Session(awsRegions[i])
		if err != nil {
			return nil, err
		}

		// az for a region. AWS Go API does not allow us to make a single call
		zoneResults, err := ec2Session.DescribeAvailabilityZones(&ec2.DescribeAvailabilityZonesInput{})
		if err != nil {
			return nil, fmt.Errorf("unable to call aws api DescribeAvailabilityZones for %q: %v", awsRegions[i], err)
		}

		var selectedZones []string
		if len(zoneResults.AvailabilityZones) >= masterCount && multipleZones {
			for _, z := range zoneResults.AvailabilityZones {
				selectedZones = append(selectedZones, *z.ZoneName)
			}

			log.Printf("Launching cluster in region: %q", awsRegions[i])
			return selectedZones, nil
		} else if !multipleZones {
			z := zoneResults.AvailabilityZones[rand.Intn(len(zoneResults.AvailabilityZones))]
			selectedZones = append(selectedZones, *z.ZoneName)
			log.Printf("Launching cluster in region: %q", awsRegions[i])
			return selectedZones, nil
		}
	}

	return nil, fmt.Errorf("unable to find region with %d zones", masterCount)
}

// getAWSEC2Session creates an returns a EC2 API session.
func getAWSEC2Session(region string) (*ec2.EC2, error) {
	config := aws.NewConfig().WithRegion(region)

	// This avoids a confusing error message when we fail to get credentials
	config = config.WithCredentialsChainVerboseErrors(true)

	s, err := session.NewSession(config)
	if err != nil {
		return nil, fmt.Errorf("unable to build aws API session with region: %q: %v", region, err)
	}

	return ec2.New(s, config), nil

}
