// Package eksconfig defines EKS test configuration.
package eksconfig

import (
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-k8s-tester/ec2config"
	"github.com/aws/aws-k8s-tester/pkg/awsapi/ec2"
	"k8s.io/client-go/util/homedir"
	"sigs.k8s.io/yaml"
)

// Config defines EKS test configuration.
type Config struct {
	// TestMode is "embedded" or "aws-cli".
	TestMode string `json:"test-mode,omitempty"`

	// Tag is the tag used for S3 bucket name.
	// If empty, deployer auto-populates it.
	Tag string `json:"tag,omitempty"`
	// ClusterName is the cluster name.
	// If empty, deployer auto-populates it.
	ClusterName string `json:"cluster-name,omitempty"`

	// AWSK8sTesterImage is the aws-k8s-tester container image.
	// Required for "aws-k8s-tester ingress server" for ALB Ingress Controller tests.
	// Only required when ALB Ingress "TestMode" is "ingress-test-server".
	AWSK8sTesterImage string `json:"aws-k8s-tester-image,omitempty"`

	// AWSK8sTesterPath is the path to download the "aws-k8s-tester".
	// This is required for Kubernetes kubetest plugin.
	AWSK8sTesterPath        string `json:"aws-k8s-tester-path,omitempty"`
	AWSK8sTesterDownloadURL string `json:"aws-k8s-tester-download-url,omitempty"`
	// KubectlPath is the path to download the "kubectl".
	KubectlPath        string `json:"kubectl-path,omitempty"`
	KubectlDownloadURL string `json:"kubectl-download-url,omitempty"`
	// AWSIAMAuthenticatorPath is the path to download the "aws-iam-authenticator".
	// This is required for Kubernetes kubetest plugin.
	AWSIAMAuthenticatorPath        string `json:"aws-iam-authenticator-path,omitempty"`
	AWSIAMAuthenticatorDownloadURL string `json:"aws-iam-authenticator-download-url,omitempty"`

	// ConfigPath is the configuration file path.
	// Must be left empty, and let deployer auto-populate this field.
	// Deployer is expected to update this file with latest status,
	// and to make a backup of original configuration
	// with the filename suffix ".backup.yaml" in the same directory.
	ConfigPath       string `json:"config-path,omitempty"`
	ConfigPathBucket string `json:"config-path-bucket,omitempty"` // read-only to user
	ConfigPathURL    string `json:"config-path-url,omitempty"`    // read-only to user

	// KubeConfigPath is the file path of KUBECONFIG for the EKS cluster.
	// If empty, auto-generate one.
	// Deployer is expected to delete this on cluster tear down.
	KubeConfigPath       string `json:"kubeconfig-path,omitempty"`        // read-only to user
	KubeConfigPathBucket string `json:"kubeconfig-path-bucket,omitempty"` // read-only to user
	KubeConfigPathURL    string `json:"kubeconfig-path-url,omitempty"`    // read-only to user

	// WaitBeforeDown is the duration to sleep before cluster tear down.
	WaitBeforeDown time.Duration `json:"wait-before-down,omitempty"`
	// Down is true to automatically tear down cluster in "test".
	// Deployer implementation should not call "Down" inside "Up" method.
	// This is meant to be used as a flag for test.
	Down bool `json:"down"`

	// EnableWorkerNodeSSH is true to enable SSH access to worker nodes.
	EnableWorkerNodeSSH bool `json:"enable-worker-node-ssh"`
	// EnableWorkerNodeHA is true to use all 3 subnets to create worker nodes.
	// Note that at least 2 subnets are required for EKS cluster.
	EnableWorkerNodeHA bool `json:"enable-worker-node-ha"`

	// VPCID is the VPC ID.
	VPCID string `json:"vpc-id"`
	// SubnetIDs is the subnet IDs.
	SubnetIDs []string `json:"subnet-ids"`
	// SecurityGroupID is the default security group ID.
	SecurityGroupID string `json:"security-group-id"`

	// AWSAccountID is the AWS account ID.
	AWSAccountID string `json:"aws-account-id,omitempty"`
	// AWSCredentialToMountPath is the file path to AWS credential.
	// Required for AWS ALB Ingress Controller deployments and other AWS specific tests.
	// If not empty, deployer is expected to mount the file as a secret object "aws-cred-aws-k8s-tester",
	// to the path "/etc/aws-cred-aws-k8s-tester/aws-cred-aws-k8s-tester", under "kube-system" namespace.
	// Path must be an absolute path, although it will try to parse '~/.aws' or '${HOME}/.aws'.
	// If "AWS_SHARED_CREDENTIALS_FILE" is specified, this field will overwritten.
	AWSCredentialToMountPath string `json:"aws-credential-to-mount-path,omitempty"`
	// AWSRegion is the AWS geographic area for EKS deployment.
	// Currently supported regions are:
	// - us-east-1; US East (N. Virginia)
	// - us-west-2; US West (Oregon)
	// - eu-west-1; EU West (Dublin)
	// If empty, set default region.
	AWSRegion string `json:"aws-region,omitempty"`
	// AWSCustomEndpoint defines AWS custom endpoint for pre-release versions.
	// Must be left empty to use production EKS service.
	// TODO: define custom endpoints for CloudFormation, EC2, STS
	AWSCustomEndpoint string `json:"aws-custom-endpoint,omitempty"`

	// WorkerNodeAMI is the Amazon EKS worker node AMI ID for the specified Region.
	// Reference https://docs.aws.amazon.com/eks/latest/userguide/getting-started.html.
	WorkerNodeAMI string `json:"worker-node-ami,omitempty"`
	// WorkerNodeInstanceType is the EC2 instance type for worker nodes.
	WorkerNodeInstanceType string `json:"worker-node-instance-type,omitempty"`
	// WorkerNodeASGMin is the minimum number of nodes in worker node ASG.
	WorkerNodeASGMin int `json:"worker-node-asg-min,omitempty"`
	// WorkerNodeASGMax is the maximum number of nodes in worker node ASG.
	WorkerNodeASGMax int `json:"worker-node-asg-max,omitempty"`
	// WorkerNodeVolumeSizeGB is the maximum number of nodes in worker node ASG.
	// If empty, set default value.
	WorkerNodeVolumeSizeGB int `json:"worker-node-volume-size-gb,omitempty"`

	// KubernetesVersion is the version of Kubernetes cluster.
	// If empty, set default version.
	KubernetesVersion string `json:"kubernetes-version,omitempty"`
	// PlatformVersion is the platform version of EKS.
	// Read-only to user.
	PlatformVersion string `json:"platform-version,omitempty"`

	// LogDebug is true to enable debug level logging.
	LogDebug bool `json:"log-debug"`
	// LogOutputs is a list of log outputs. Valid values are 'default', 'stderr', 'stdout', or file names.
	// Logs are appended to the existing file, if any.
	// Multiple values are accepted. If empty, it sets to 'default', which outputs to stderr.
	// See https://godoc.org/go.uber.org/zap#Open and https://godoc.org/go.uber.org/zap#Config for more details.
	LogOutputs []string `json:"log-outputs,omitempty"`
	// LogOutputToUploadPath is the aws-k8s-tester log file path to upload to cloud storage.
	// Must be left empty.
	// This will be overwritten by cluster name.
	LogOutputToUploadPath       string `json:"log-output-to-upload-path,omitempty"`
	LogOutputToUploadPathBucket string `json:"log-output-to-upload-path-bucket,omitempty"`
	LogOutputToUploadPathURL    string `json:"log-output-to-upload-path-url,omitempty"`

	// LogAccess is true to enable AWS API access logs (e.g. ALB access logs).
	// Automatically uploaded to S3 bucket named by cluster name.
	// https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-access-logs.html
	// https://github.com/kubernetes-sigs/aws-alb-ingress-controller/blob/master/docs/ingress-resources.md
	LogAccess bool `json:"log-access"`

	// UploadTesterLogs is true to auto-upload log files.
	UploadTesterLogs bool `json:"upload-tester-logs"`
	// UploadWorkerNodeLogs is true to auto-upload worker node log files.
	UploadWorkerNodeLogs bool `json:"upload-worker-node-logs"`

	// UpdatedAt is the timestamp when the configuration has been updated.
	// Read only to 'Config' struct users.
	UpdatedAt time.Time `json:"updated-at,omitempty"` // read-only to user

	// ClusterState is the EKS status state.
	// Deployer is expected to keep this in sync.
	// Read-only to kubetest.
	ClusterState *ClusterState `json:"cluster-state,omitempty"`

	// ALBIngressController is the EKS ALB Ingress Controller configuration and its state.
	// Deployer is expected to keep this in sync.
	// Read-only to kubetest.
	ALBIngressController *ALBIngressController `json:"alb-ingress-controller,omitempty"`
}

