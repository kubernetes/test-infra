package eksconfig

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/awstester/pkg/awsapi/ec2"

	gyaml "github.com/ghodss/yaml"
	"k8s.io/client-go/util/homedir"
)

// Config defines EKS testing configuration.
type Config struct {
	// KubetestEmbeddedBinary is true to use "awstester eks test" binary to run kubetest.
	// TODO: If false, use AWS CLI.
	KubetestEmbeddedBinary bool `json:"kubetest-embedded-binary,omitempty"`

	// AWSTesterImage is the awstester container image.
	// Required for "awstester ingress server" for ALB Ingress Controller tests.
	AWSTesterImage string `json:"awstester-image,omitempty"`

	// WaitBeforeDown is the duration to sleep before cluster tear down.
	WaitBeforeDown time.Duration `json:"wait-before-down,omitempty"`
	// Down is true to automatically tear down cluster in "test".
	// Deployer implementation should not call "Down" inside "Up" method.
	// This is meant to be used as a flag for test.
	Down bool `json:"down"`

	// EnableNodeSSH is true to enable SSH access to worker nodes.
	EnableNodeSSH bool `json:"enable-node-ssh"`

	// Tag is the tag used for all cloudformation stacks.
	// Must be left empty, and let deployer auto-populate this field.
	Tag string `json:"tag,omitempty"` // read-only to user
	// ClusterName is the EKS cluster name.
	// Must be left empty, and let deployer auto-populate this field.
	ClusterName string `json:"cluster-name,omitempty"` // read-only to user
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

	// AWSAccountID is the AWS account ID.
	AWSAccountID string `json:"aws-account-id,omitempty"`
	// AWSCredentialToMountPath is the file path to AWS credential.
	// Required for AWS ALB Ingress Controller deployments and other AWS specific tests.
	// If not empty, deployer is expected to mount the file as a secret object "aws-cred-awstester",
	// to the path "/etc/aws-cred-awstester/aws-cred-awstester", under "kube-system" namespace.
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
	// WorkderNodeASGMin is the minimum number of nodes in worker node ASG.
	WorkderNodeASGMin int `json:"worker-node-asg-min,omitempty"`
	// WorkderNodeASGMax is the maximum number of nodes in worker node ASG.
	WorkderNodeASGMax int `json:"worker-node-asg-max,omitempty"`
	// WorkerNodeVolumeSizeGB is the maximum number of nodes in worker node ASG.
	// If empty, set default value.
	WorkerNodeVolumeSizeGB int `json:"worker-node-volume-size-gb,omitempty"`

	// KubernetesVersion is the version of Kubernetes cluster.
	// If empty, set default version.
	KubernetesVersion string `json:"kubernetes-version,omitempty"`

	// LogDebug is true to enable debug level logging.
	LogDebug bool `json:"log-debug"`
	// LogOutputs is a list of log outputs. Valid values are 'default', 'stderr', 'stdout', or file names.
	// Logs are appended to the existing file, if any.
	// Multiple values are accepted. If empty, it sets to 'default', which outputs to stderr.
	// See https://godoc.org/go.uber.org/zap#Open and https://godoc.org/go.uber.org/zap#Config for more details.
	LogOutputs []string `json:"log-outputs,omitempty"`
	// LogOutputToUploadPath is the awstester log file path to upload to cloud storage.
	// Must be left empty.
	// This will be overwritten by cluster name.
	LogOutputToUploadPath       string `json:"log-output-to-upload-path,omitempty"`
	LogOutputToUploadPathBucket string `json:"log-output-to-upload-path-bucket,omitempty"`
	LogOutputToUploadPathURL    string `json:"log-output-to-upload-path-url,omitempty"`
	// LogAutoUpload is true to auto-upload log files.
	LogAutoUpload bool `json:"log-auto-upload"`

	// LogAccess is true to enable AWS API access logs (e.g. ALB access logs).
	// Automatically uploaded to S3 bucket named by cluster name.
	// https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-access-logs.html
	// https://github.com/kubernetes-sigs/aws-alb-ingress-controller/blob/master/docs/ingress-resources.md
	LogAccess bool `json:"log-access"`

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

	// PlatformVersion is the platform version of EKS.
	PlatformVersion string `json:"platform-version,omitempty"` // read-only to user

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
	// CFStackVPCID is the VPC ID that VPC cloudformation stack created.
	CFStackVPCID string `json:"cf-stack-vpc-id,omitempty"`
	// CFStackVPCSubnetIDs is the subnet IDS that VPC cloudformation stack created.
	CFStackVPCSubnetIDs []string `json:"cf-stack-vpc-subnet-ids,omitempty"`
	// CFStackVPCSecurityGroupID is the security group ID that VPC cloudformation stack created.
	CFStackVPCSecurityGroupID string `json:"cf-stack-vpc-security-group-id,omitempty"`

	// Endpoint is the cluster endpoint of the EKS cluster, required for KUBECONFIG write.
	Endpoint string `json:"endpoint,omitempty"`
	// CA is the EKS cluster CA, required for KUBECONFIG write.
	CA string `json:"ca,omitempty"`

	// WorkerNodeGroupStatus is the status Kubernetes worker node group.
	// "READY" when they successfully join the EKS cluster as worker nodes.
	WorkerNodeGroupStatus string `json:"worker-node-group-status,omitempty"`
	// WorkerNodes is a list of worker nodes.
	WorkerNodes []Instance `json:"worker-nodes,omitempty"`

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

const (
	// MaxTestServerRoutes is the maximum number of routes.
	MaxTestServerRoutes = 30
	// MaxTestClients is the maximum number of clients.
	MaxTestClients = 1000
	// MaxTestClientRequests is the maximum number of requests.
	MaxTestClientRequests = 50000
	// MaxTestResponseSize is the maximum response size for ingress test server.
	MaxTestResponseSize = 500 * 1024 // 500 KB == 4000 Kbit
)

// ALBIngressController configures ingress controller for EKS.
type ALBIngressController struct {
	// Created is true if ALB had started its creation operation.
	Created bool `json:"created"`
	// Enable is true to create an ALB Ingress Controller with sample ingress deployment.
	// 'AWSCredentialToMountPath' must be provided to configure ALB Ingress Controller.
	Enable bool `json:"enable"`

	// ALBIngressControllerImage is the ALB Ingress Controller container image.
	ALBIngressControllerImage string `json:"alb-ingress-controller-image,omitempty"`

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
	// TestServerReplicas is the number of ingress test server pods to deploy.
	TestServerReplicas int `json:"test-server-replicas,omitempty"`
	// TestServerRoutes is the number of ALB Ingress Controller routes to test.
	// It will be auto-generated starting with '/ingress-test-0000000'.
	// Supports up to 30.
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
	// Thus, pass "CFStackVPCSecurityGroupID" and "SecurityGroupIDPortOpen" for Ingress object

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

// Load loads configuration from YAML.
// Useful when injecting shared configuration via ConfigMap.
//
// Example usage:
//
//  import "github.com/aws/awstester/eksconfig"
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
	if err = gyaml.Unmarshal(d, cfg); err != nil {
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
	d, err = gyaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(cfg.ConfigPath, d, 0600)
}

// BackupConfig stores the original awstester configuration
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

// ValidateAndSetDefaults returns an error for invalid configurations.
// And updates empty fields with default values.
// At the end, it writes populated YAML to awstester config path.
func (cfg *Config) ValidateAndSetDefaults() error {
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
		if !checkMaxPods(cfg.WorkerNodeInstanceType, cfg.WorkderNodeASGMax, cfg.ALBIngressController.TestServerReplicas) {
			return fmt.Errorf(
				"EKS WorkerNodeInstanceType %q only supports %d pods per node (ASG Max %d, allowed up to %d, test server replicas %d)",
				cfg.WorkerNodeInstanceType,
				ec2.InstanceTypes[cfg.WorkerNodeInstanceType].MaxPods,
				cfg.WorkderNodeASGMax,
				ec2.InstanceTypes[cfg.WorkerNodeInstanceType].MaxPods*int64(cfg.WorkderNodeASGMax),
				cfg.ALBIngressController.TestServerReplicas,
			)
		}
	}
	if cfg.WorkderNodeASGMin == 0 {
		return errors.New("EKS WorkderNodeASGMin is not specified")
	}
	if cfg.WorkderNodeASGMax == 0 {
		return errors.New("EKS WorkderNodeASGMax is not specified")
	}
	if !checkWorkderNodeASG(cfg.WorkderNodeASGMin, cfg.WorkderNodeASGMax) {
		return fmt.Errorf("EKS WorkderNodeASG %d and %d is not valid", cfg.WorkderNodeASGMin, cfg.WorkderNodeASGMax)
	}
	if cfg.WorkerNodeVolumeSizeGB == 0 {
		cfg.WorkerNodeVolumeSizeGB = defaultWorkderNodeVolumeSizeGB
	}
	if ok := checkEKSEp(cfg.AWSCustomEndpoint); !ok {
		return fmt.Errorf("AWSCustomEndpoint %q is not valid", cfg.AWSCustomEndpoint)
	}

	if cfg.Tag == "" {
		cfg.Tag = GenTag()
	}
	if cfg.ClusterName == "" {
		cfg.ClusterName = genClusterName()
	}

	// resources created from awstester always follow
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
	cfg.ConfigPathBucket = filepath.Join(cfg.ClusterName, "awstester-eks.config.yaml")
	cfg.ConfigPathURL = genS3URL(cfg.AWSRegion, cfg.Tag, cfg.ConfigPathBucket)

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
	cfg.LogOutputToUploadPathBucket = filepath.Join(cfg.ClusterName, "awstester-eks.log")
	cfg.LogOutputToUploadPathURL = genS3URL(cfg.AWSRegion, cfg.Tag, cfg.LogOutputToUploadPathBucket)

	cfg.KubeConfigPath = fmt.Sprintf(
		"%s.%s.kubeconfig.generated.yaml",
		cfg.ConfigPath,
		cfg.ClusterName,
	)
	cfg.KubeConfigPathBucket = filepath.Join(
		cfg.ClusterName,
		"kubeconfig",
	)
	cfg.KubeConfigPathURL = genS3URL(
		cfg.AWSRegion,
		cfg.Tag,
		cfg.KubeConfigPathBucket,
	)

	cfg.ALBIngressController.IngressTestServerDeploymentServiceSpecPath = fmt.Sprintf(
		"%s.%s.alb.ingress-test-server.yaml",
		cfg.ConfigPath,
		cfg.ClusterName,
	)
	cfg.ALBIngressController.IngressTestServerDeploymentServiceSpecPathBucket = filepath.Join(
		cfg.ClusterName,
		"alb.ingress-test-server.deployment.service.yaml",
	)
	cfg.ALBIngressController.IngressTestServerDeploymentServiceSpecPathURL = genS3URL(
		cfg.AWSRegion,
		cfg.Tag,
		cfg.ALBIngressController.IngressTestServerDeploymentServiceSpecPathBucket,
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
	cfg.ALBIngressController.IngressControllerSpecPathURL = genS3URL(
		cfg.AWSRegion,
		cfg.Tag,
		cfg.ALBIngressController.IngressControllerSpecPathBucket,
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
	cfg.ALBIngressController.IngressObjectSpecPathURL = genS3URL(
		cfg.AWSRegion,
		cfg.Tag,
		cfg.ALBIngressController.IngressObjectSpecPathBucket,
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
	cfg.ALBIngressController.ScalabilityOutputToUploadPathURL = genS3URL(
		cfg.AWSRegion,
		cfg.Tag,
		cfg.ALBIngressController.ScalabilityOutputToUploadPathBucket,
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
	cfg.ALBIngressController.MetricsOutputToUploadPathURL = genS3URL(
		cfg.AWSRegion,
		cfg.Tag,
		cfg.ALBIngressController.MetricsOutputToUploadPathBucket,
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
		if cfg.AWSTesterImage == "" {
			return errors.New("cannot create AWS ALB Ingress Controller without ingress test server image")
		}
		if cfg.ALBIngressController.ALBIngressControllerImage == "" {
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
		case "nginx":
		default:
			return fmt.Errorf("ALB Ingress test mode %q is not supported", cfg.ALBIngressController.TestMode)
		}

		if cfg.AWSTesterImage == "" {
			return errors.New("awstester image not specified")
		}
		if cfg.ALBIngressController.TargetType != "instance" &&
			cfg.ALBIngressController.TargetType != "ip" {
			return fmt.Errorf("ALB Ingress Controller target type not found %q", cfg.ALBIngressController.TargetType)
		}
		if cfg.ALBIngressController.ALBIngressControllerImage == "" {
			return errors.New("ALB Ingress Controller image not specified")
		}
		cfg.ALBIngressController.ScalabilityOutputToUploadPath = fmt.Sprintf("%s.alb-ingress-controller.scalability.log", cfg.ConfigPath)
		cfg.ALBIngressController.MetricsOutputToUploadPath = fmt.Sprintf("%s.alb-ingress-controller.metrics.log", cfg.ConfigPath)

		if cfg.ALBIngressController.TestServerRoutes == 0 {
			return fmt.Errorf("cannot create AWS ALB Ingress Controller with empty test response size %d", cfg.ALBIngressController.TestServerRoutes)
		}
		if cfg.ALBIngressController.TestServerRoutes > MaxTestServerRoutes {
			return fmt.Errorf("cannot create AWS ALB Ingress Controller with test routes %d (> max size %d)", cfg.ALBIngressController.TestServerRoutes, MaxTestServerRoutes)
		}

		if cfg.ALBIngressController.TestClients == 0 {
			return fmt.Errorf("cannot create AWS ALB Ingress Controller with empty test response size %d", cfg.ALBIngressController.TestClients)
		}
		if cfg.ALBIngressController.TestClients > MaxTestClients {
			return fmt.Errorf("cannot create AWS ALB Ingress Controller with test clients %d (> max size %d)", cfg.ALBIngressController.TestClients, MaxTestClients)
		}

		if cfg.ALBIngressController.TestClientRequests == 0 {
			return fmt.Errorf("cannot create AWS ALB Ingress Controller with empty test response size %d", cfg.ALBIngressController.TestClientRequests)
		}
		if cfg.ALBIngressController.TestClientRequests > MaxTestClientRequests {
			return fmt.Errorf("cannot create AWS ALB Ingress Controller with test requests %d (> max size %d)", cfg.ALBIngressController.TestClientRequests, MaxTestClientRequests)
		}

		if cfg.ALBIngressController.TestResponseSize == 0 {
			return fmt.Errorf("cannot create AWS ALB Ingress Controller with empty test response size %d", cfg.ALBIngressController.TestResponseSize)
		}
		if cfg.ALBIngressController.TestResponseSize > MaxTestResponseSize {
			return fmt.Errorf("cannot create AWS ALB Ingress Controller with test response size %d (> max size %d)", cfg.ALBIngressController.TestResponseSize, MaxTestResponseSize)
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