// ClusterState contains EKS cluster specific states.
// Deployer is expected to write and read this.
// Read-only to kubetest.
type ClusterState struct {
	// Status is the cluster status from EKS API.
	// It's either CREATING, ACTIVE, DELETING, FAILED, "DELETE_COMPLETE".
	// Reference: https://docs.aws.amazon.com/eks/latest/APIReference/API_Cluster.html#AmazonEKS-Type-Cluster-status.
	Status string `json:"status,omitempty"` // read-only to user

	StatusRoleCreated       bool `json:"status-role-created"`        // read-only to user
	StatusPolicyAttached    bool `json:"status-policy-attached"`     // read-only to user
	StatusVPCCreated        bool `json:"status-vpc-created"`         // read-only to user
	StatusClusterCreated    bool `json:"status-cluster-created"`     // read-only to user
	StatusKeyPairCreated    bool `json:"status-key-pair-created"`    // read-only to user
	StatusWorkerNodeCreated bool `json:"status-worker-node-created"` // read-only to user

	// Created is the timestamp of cluster creation.
	Created time.Time `json:"created,omitempty"` // read-only to user

	// UpTook is total duration that took to set up cluster up and running.
	// Does not include sub-project resource creation (e.g. ALB Ingress Controller).
	UpTook string        `json:"up-took,omitempty"` // read-only to user
	upTook time.Duration // read-only to user

	// ServiceRoleWithPolicyName is the name of the EKS cluster service role with policy.
	// Prefixed with cluster name and suffixed with 'SERVICE-ROLE'.
	ServiceRoleWithPolicyName string `json:"service-role-with-policy-name,omitempty"`
	// ServiceRolePolicies is the list of policy ARNs to create cluster service role with.
	ServiceRolePolicies []string `json:"service-role-policies,omitempty"`
	// ServiceRoleWithPolicyARN is the ARN of the created cluster service role.
	ServiceRoleWithPolicyARN string `json:"service-role-with-policy-arn,omitempty"`

	// CFStackVPCName is the name of VPC cloudformation stack.
	CFStackVPCName string `json:"cf-stack-vpc-name,omitempty"`
	// CFStackVPCStatus is the last cloudformation status of VPC stack.
	CFStackVPCStatus string `json:"cf-stack-vpc-status,omitempty"`

	// Endpoint is the cluster endpoint of the EKS cluster, required for KUBECONFIG write.
	Endpoint string `json:"endpoint,omitempty"`
	// CA is the EKS cluster CA, required for KUBECONFIG write.
	CA string `json:"ca,omitempty"`

	// WorkerNodeGroupStatus is the status Kubernetes worker node group.
	// "READY" when they successfully join the EKS cluster as worker nodes.
	WorkerNodeGroupStatus string `json:"worker-node-group-status,omitempty"`
	// WorkerNodes is a list of worker nodes.
	WorkerNodes map[string]ec2config.Instance `json:"worker-nodes,omitempty"`

	// WorkerNodeLogs is a list of worker node log file paths, fetched via SSH.
	WorkerNodeLogs map[string]string `json:"worker-node-logs,omitempty"`

	// CFStackWorkerNodeGroupName is the name of cloudformation stack for worker node group.
	CFStackWorkerNodeGroupName string `json:"cf-stack-worker-node-group-name,omitempty"`
	// CFStackWorkerNodeGroupStatus is the last cloudformation status of node group stack.
	CFStackWorkerNodeGroupStatus string `json:"cf-stack-worker-node-group-status,omitempty"`
	// CFStackWorkerNodeGroupKeyPairName is required for node group creation.
	CFStackWorkerNodeGroupKeyPairName string `json:"cf-stack-worker-node-group-key-pair-name,omitempty"`
	// CFStackWorkerNodeGroupKeyPairPrivateKeyPath is the file path to store node group key pair private key.
	// Thus, deployer must delete the private key right after node group creation.
	// MAKE SURE PRIVATE KEY NEVER GETS UPLOADED TO CLOUD STORAGE AND DLETE AFTER USE!!!
	CFStackWorkerNodeGroupKeyPairPrivateKeyPath string `json:"cf-stack-worker-node-group-key-pair-private-key-path,omitempty"`
	// CFStackWorkerNodeGroupSecurityGroupID is the security group ID
	// that worker node cloudformation stack created.
	CFStackWorkerNodeGroupSecurityGroupID string `json:"cf-stack-worker-node-group-security-group-id,omitempty"`
	// CFStackWorkerNodeGroupAutoScalingGroupName is the name of worker node auto scaling group.
	CFStackWorkerNodeGroupAutoScalingGroupName string `json:"cf-stack-worker-node-group-auto-scaling-group-name,omitempty"`

	// CFStackWorkerNodeGroupWorkerNodeInstanceRoleARN is the ARN of NodeInstance role of node group.
	// Required to enable worker nodes to join cluster.
	// Update this after creating node group stack
	CFStackWorkerNodeGroupWorkerNodeInstanceRoleARN string `json:"cf-stack-worker-node-group-worker-node-instance-role-arn,omitempty"`
}

// ALBIngressController configures ingress controller for EKS.
type ALBIngressController struct {
	// Created is true if ALB had started its creation operation.
	Created bool `json:"created"`
	// Enable is true to create an ALB Ingress Controller with sample ingress deployment.
	// 'AWSCredentialToMountPath' must be provided to configure ALB Ingress Controller.
	Enable bool `json:"enable"`

	// IngressControllerImage is the ALB Ingress Controller container image.
	IngressControllerImage string `json:"ingress-controller-image,omitempty"`
	// UploadTesterLogs is true to auto-upload ALB tester logs.
	UploadTesterLogs bool `json:"upload-tester-logs"`

	// TargetType specifies the target type for target groups:
	// - 'instance' to use node port
	// - 'ip' to use pod IP
	// e.g. alb.ingress.kubernetes.io/target-type: instance
	// e.g. alb.ingress.kubernetes.io/target-type: ip
	// With instance the Target Group targets are <ec2 instance id>:<node port>,
	// for ip the targets are <pod ip>:<pod port>.
	// ip is to be used when the pod network is routable and can be reached by the ALB.
	// https://github.com/kubernetes-sigs/aws-alb-ingress-controller/blob/master/docs/ingress-resources.md
	TargetType string `json:"target-type,omitempty"`
	// TestMode is either "ingress-test-server" or "nginx".
	TestMode string `json:"test-mode,omitempty"`

	// TestScalability is true to run scalability tests.
	TestScalability bool `json:"test-scalability"`
	// TestScalabilityMinutes is the number of minutes to send scalability test workloads.
	// Reference: https://github.com/wg/wrk#command-line-options.
	TestScalabilityMinutes int `json:"test-scalability-minutes"`
	// TestMetrics is true to run metrics tests.
	TestMetrics bool `json:"test-metrics"`
	// TestServerReplicas is the number of ingress test server pods to deploy.
	TestServerReplicas int `json:"test-server-replicas,omitempty"`
	// TestServerRoutes is the number of ALB Ingress Controller routes to test.
	// It will be auto-generated starting with '/ingress-test-0000000'.
	// Supports up to 30.
	// Only required when ALB Ingress "TestMode" is "ingress-test-server".
	// Otherwise, set it to 1.
	TestServerRoutes int `json:"test-server-routes,omitempty"`
	// TestClients is the number of concurrent ALB Ingress Controller test clients.
	// Supports up to 300.
	TestClients int `json:"test-clients,omitempty"`
	// TestClientRequests is the number of ALB Ingress Controller test requests.
	// This is ignored when test mode is nginx (because it will use "wrk" for QPS tests).
	TestClientRequests int `json:"test-client-requests,omitempty"`
	// TestResponseSize is the response payload size.
	// Ingress test server always returns '0' x response size.
	// Supports up to 500 KB.
	TestResponseSize int `json:"test-response-size,omitempty"`
	// TestClientErrorThreshold is the maximum errors that are ok to happen before failing the tests.
	TestClientErrorThreshold int64 `json:"test-client-error-threshold,omitempty"`
	// TestExpectQPS is the expected QPS.
	// It is used as a scalability test lower bound.
	TestExpectQPS float64 `json:"test-expect-qps,omitempty"`
	// TestResultQPS is the QPS of last test run.
	TestResultQPS float64 `json:"test-result-qps,omitempty"`
	// TestResultFailures is the number of failed requests of last test run.
	TestResultFailures int64 `json:"test-result-failures,omitempty"`

	// IngressTestServerDeploymentServiceSpecPath is the file path to test pod deployment and service YAML spec.
	IngressTestServerDeploymentServiceSpecPath       string `json:"ingress-test-server-deployment-service-spec-path,omitempty"`
	IngressTestServerDeploymentServiceSpecPathBucket string `json:"ingress-test-server-deployment-service-spec-path-bucket,omitempty"`
	IngressTestServerDeploymentServiceSpecPathURL    string `json:"ingress-test-server-deployment-service-spec-path-url,omitempty"`
	// IngressControllerSpecPath is the file path to ALB Ingress Controller YAML spec.
	IngressControllerSpecPath       string `json:"ingress-controller-spec-path,omitempty"`
	IngressControllerSpecPathBucket string `json:"ingress-controller-spec-path-bucket,omitempty"`
	IngressControllerSpecPathURL    string `json:"ingress-controller-spec-path-url,omitempty"`
	// IngressObjectSpecPath is the file path to Ingress object YAML spec.
	IngressObjectSpecPath       string `json:"ingress-object-spec-path,omitempty"`
	IngressObjectSpecPathBucket string `json:"ingress-object-spec-path-bucket,omitempty"`
	IngressObjectSpecPathURL    string `json:"ingress-object-spec-path-url,omitempty"`

	// required for ALB Ingress Controller
	// Ingress object requires:
	//  - Subnet IDs from VPC stack
	//  - Security Group IDs
	//    - one from "aws ec2 describe-security-groups" with VPC stack VPC ID
	//    - the other from "aws ec2 create-security-group" for ALB port wide open
	// Thus, pass "SecurityGroupID" and "SecurityGroupIDPortOpen" for Ingress object

	// ELBv2SecurityGroupIDPortOpen is the security group ID created to
	// open 80 and 443 ports for ALB Ingress Controller.
	ELBv2SecurityGroupIDPortOpen string `json:"elbv2-security-group-id-port-open,omitempty"`
	// ELBv2NamespaceToDNSName maps each namespace to ALB Ingress DNS name (address).
	ELBv2NamespaceToDNSName map[string]string `json:"elbv2-namespace-to-dns-name,omitempty"`
	// ELBv2NameToDNSName maps each ALB name to its DNS name.
	// e.g. address is 431f09fb-default-ingressfo-0222-899555794.us-west-2.elb.amazonaws.com,
	// then AWS ELBv2 name is 431f09fb-default-ingressfo-0222.
	ELBv2NameToDNSName map[string]string `json:"elbv2-name-to-dns-name,omitempty"`
	// ELBv2NameToARN maps each ALB name to its ARN.
	// Useful for garbage collection.
	ELBv2NameToARN map[string]string `json:"elbv2-name-to-arn,omitempty"`
	// ELBv2SecurityGroupStatus is the status of ALB Ingress Controller security group creation.
	ELBv2SecurityGroupStatus string `json:"elbv2-security-group-status,omitempty"`

	// DeploymentStatus is the deployment status of ALB Ingress Controller itself.
	DeploymentStatus string `json:"deployment-status,omitempty"`
	// IngressRuleStatusKubeSystem is the status of ALB Ingress Controller Ingress
	// rule creation for default namespace.
	IngressRuleStatusKubeSystem string `json:"ingress-rule-status-kube-system,omitempty"`
	// IngressRuleStatusDefault is the status of ALB Ingress Controller Ingress
	// rule creation for default namespace.
	IngressRuleStatusDefault string `json:"ingress-rule-status-default,omitempty"`

	// IngressUpTook is total duration that took to set up ALB Ingress Controller.
	// Include Ingress object creation and DNS propagation.
	IngressUpTook string `json:"ingress-up-took,omitempty"`
	ingressUpTook time.Duration

	// ScalabilityOutputToUploadPath is the ALB Ingress Controller scalability
	// test output file path to upload to cloud storage.
	// Must be left empty.
	// This will be overwritten by cluster name.
	ScalabilityOutputToUploadPath       string `json:"scalability-output-to-upload-path,omitempty"`
	ScalabilityOutputToUploadPathBucket string `json:"scalability-output-to-upload-path-bucket,omitempty"`
	ScalabilityOutputToUploadPathURL    string `json:"scalability-output-to-upload-path-url,omitempty"`
	// MetricsOutputToUploadPath is the ALB Ingress Controller metrics output
	// file path to upload to cloud storage.
	// Must be left empty.
	// This will be overwritten by cluster name.
	MetricsOutputToUploadPath       string `json:"metrics-output-to-upload-path,omitempty"`
	MetricsOutputToUploadPathBucket string `json:"metrics-output-to-upload-path-bucket,omitempty"`
	MetricsOutputToUploadPathURL    string `json:"metrics-output-to-upload-path-url,omitempty"`
}

// NewDefault returns a copy of the default configuration.
func NewDefault() *Config {
	vv := defaultConfig
	return &vv
}

func init() {
	defaultConfig.Tag = genTag()
	defaultConfig.ClusterName = defaultConfig.Tag + "-" + randString(7)
	if runtime.GOOS == "darwin" {
		defaultConfig.KubectlDownloadURL = strings.Replace(defaultConfig.KubectlDownloadURL, "linux", "darwin", -1)
		defaultConfig.AWSIAMAuthenticatorDownloadURL = strings.Replace(defaultConfig.AWSIAMAuthenticatorDownloadURL, "linux", "darwin", -1)
	}
}

// genTag generates a tag for cluster name, CloudFormation, and S3 bucket.
// Note that this would be used as S3 bucket name to upload tester logs.
func genTag() string {
	// use UTC time for everything
	now := time.Now().UTC()
	return fmt.Sprintf("awsk8stester-eks-%d%02d%02d", now.Year(), now.Month(), now.Day())
}

// defaultConfig is the default configuration.
//  - empty string creates a non-nil object for pointer-type field
//  - omitting an entire field returns nil value
//  - make sure to check both
var defaultConfig = Config{
	TestMode: "embedded",

	AWSK8sTesterDownloadURL:        "https://github.com/aws/aws-k8s-tester/releases/download/0.1.2/aws-k8s-tester-0.1.2-linux-amd64",
	AWSK8sTesterPath:               "/tmp/aws-k8s-tester/aws-k8s-tester",
	KubectlDownloadURL:             "https://amazon-eks.s3-us-west-2.amazonaws.com/1.10.3/2018-07-26/bin/linux/amd64/kubectl",
	KubectlPath:                    "/tmp/aws-k8s-tester/kubectl",
	AWSIAMAuthenticatorDownloadURL: "https://amazon-eks.s3-us-west-2.amazonaws.com/1.10.3/2018-07-26/bin/linux/amd64/aws-iam-authenticator",
	AWSIAMAuthenticatorPath:        "/tmp/aws-k8s-tester/aws-iam-authenticator",

	// enough time for ALB access log
	WaitBeforeDown: time.Minute,
	Down:           true,

	EnableWorkerNodeHA:  true,
	EnableWorkerNodeSSH: true,

	AWSAccountID: "",
	// to be overwritten by AWS_SHARED_CREDENTIALS_FILE
	AWSCredentialToMountPath: filepath.Join(homedir.HomeDir(), ".aws", "credentials"),
	AWSRegion:                "us-west-2",
	AWSCustomEndpoint:        "",

	// Amazon EKS-optimized AMI, https://docs.aws.amazon.com/eks/latest/userguide/getting-started.html
	WorkerNodeAMI: "ami-0f54a2f7d2e9c88b3",

	WorkerNodeInstanceType: "m5.large",
	WorkerNodeASGMin:       1,
	WorkerNodeASGMax:       1,
	WorkerNodeVolumeSizeGB: 20,

	KubernetesVersion: "1.10",

	LogDebug: false,

	// default, stderr, stdout, or file name
	// log file named with cluster name will be added automatically
	LogOutputs:           []string{"stderr"},
	LogAccess:            false,
	UploadTesterLogs:     false,
	UploadWorkerNodeLogs: false,

	ClusterState: &ClusterState{},
	ALBIngressController: &ALBIngressController{
		Enable:           false,
		UploadTesterLogs: false,

		IngressControllerImage: "quay.io/coreos/alb-ingress-controller:1.0-beta.7",

		// 'instance' to use node port
		// 'ip' to use pod IP
		TargetType: "instance",
		TestMode:   "nginx",

		TestScalability:          true,
		TestScalabilityMinutes:   1,
		TestMetrics:              true,
		TestServerReplicas:       1,
		TestServerRoutes:         1,
		TestClients:              200,
		TestClientRequests:       20000,
		TestResponseSize:         40 * 1024, // 40 KB
		TestClientErrorThreshold: 10,
		TestExpectQPS:            20000,
	},
}

// Load loads configuration from YAML.
// Useful when injecting shared configuration via ConfigMap.
//
// Example usage:
//
//  import "github.com/aws/aws-k8s-tester/eksconfig"
//  cfg := eksconfig.Load("test.yaml")
//  p, err := cfg.BackupConfig()
//  err = cfg.ValidateAndSetDefaults()
//
// Do not set default values in this function.
// "ValidateAndSetDefaults" must be called separately,
// to prevent overwriting previous data when loaded from disks.
func Load(p string) (cfg *Config, err error) {
	var d []byte
	d, err = ioutil.ReadFile(p)
	if err != nil {
		return nil, err
	}
	cfg = new(Config)
	if err = yaml.Unmarshal(d, cfg); err != nil {
		return nil, err
	}

	if cfg.ClusterState == nil {
		cfg.ClusterState = &ClusterState{}
	}
	if cfg.ALBIngressController == nil {
		cfg.ALBIngressController = &ALBIngressController{}
	}

	cfg.ConfigPath, err = filepath.Abs(p)
	if err != nil {
		return nil, err
	}
	if cfg.ClusterState.UpTook != "" {
		cfg.ClusterState.upTook, err = time.ParseDuration(cfg.ClusterState.UpTook)
		if err != nil {
			return nil, err
		}
	}
	if cfg.ALBIngressController.IngressUpTook != "" {
		cfg.ALBIngressController.ingressUpTook, err = time.ParseDuration(cfg.ALBIngressController.IngressUpTook)
		if err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

// Sync persists current configuration and states to disk.
func (cfg *Config) Sync() (err error) {
	if !filepath.IsAbs(cfg.ConfigPath) {
		cfg.ConfigPath, err = filepath.Abs(cfg.ConfigPath)
		if err != nil {
			return err
		}
	}
	cfg.UpdatedAt = time.Now().UTC()
	var d []byte
	d, err = yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	fmt.Println("writing to:", cfg.ConfigPath)
	return ioutil.WriteFile(cfg.ConfigPath, d, 0600)
}

// BackupConfig stores the original aws-k8s-tester configuration
// file to backup, suffixed with ".backup.yaml".
// Otherwise, deployer will overwrite its state back to YAML.
// Useful when the original configuration would be reused
// for other tests.
func (cfg *Config) BackupConfig() (p string, err error) {
	var d []byte
	d, err = ioutil.ReadFile(cfg.ConfigPath)
	if err != nil {
		return "", err
	}
	p = fmt.Sprintf("%s.%X.backup.yaml",
		cfg.ConfigPath,
		time.Now().UTC().UnixNano(),
	)
	return p, ioutil.WriteFile(p, d, 0600)
}

const (
	// maxTestServerRoutes is the maximum number of routes.
	maxTestServerRoutes = 30
	// maxTestClients is the maximum number of clients.
	maxTestClients = 1000
	// maxTestClientRequests is the maximum number of requests.
	maxTestClientRequests = 50000
	// maxTestResponseSize is the maximum response size for ingress test server.
	maxTestResponseSize = 500 * 1024 // 500 KB == 4000 Kbit
)

// ValidateAndSetDefaults returns an error for invalid configurations.
// And updates empty fields with default values.
// At the end, it writes populated YAML to aws-k8s-tester config path.
func (cfg *Config) ValidateAndSetDefaults() error {
	switch cfg.TestMode {
	case "embedded":
	case "aws-cli":
		// TODO: remove this
		return errors.New("TestMode 'aws-cli' is not implemented yet")
	default:
		return fmt.Errorf("TestMode %q unknown", cfg.TestMode)
	}
	if len(cfg.LogOutputs) == 0 {
		return errors.New("EKS LogOutputs is not specified")
	}
	if cfg.KubernetesVersion == "" {
		return errors.New("Kubernetes version is empty")
	}
	if !checkKubernetesVersion(cfg.KubernetesVersion) {
		return fmt.Errorf("EKS Kubernetes version %q is not valid", cfg.KubernetesVersion)
	}
	if cfg.AWSRegion == "" {
		return errors.New("AWS Region is empty")
	}
	if !checkRegion(cfg.AWSRegion) {
		return fmt.Errorf("EKS Region %q is not valid", cfg.AWSRegion)
	}
	if cfg.Tag == "" {
		return errors.New("Tag is empty")
	}
	if cfg.ClusterName == "" {
		return errors.New("ClusterName is empty")
	}
	if cfg.WorkerNodeAMI == "" {
		return errors.New("EKS WorkerNodeAMI is not specified")
	}
	if !checkAMI(cfg.AWSRegion, cfg.WorkerNodeAMI) {
		return fmt.Errorf("EKS WorkerNodeAMI %q is not valid", cfg.WorkerNodeAMI)
	}
	if cfg.WorkerNodeInstanceType == "" {
		return errors.New("EKS WorkerNodeInstanceType is not specified")
	}
	if !checkEC2InstanceType(cfg.WorkerNodeInstanceType) {
		return fmt.Errorf("EKS WorkerNodeInstanceType %q is not valid", cfg.WorkerNodeInstanceType)
	}
	if cfg.ALBIngressController != nil && cfg.ALBIngressController.TestServerReplicas > 0 {
		if !checkMaxPods(cfg.WorkerNodeInstanceType, cfg.WorkerNodeASGMax, cfg.ALBIngressController.TestServerReplicas) {
			return fmt.Errorf(
				"EKS WorkerNodeInstanceType %q only supports %d pods per node (ASG Max %d, allowed up to %d, test server replicas %d)",
				cfg.WorkerNodeInstanceType,
				ec2.InstanceTypes[cfg.WorkerNodeInstanceType].MaxPods,
				cfg.WorkerNodeASGMax,
				ec2.InstanceTypes[cfg.WorkerNodeInstanceType].MaxPods*int64(cfg.WorkerNodeASGMax),
				cfg.ALBIngressController.TestServerReplicas,
			)
		}
	}
	if cfg.WorkerNodeASGMin == 0 {
		return errors.New("EKS WorkerNodeASGMin is not specified")
	}
	if cfg.WorkerNodeASGMax == 0 {
		return errors.New("EKS WorkerNodeASGMax is not specified")
	}
	if !checkWorkderNodeASG(cfg.WorkerNodeASGMin, cfg.WorkerNodeASGMax) {
		return fmt.Errorf("EKS WorkderNodeASG %d and %d is not valid", cfg.WorkerNodeASGMin, cfg.WorkerNodeASGMax)
	}
	if cfg.WorkerNodeVolumeSizeGB == 0 {
		cfg.WorkerNodeVolumeSizeGB = defaultWorkderNodeVolumeSizeGB
	}
	if ok := checkEKSEp(cfg.AWSCustomEndpoint); !ok {
		return fmt.Errorf("AWSCustomEndpoint %q is not valid", cfg.AWSCustomEndpoint)
	}

	// resources created from aws-k8s-tester always follow
	// the same naming convention
	cfg.ClusterState.ServiceRoleWithPolicyName = genServiceRoleWithPolicy(cfg.ClusterName)
	cfg.ClusterState.ServiceRolePolicies = []string{serviceRolePolicyARNCluster, serviceRolePolicyARNService}
	cfg.ClusterState.CFStackVPCName = genCFStackVPC(cfg.ClusterName)
	cfg.ClusterState.CFStackWorkerNodeGroupKeyPairName = genNodeGroupKeyPairName(cfg.ClusterName)
	// SECURITY NOTE: MAKE SURE PRIVATE KEY NEVER GETS UPLOADED TO CLOUD STORAGE AND DLETE AFTER USE!!!
	cfg.ClusterState.CFStackWorkerNodeGroupKeyPairPrivateKeyPath = filepath.Join(
		os.TempDir(),
		cfg.ClusterState.CFStackWorkerNodeGroupKeyPairName+".private.key",
	)
	cfg.ClusterState.CFStackWorkerNodeGroupName = genCFStackWorkerNodeGroup(cfg.ClusterName)

	////////////////////////////////////////////////////////////////////////
	// populate all paths on disks and on remote storage
	if cfg.ConfigPath == "" {
		f, err := ioutil.TempFile(os.TempDir(), "awsk8stester-eksconfig")
		if err != nil {
			return err
		}
		cfg.ConfigPath, _ = filepath.Abs(f.Name())
		f.Close()
		os.RemoveAll(cfg.ConfigPath)
	}
	cfg.ConfigPathBucket = filepath.Join(cfg.ClusterName, "awsk8stester-eksconfig.yaml")

	cfg.LogOutputToUploadPath = filepath.Join(os.TempDir(), fmt.Sprintf("%s.log", cfg.ClusterName))
	logOutputExist := false
	for _, lv := range cfg.LogOutputs {
		if cfg.LogOutputToUploadPath == lv {
			logOutputExist = true
			break
		}
	}
	if !logOutputExist {
		// auto-insert generated log output paths to zap logger output list
		cfg.LogOutputs = append(cfg.LogOutputs, cfg.LogOutputToUploadPath)
	}
	cfg.LogOutputToUploadPathBucket = filepath.Join(cfg.ClusterName, "awsk8stester-eks.log")

	// cfg.KubeConfigPath = fmt.Sprintf("%s.%s.kubeconfig.generated.yaml", cfg.ConfigPath, cfg.ClusterName)
	cfg.KubeConfigPath = "/tmp/aws-k8s-tester/kubeconfig"
	cfg.KubeConfigPathBucket = filepath.Join(cfg.ClusterName, "kubeconfig")

	cfg.ALBIngressController.IngressTestServerDeploymentServiceSpecPath = fmt.Sprintf(
		"%s.%s.alb.ingress-test-server.yaml",
		cfg.ConfigPath,
		cfg.ClusterName,
	)
	cfg.ALBIngressController.IngressTestServerDeploymentServiceSpecPathBucket = filepath.Join(
		cfg.ClusterName,
		"alb.ingress-test-server.deployment.service.yaml",
	)

	cfg.ALBIngressController.IngressControllerSpecPath = fmt.Sprintf(
		"%s.%s.alb.controller.deployment.service.yaml",
		cfg.ConfigPath,
		cfg.ClusterName,
	)
	cfg.ALBIngressController.IngressControllerSpecPathBucket = filepath.Join(
		cfg.ClusterName,
		"alb.controller.deployment.service.yaml",
	)

	cfg.ALBIngressController.IngressObjectSpecPath = fmt.Sprintf(
		"%s.%s.alb.ingress.yaml",
		cfg.ConfigPath,
		cfg.ClusterName,
	)
	cfg.ALBIngressController.IngressObjectSpecPathBucket = filepath.Join(
		cfg.ClusterName,
		"alb.ingress.yaml",
	)

	cfg.ALBIngressController.ScalabilityOutputToUploadPath = fmt.Sprintf(
		"%s.%s.alb.scalability.txt",
		cfg.ConfigPath,
		cfg.ClusterName,
	)
	cfg.ALBIngressController.ScalabilityOutputToUploadPathBucket = filepath.Join(
		cfg.ClusterName,
		"alb.scalability.txt",
	)

	cfg.ALBIngressController.MetricsOutputToUploadPath = fmt.Sprintf(
		"%s.%s.alb.metrics.txt",
		cfg.ConfigPath,
		cfg.ClusterName,
	)
	cfg.ALBIngressController.MetricsOutputToUploadPathBucket = filepath.Join(
		cfg.ClusterName,
		"alb.metrics.txt",
	)
	////////////////////////////////////////////////////////////////////////

	if cfg.AWSCredentialToMountPath != "" && os.Getenv("AWS_SHARED_CREDENTIALS_FILE") == "" {
		p := cfg.AWSCredentialToMountPath
		if filepath.IsAbs(p) && !exist(p) {
			return fmt.Errorf("AWSCredentialToMountPath %q does not exist", cfg.AWSCredentialToMountPath)
		}
		// expand manually
		if strings.HasPrefix(p, "~/.aws") ||
			strings.HasPrefix(p, "$HOME/.aws") ||
			strings.HasPrefix(p, "${HOME}/.aws") {
			p = filepath.Join(homedir.HomeDir(), ".aws", filepath.Base(p))
		}
		if !exist(p) {
			return fmt.Errorf("AWSCredentialToMountPath %q does not exist", p)
		}
		cfg.AWSCredentialToMountPath = p
	}

	// overwrite with env
	if os.Getenv("AWS_SHARED_CREDENTIALS_FILE") != "" {
		p := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
		var err error
		p, err = filepath.Abs(p)
		if err != nil {
			return fmt.Errorf("failed to expand AWS_SHARED_CREDENTIALS_FILE %q (%v)", p, err)
		}
		if !exist(p) {
			return fmt.Errorf("AWS_SHARED_CREDENTIALS_FILE %q does not exist", p)
		}
		cfg.AWSCredentialToMountPath = p
	}

	if cfg.AWSCredentialToMountPath == "" && cfg.ALBIngressController != nil {
		if cfg.ALBIngressController.Enable {
			return errors.New("cannot create AWS ALB Ingress Controller without AWS credential")
		}
		if cfg.ALBIngressController.IngressControllerImage == "" {
			return errors.New("cannot create AWS ALB Ingress Controller without ingress controller test image")
		}
		if cfg.ALBIngressController.TestServerRoutes > 0 {
			return errors.New("cannot create AWS ALB Ingress Controller routes without test routes")
		}
		if cfg.ALBIngressController.TestClients > 0 {
			return errors.New("cannot create AWS ALB Ingress Controller clients without test clients")
		}
		if cfg.ALBIngressController.TestClientRequests > 0 {
			return errors.New("cannot create AWS ALB Ingress Controller requests without test requests")
		}
		if cfg.ALBIngressController.TestResponseSize > 0 {
			return errors.New("cannot create AWS ALB Ingress Controller requests without test response size")
		}
	}

	if cfg.ALBIngressController != nil && cfg.ALBIngressController.Enable {
		switch cfg.ALBIngressController.TestMode {
		case "ingress-test-server":
			if cfg.AWSK8sTesterImage == "" {
				return errors.New("'ingress-test-server' requires AWSK8sTesterImage")
			}
		case "nginx":
			if cfg.ALBIngressController.TestServerRoutes != 1 {
				return errors.New("'nginx' only needs 1 server route")
			}
		default:
			return fmt.Errorf("ALB Ingress test mode %q is not supported", cfg.ALBIngressController.TestMode)
		}

		if cfg.ALBIngressController.TargetType != "instance" &&
			cfg.ALBIngressController.TargetType != "ip" {
			return fmt.Errorf("ALB Ingress Controller target type not found %q", cfg.ALBIngressController.TargetType)
		}
		if cfg.ALBIngressController.IngressControllerImage == "" {
			return errors.New("ALB Ingress Controller image not specified")
		}
		cfg.ALBIngressController.ScalabilityOutputToUploadPath = fmt.Sprintf("%s.alb-ingress-controller.scalability.log", cfg.ConfigPath)
		cfg.ALBIngressController.MetricsOutputToUploadPath = fmt.Sprintf("%s.alb-ingress-controller.metrics.log", cfg.ConfigPath)

		if cfg.ALBIngressController.TestServerRoutes == 0 {
			return fmt.Errorf("cannot create AWS ALB Ingress Controller with empty test response size %d", cfg.ALBIngressController.TestServerRoutes)
		}
		if cfg.ALBIngressController.TestServerRoutes > maxTestServerRoutes {
			return fmt.Errorf("cannot create AWS ALB Ingress Controller with test routes %d (> max size %d)", cfg.ALBIngressController.TestServerRoutes, maxTestServerRoutes)
		}

		if cfg.ALBIngressController.TestClients == 0 {
			return fmt.Errorf("cannot create AWS ALB Ingress Controller with empty test response size %d", cfg.ALBIngressController.TestClients)
		}
		if cfg.ALBIngressController.TestClients > maxTestClients {
			return fmt.Errorf("cannot create AWS ALB Ingress Controller with test clients %d (> max size %d)", cfg.ALBIngressController.TestClients, maxTestClients)
		}

		if cfg.ALBIngressController.TestClientRequests == 0 {
			return fmt.Errorf("cannot create AWS ALB Ingress Controller with empty test response size %d", cfg.ALBIngressController.TestClientRequests)
		}
		if cfg.ALBIngressController.TestClientRequests > maxTestClientRequests {
			return fmt.Errorf("cannot create AWS ALB Ingress Controller with test requests %d (> max size %d)", cfg.ALBIngressController.TestClientRequests, maxTestClientRequests)
		}

		if cfg.ALBIngressController.TestResponseSize == 0 {
			return fmt.Errorf("cannot create AWS ALB Ingress Controller with empty test response size %d", cfg.ALBIngressController.TestResponseSize)
		}
		if cfg.ALBIngressController.TestResponseSize > maxTestResponseSize {
			return fmt.Errorf("cannot create AWS ALB Ingress Controller with test response size %d (> max size %d)", cfg.ALBIngressController.TestResponseSize, maxTestResponseSize)
		}
	}

	return cfg.Sync()
}

// SetClusterUpTook updates 'ClusterUpTook' field.
func (cfg *Config) SetClusterUpTook(d time.Duration) {
	cfg.ClusterState.upTook = d
	cfg.ClusterState.UpTook = d.String()
}

// SetIngressUpTook updates 'IngressUpTook' field.
func (cfg *Config) SetIngressUpTook(d time.Duration) {
	cfg.ALBIngressController.ingressUpTook = d
	cfg.ALBIngressController.IngressUpTook = d.String()
}

const (
	envPfx    = "AWS_K8S_TESTER_EKS_"
	envPfxALB = "AWS_K8S_TESTER_EKS_ALB_"
)

// UpdateFromEnvs updates fields from environmental variables.
func (cfg *Config) UpdateFromEnvs() error {
	cc := *cfg

	tp1, vv1 := reflect.TypeOf(&cc).Elem(), reflect.ValueOf(&cc).Elem()
	for i := 0; i < tp1.NumField(); i++ {
		jv := tp1.Field(i).Tag.Get("json")
		if jv == "" {
			continue
		}
		jv = strings.Replace(jv, ",omitempty", "", -1)
		jv = strings.Replace(jv, "-", "_", -1)
		jv = strings.ToUpper(strings.Replace(jv, "-", "_", -1))
		env := envPfx + jv
		if os.Getenv(env) == "" {
			continue
		}
		sv := os.Getenv(env)

		fieldName := tp1.Field(i).Name

		switch vv1.Field(i).Type().Kind() {
		case reflect.String:
			vv1.Field(i).SetString(sv)

		case reflect.Bool:
			bb, err := strconv.ParseBool(sv)
			if err != nil {
				return fmt.Errorf("failed to parse %q (%q, %v)", sv, env, err)
			}
			vv1.Field(i).SetBool(bb)

		case reflect.Int, reflect.Int32, reflect.Int64:
			if fieldName == "WaitBeforeDown" {
				dv, err := time.ParseDuration(sv)
				if err != nil {
					return fmt.Errorf("failed to parse %q (%q, %v)", sv, env, err)
				}
				vv1.Field(i).SetInt(int64(dv))
				continue
			}
			iv, err := strconv.ParseInt(sv, 10, 64)
			if err != nil {
				return fmt.Errorf("failed to parse %q (%q, %v)", sv, env, err)
			}
			vv1.Field(i).SetInt(iv)

		case reflect.Uint, reflect.Uint32, reflect.Uint64:
			iv, err := strconv.ParseUint(sv, 10, 64)
			if err != nil {
				return fmt.Errorf("failed to parse %q (%q, %v)", sv, env, err)
			}
			vv1.Field(i).SetUint(iv)

		case reflect.Float32, reflect.Float64:
			fv, err := strconv.ParseFloat(sv, 64)
			if err != nil {
				return fmt.Errorf("failed to parse %q (%q, %v)", sv, env, err)
			}
			vv1.Field(i).SetFloat(fv)

		case reflect.Slice:
			ss := strings.Split(sv, ",")
			slice := reflect.MakeSlice(reflect.TypeOf([]string{}), len(ss), len(ss))
			for i := range ss {
				slice.Index(i).SetString(ss[i])
			}
			vv1.Field(i).Set(slice)

		default:
			return fmt.Errorf("%q (%v) is not supported as an env", env, vv1.Field(i).Type())
		}
	}
	*cfg = cc

	av := *cc.ALBIngressController
	tp2, vv2 := reflect.TypeOf(&av).Elem(), reflect.ValueOf(&av).Elem()
	for i := 0; i < tp2.NumField(); i++ {
		jv := tp2.Field(i).Tag.Get("json")
		if jv == "" {
			continue
		}
		jv = strings.Replace(jv, ",omitempty", "", -1)
		jv = strings.ToUpper(strings.Replace(jv, "-", "_", -1))
		env := envPfxALB + jv
		if os.Getenv(env) == "" {
			continue
		}
		sv := os.Getenv(env)

		switch vv2.Field(i).Type().Kind() {
		case reflect.String:
			vv2.Field(i).SetString(sv)

		case reflect.Bool:
			bb, err := strconv.ParseBool(sv)
			if err != nil {
				return fmt.Errorf("failed to parse %q (%q, %v)", sv, env, err)
			}
			vv2.Field(i).SetBool(bb)

		case reflect.Int, reflect.Int32, reflect.Int64:
			iv, err := strconv.ParseInt(sv, 10, 64)
			if err != nil {
				return fmt.Errorf("failed to parse %q (%q, %v)", sv, env, err)
			}
			vv2.Field(i).SetInt(iv)

		case reflect.Uint, reflect.Uint32, reflect.Uint64:
			iv, err := strconv.ParseUint(sv, 10, 64)
			if err != nil {
				return fmt.Errorf("failed to parse %q (%q, %v)", sv, env, err)
			}
			vv2.Field(i).SetUint(iv)

		case reflect.Float32, reflect.Float64:
			fv, err := strconv.ParseFloat(sv, 64)
			if err != nil {
				return fmt.Errorf("failed to parse %q (%q, %v)", sv, env, err)
			}
			vv2.Field(i).SetFloat(fv)

		default:
			return fmt.Errorf("%q (%v) is not supported as an env", env, vv2.Field(i).Type())
		}
	}
	cfg.ALBIngressController = &av

	return nil
}

// supportedKubernetesVersions is a list of EKS supported Kubernets versions.
var supportedKubernetesVersions = map[string]struct{}{
	"1.10": {},
}

func checkKubernetesVersion(s string) (ok bool) {
	_, ok = supportedKubernetesVersions[s]
	return ok
}

// supportedRegions is a list of currently EKS supported AWS regions.
// See https://aws.amazon.com/about-aws/global-infrastructure/regional-product-services.
var supportedRegions = map[string]struct{}{
	"us-west-2": {},
	"us-east-1": {},
	"us-east-2": {},
	"eu-west-1": {},
}

func checkRegion(s string) (ok bool) {
	_, ok = supportedRegions[s]
	return ok
}

// https://docs.aws.amazon.com/eks/latest/userguide/getting-started.html
// https://docs.aws.amazon.com/eks/latest/userguide/eks-optimized-ami.html
var regionToAMICPU = map[string]string{
	"us-west-2": "ami-0f54a2f7d2e9c88b3",
	"us-east-1": "ami-0a0b913ef3249b655",
	"us-east-2": "ami-0958a76db2d150238",
	"eu-west-1": "ami-00c3b2d35bddd4f5c",
}

// https://docs.aws.amazon.com/eks/latest/userguide/getting-started.html
// https://docs.aws.amazon.com/eks/latest/userguide/eks-optimized-ami.html
var regionToAMIGPU = map[string]string{
	"us-west-2": "ami-08156e8fd65879a13",
	"us-east-1": "ami-0c974dde3f6d691a1",
	"us-east-2": "ami-089849e811ace242f",
	"eu-west-1": "ami-0c3479bcd739094f0",
}

func checkAMI(region, imageID string) (ok bool) {
	var id string
	id, ok = regionToAMICPU[region]
	if !ok {
		id, ok = regionToAMIGPU[region]
		if !ok {
			return false
		}
	}
	return id == imageID
}

func checkEC2InstanceType(s string) (ok bool) {
	_, ok = ec2.InstanceTypes[s]
	return ok
}

func checkMaxPods(s string, nodesN, serverReplicas int) (ok bool) {
	var v *ec2.InstanceType
	v, ok = ec2.InstanceTypes[s]
	if !ok {
		return false
	}
	maxPods := v.MaxPods * int64(nodesN)
	if int64(serverReplicas) > maxPods {
		return false
	}
	return true
}

const (
	defaultASGMin = 2
	defaultASGMax = 2
)

func checkWorkderNodeASG(min, max int) (ok bool) {
	if min == 0 || max == 0 {
		return false
	}
	if min > max {
		return false
	}
	return true
}

const (
	serviceRolePolicyARNService = "arn:aws:iam::aws:policy/AmazonEKSServicePolicy"
	serviceRolePolicyARNCluster = "arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"
)

func genServiceRoleWithPolicy(clusterName string) string {
	return fmt.Sprintf("%s-SERVICE-ROLE", clusterName)
}

func genCFStackVPC(clusterName string) string {
	return fmt.Sprintf("%s-VPC-STACK", clusterName)
}

func genNodeGroupKeyPairName(clusterName string) string {
	return fmt.Sprintf("%s-KEY-PAIR", clusterName)
}

func genCFStackWorkerNodeGroup(clusterName string) string {
	return fmt.Sprintf("%s-NODE-GROUP-STACK", clusterName)
}

var (
	// supportedEKSEps maps each test environments to EKS endpoint.
	supportedEKSEps = map[string]struct{}{
		// TODO: support EKS testing endpoint
		// https://docs.aws.amazon.com/sdk-for-go/v1/developer-guide/configuring-sdk.html#custom-endpoint
		// "https://test.us-west-2.amazonaws.com" : struct{}{},
	}

	allEKSEps = []string{}
)

func init() {
	allEKSEps = make([]string, 0, len(supportedEKSEps))
	for k := range supportedEKSEps {
		allEKSEps = append(allEKSEps, k)
	}
	sort.Strings(allEKSEps)
}

func checkEKSEp(s string) (ok bool) {
	if s == "" { // prod
		return true
	}
	_, ok = supportedEKSEps[s]
	return ok
}

// defaultWorkderNodeVolumeSizeGB is the default EKS worker node volume size in gigabytes.
// https://docs.aws.amazon.com/eks/latest/userguide/getting-started.html
const defaultWorkderNodeVolumeSizeGB = 20

func exist(name string) bool {
	_, err := os.Stat(name)
	return err == nil
}

const ll = "0123456789abcdefghijklmnopqrstuvwxyz"

func randString(n int) string {
	b := make([]byte, n)
	for i := range b {
		rand.Seed(time.Now().UTC().UnixNano())
		b[i] = ll[rand.Intn(len(ll))]
	}
	return string(b)
}
