/*
Copyright 2018 The Kubernetes Authors.

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
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/authorization/mgmt/2015-07-01/authorization"
	"github.com/Azure/azure-sdk-for-go/services/containerservice/mgmt/2019-10-01/containerservice"
	"github.com/Azure/azure-sdk-for-go/services/preview/msi/mgmt/2015-08-31-preview/msi"
	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2018-05-01/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/pelletier/go-toml"
	uuid "github.com/satori/go.uuid"
)

const (
	// aadOwnerRoleID is the role id that exists in every subscription for 'Owner'
	aadOwnerRoleID = "8e3af657-a8ff-443c-a75c-2fe8c4bcb635"
	// aadRoleReferenceTemplate is a template for a roleDefinitionId
	aadRoleReferenceTemplate = "/subscriptions/%s/providers/Microsoft.Authorization/roleDefinitions/%s"
	// aadRoleResourceGroupScopeTemplate is a template for a roleDefinition scope
	aadRoleResourceGroupScopeTemplate = "/subscriptions/%s"
)

type AKSEngineAPIModel struct {
	Location   string            `json:"location,omitempty"`
	Name       string            `json:"name,omitempty"`
	Tags       map[string]string `json:"tags,omitempty"`
	APIVersion string            `json:"apiVersion"`
	Properties *Properties       `json:"properties"`
}

type Properties struct {
	OrchestratorProfile     *OrchestratorProfile     `json:"orchestratorProfile,omitempty"`
	MasterProfile           *MasterProfile           `json:"masterProfile,omitempty"`
	AgentPoolProfiles       []*AgentPoolProfile      `json:"agentPoolProfiles,omitempty"`
	LinuxProfile            *LinuxProfile            `json:"linuxProfile,omitempty"`
	WindowsProfile          *WindowsProfile          `json:"windowsProfile,omitempty"`
	ServicePrincipalProfile *ServicePrincipalProfile `json:"servicePrincipalProfile,omitempty"`
	ExtensionProfiles       []map[string]string      `json:"extensionProfiles,omitempty"`
	CustomCloudProfile      *CustomCloudProfile      `json:"customCloudProfile,omitempty"`
	FeatureFlags            *FeatureFlags            `json:"featureFlags,omitempty"`
}

type ServicePrincipalProfile struct {
	ClientID string `json:"clientId,omitempty"`
	Secret   string `json:"secret,omitempty"`
}

type LinuxProfile struct {
	AdminUsername string `json:"adminUsername"`
	SSHKeys       *SSH   `json:"ssh"`
}

type SSH struct {
	PublicKeys []PublicKey `json:"publicKeys"`
}

type PublicKey struct {
	KeyData string `json:"keyData"`
}

type WindowsProfile struct {
	AdminUsername         string           `json:"adminUsername,omitempty"`
	AdminPassword         string           `json:"adminPassword,omitempty"`
	ImageVersion          string           `json:"imageVersion,omitempty"`
	WindowsImageSourceURL string           `json:"WindowsImageSourceUrl"`
	WindowsPublisher      string           `json:"WindowsPublisher"`
	WindowsOffer          string           `json:"WindowsOffer"`
	WindowsSku            string           `json:"WindowsSku"`
	WindowsDockerVersion  string           `json:"windowsDockerVersion"`
	SSHEnabled            bool             `json:"sshEnabled,omitempty"`
	EnableCSIProxy        bool             `json:"enableCSIProxy,omitempty"`
	CSIProxyURL           string           `json:"csiProxyURL,omitempty"`
	WindowsRuntimes       *WindowsRuntimes `json:"windowsRuntimes,omitempty"`
	WindowsPauseImageURL  string           `json:"windowsPauseImageURL,omitempty"`
}

// WindowsRuntimes configures containerd runtimes that are available on the windows nodes
type WindowsRuntimes struct {
	Default        string            `json:"default,omitempty"`
	HypervRuntimes []RuntimeHandlers `json:"hypervRuntimes,omitempty"`
}

// RuntimeHandlers configures the runtime settings in containerd
type RuntimeHandlers struct {
	BuildNumber string `json:"buildNumber,omitempty"`
}

// KubernetesContainerSpec defines configuration for a container spec
type KubernetesContainerSpec struct {
	Name           string `json:"name,omitempty"`
	Image          string `json:"image,omitempty"`
	CPURequests    string `json:"cpuRequests,omitempty"`
	MemoryRequests string `json:"memoryRequests,omitempty"`
	CPULimits      string `json:"cpuLimits,omitempty"`
	MemoryLimits   string `json:"memoryLimits,omitempty"`
}

// AddonNodePoolsConfig defines configuration for pool-specific cluster-autoscaler configuration
type AddonNodePoolsConfig struct {
	Name   string            `json:"name,omitempty"`
	Config map[string]string `json:"config,omitempty"`
}

// KubernetesAddon defines a list of addons w/ configuration to include with the cluster deployment
type KubernetesAddon struct {
	Name       string                    `json:"name,omitempty"`
	Enabled    *bool                     `json:"enabled,omitempty"`
	Mode       string                    `json:"mode,omitempty"`
	Containers []KubernetesContainerSpec `json:"containers,omitempty"`
	Config     map[string]string         `json:"config,omitempty"`
	Pools      []AddonNodePoolsConfig    `json:"pools,omitempty"`
	Data       string                    `json:"data,omitempty"`
}

type KubernetesConfig struct {
	ContainerRuntime                 string            `json:"containerRuntime,omitempty"`
	CustomWindowsPackageURL          string            `json:"customWindowsPackageURL,omitempty"`
	CustomHyperkubeImage             string            `json:"customHyperkubeImage,omitempty"`
	CustomCcmImage                   string            `json:"customCcmImage,omitempty"` // Image for cloud-controller-manager
	UseCloudControllerManager        *bool             `json:"useCloudControllerManager,omitempty"`
	NetworkPlugin                    string            `json:"networkPlugin,omitempty"`
	PrivateAzureRegistryServer       string            `json:"privateAzureRegistryServer,omitempty"`
	AzureCNIURLLinux                 string            `json:"azureCNIURLLinux,omitempty"`
	AzureCNIURLWindows               string            `json:"azureCNIURLWindows,omitempty"`
	Addons                           []KubernetesAddon `json:"addons,omitempty"`
	NetworkPolicy                    string            `json:"networkPolicy,omitempty"`
	CloudProviderRateLimitQPS        float64           `json:"cloudProviderRateLimitQPS,omitempty"`
	CloudProviderRateLimitBucket     int               `json:"cloudProviderRateLimitBucket,omitempty"`
	APIServerConfig                  map[string]string `json:"apiServerConfig,omitempty"`
	CloudControllerManagerConfig     map[string]string `json:"cloudControllerManagerConfig,omitempty"`
	KubernetesImageBase              string            `json:"kubernetesImageBase,omitempty"`
	ControllerManagerConfig          map[string]string `json:"controllerManagerConfig,omitempty"`
	KubeletConfig                    map[string]string `json:"kubeletConfig,omitempty"`
	SchedulerConfig                  map[string]string `json:"schedulerConfig,omitempty"`
	KubeProxyMode                    string            `json:"kubeProxyMode,omitempty"`
	LoadBalancerSku                  string            `json:"loadBalancerSku,omitempty"`
	ExcludeMasterFromStandardLB      *bool             `json:"excludeMasterFromStandardLB,omitempty"`
	ServiceCidr                      string            `json:"serviceCidr,omitempty"`
	DNSServiceIP                     string            `json:"dnsServiceIP,omitempty"`
	OutboundRuleIdleTimeoutInMinutes int32             `json:"outboundRuleIdleTimeoutInMinutes,omitempty"`
	ClusterSubnet                    string            `json:"clusterSubnet,omitempty"`
	CustomKubeAPIServerImage         string            `json:"customKubeAPIServerImage,omitempty"`
	CustomKubeControllerManagerImage string            `json:"customKubeControllerManagerImage,omitempty"`
	CustomKubeProxyImage             string            `json:"customKubeProxyImage,omitempty"`
	CustomKubeSchedulerImage         string            `json:"customKubeSchedulerImage,omitempty"`
	CustomKubeBinaryURL              string            `json:"customKubeBinaryURL,omitempty"`
	UseManagedIdentity               *bool             `json:"useManagedIdentity,omitempty"`
	UserAssignedID                   string            `json:"userAssignedID,omitempty"`
	WindowsContainerdURL             string            `json:"windowsContainerdURL,omitempty"`
	WindowsSdnPluginURL              string            `json:"windowsSdnPluginURL,omitempty"`
}

type OrchestratorProfile struct {
	OrchestratorType    string            `json:"orchestratorType"`
	OrchestratorRelease string            `json:"orchestratorRelease"`
	KubernetesConfig    *KubernetesConfig `json:"kubernetesConfig,omitempty"`
}

type MasterProfile struct {
	Count               int                 `json:"count"`
	Distro              string              `json:"distro"`
	DNSPrefix           string              `json:"dnsPrefix"`
	VMSize              string              `json:"vmSize" validate:"required"`
	IPAddressCount      int                 `json:"ipAddressCount,omitempty"`
	Extensions          []map[string]string `json:"extensions,omitempty"`
	OSDiskSizeGB        int                 `json:"osDiskSizeGB,omitempty" validate:"min=0,max=1023"`
	AvailabilityProfile string              `json:"availabilityProfile,omitempty"`
	AvailabilityZones   []string            `json:"availabilityZones,omitempty"`
	UltraSSDEnabled     bool                `json:"ultraSSDEnabled,omitempty"`
}

type AgentPoolProfile struct {
	Name                   string              `json:"name"`
	Count                  int                 `json:"count"`
	Distro                 string              `json:"distro"`
	VMSize                 string              `json:"vmSize"`
	OSType                 string              `json:"osType,omitempty"`
	AvailabilityProfile    string              `json:"availabilityProfile"`
	AvailabilityZones      []string            `json:"availabilityZones,omitempty"`
	IPAddressCount         int                 `json:"ipAddressCount,omitempty"`
	PreProvisionExtension  map[string]string   `json:"preProvisionExtension,omitempty"`
	Extensions             []map[string]string `json:"extensions,omitempty"`
	OSDiskSizeGB           int                 `json:"osDiskSizeGB,omitempty" validate:"min=0,max=1023"`
	EnableVMSSNodePublicIP bool                `json:"enableVMSSNodePublicIP,omitempty"`
	StorageProfile         string              `json:"storageProfile,omitempty"`
	UltraSSDEnabled        bool                `json:"ultraSSDEnabled,omitempty"`
}

type AzureClient struct {
	environment           azure.Environment
	subscriptionID        string
	deploymentsClient     resources.DeploymentsClient
	groupsClient          resources.GroupsClient
	msiClient             msi.UserAssignedIdentitiesClient
	authorizationClient   authorization.RoleAssignmentsClient
	managedClustersClient containerservice.ManagedClustersClient
}

type FeatureFlags struct {
	EnableIPv6DualStack bool `json:"enableIPv6DualStack,omitempty"`
	EnableIPv6Only      bool `json:"enableIPv6Only,omitempty"`
	EnableTelemetry     bool `json:"enableTelemetry,omitempty"`
}

// CustomCloudProfile defines configuration for custom cloud profile( for ex: Azure Stack)
type CustomCloudProfile struct {
	PortalURL string `json:"portalURL,omitempty"`
}

// AzureStackMetadataEndpoints defines configuration for Azure Stack
type AzureStackMetadataEndpoints struct {
	GalleryEndpoint string                            `json:"galleryEndpoint,omitempty"`
	GraphEndpoint   string                            `json:"graphEndpoint,omitempty"`
	PortalEndpoint  string                            `json:"portalEndpoint,omitempty"`
	Authentication  *AzureStackMetadataAuthentication `json:"authentication,omitempty"`
}

// AzureStackMetadataAuthentication defines configuration for Azure Stack
type AzureStackMetadataAuthentication struct {
	LoginEndpoint string   `json:"loginEndpoint,omitempty"`
	Audiences     []string `json:"audiences,omitempty"`
}

func (az *AzureClient) ValidateDeployment(ctx context.Context, resourceGroupName, deploymentName string, template, params *map[string]interface{}) (valid resources.DeploymentValidateResult, err error) {
	return az.deploymentsClient.Validate(ctx,
		resourceGroupName,
		deploymentName,
		resources.Deployment{
			Properties: &resources.DeploymentProperties{
				Template:   template,
				Parameters: params,
				Mode:       resources.Incremental,
			},
		})
}

func (az *AzureClient) DeployTemplate(ctx context.Context, resourceGroupName, deploymentName string, template, parameters *map[string]interface{}) (de resources.DeploymentExtended, err error) {
	future, err := az.deploymentsClient.CreateOrUpdate(
		ctx,
		resourceGroupName,
		deploymentName,
		resources.Deployment{
			Properties: &resources.DeploymentProperties{
				Template:   template,
				Parameters: parameters,
				Mode:       resources.Incremental,
			},
		})
	if err != nil {
		return de, fmt.Errorf("cannot create deployment: %v", err)
	}

	err = future.WaitForCompletionRef(ctx, az.deploymentsClient.Client)
	if err != nil {
		return de, fmt.Errorf("cannot get the create deployment future response: %v", err)
	}

	return future.Result(az.deploymentsClient)
}

func (az *AzureClient) EnsureResourceGroup(ctx context.Context, name, location string, managedBy *string) (resourceGroup *resources.Group, err error) {
	var tags map[string]*string
	group, err := az.groupsClient.Get(ctx, name)
	if err == nil && group.Tags != nil {
		tags = group.Tags
	} else {
		tags = make(map[string]*string)
	}
	// Tags for correlating resource groups with prow jobs on testgrid
	tags["buildID"] = stringPointer(os.Getenv("BUILD_ID"))
	tags["jobName"] = stringPointer(os.Getenv("JOB_NAME"))
	tags["creationTimestamp"] = stringPointer(time.Now().UTC().Format(time.RFC3339))

	response, err := az.groupsClient.CreateOrUpdate(ctx, name, resources.Group{
		Name:      &name,
		Location:  &location,
		ManagedBy: managedBy,
		Tags:      tags,
	})
	if err != nil {
		return &response, err
	}

	return &response, nil
}

func (az *AzureClient) DeleteResourceGroup(ctx context.Context, groupName string) error {
	_, err := az.groupsClient.Get(ctx, groupName)
	if err == nil {
		future, err := az.groupsClient.Delete(ctx, groupName)
		if err != nil {
			return fmt.Errorf("cannot delete resource group %v: %v", groupName, err)
		}
		err = future.WaitForCompletionRef(ctx, az.groupsClient.Client)
		if err != nil {
			// Skip the teardown errors because of https://github.com/Azure/go-autorest/issues/357
			// TODO(feiskyer): fix the issue by upgrading go-autorest version >= v11.3.2.
			log.Printf("Warning: failed to delete resource group %q with error %v", groupName, err)
		}
	}
	return nil
}

func (az *AzureClient) AssignOwnerRoleToIdentity(ctx context.Context, resourceGroupName, identityName string) error {
	identity, err := az.msiClient.Get(ctx, resourceGroupName, identityName)
	if err != nil {
		return fmt.Errorf("failed to get identity's client ID: %v", err)
	}

	identityPrincipalID := identity.PrincipalID.String()
	// Grant the identity 'Owner' access to the subscription
	// so it can pull images from private registry
	roleDefinitionID := fmt.Sprintf(aadRoleReferenceTemplate, az.subscriptionID, aadOwnerRoleID)
	scope := fmt.Sprintf(aadRoleResourceGroupScopeTemplate, az.subscriptionID)
	roleAssignmentParameters := authorization.RoleAssignmentCreateParameters{
		Properties: &authorization.RoleAssignmentProperties{
			RoleDefinitionID: stringPointer(roleDefinitionID),
			PrincipalID:      stringPointer(identityPrincipalID),
		},
	}

	if _, err := az.authorizationClient.Create(ctx, scope, uuid.NewV1().String(), roleAssignmentParameters); err != nil {
		return fmt.Errorf("failed to assign 'Owner' role to user assigned identity: %v", err)
	}

	return nil
}

func getOAuthConfig(env azure.Environment, subscriptionID, tenantID string) (*adal.OAuthConfig, error) {

	oauthConfig, err := adal.NewOAuthConfig(env.ActiveDirectoryEndpoint, tenantID)
	if err != nil {
		return nil, err
	}

	return oauthConfig, nil
}

func getAzCredentials() (*Creds, error) {
	content, err := ioutil.ReadFile(*aksCredentialsFile)
	log.Printf("Reading credentials file %v", *aksCredentialsFile)
	if err != nil {
		return nil, fmt.Errorf("error reading credentials file %v %v", *aksCredentialsFile, err)
	}
	config := Config{}
	err = toml.Unmarshal(content, &config)
	if err != nil {
		return nil, fmt.Errorf("error parsing credentials file %v %v", *aksCredentialsFile, err)
	}
	return &config.Creds, nil
}

func getAzureClient(env azure.Environment, subscriptionID, clientID, tenantID, clientSecret string) (*AzureClient, error) {
	oauthConfig, err := getOAuthConfig(env, subscriptionID, tenantID)
	if err != nil {
		return nil, err
	}

	armSpt, err := adal.NewServicePrincipalToken(*oauthConfig, clientID, clientSecret, env.ServiceManagementEndpoint)
	if err != nil {
		return nil, err
	}

	return getClient(env, subscriptionID, tenantID, armSpt), nil
}

func getClient(env azure.Environment, subscriptionID, tenantID string, armSpt *adal.ServicePrincipalToken) *AzureClient {
	c := &AzureClient{
		environment:           env,
		subscriptionID:        subscriptionID,
		deploymentsClient:     resources.NewDeploymentsClientWithBaseURI(env.ResourceManagerEndpoint, subscriptionID),
		groupsClient:          resources.NewGroupsClientWithBaseURI(env.ResourceManagerEndpoint, subscriptionID),
		msiClient:             msi.NewUserAssignedIdentitiesClient(subscriptionID),
		authorizationClient:   authorization.NewRoleAssignmentsClientWithBaseURI(env.ResourceManagerEndpoint, subscriptionID),
		managedClustersClient: containerservice.NewManagedClustersClientWithBaseURI(env.ResourceManagerEndpoint, subscriptionID),
	}

	authorizer := autorest.NewBearerAuthorizer(armSpt)
	c.deploymentsClient.Authorizer = authorizer
	c.deploymentsClient.PollingDuration = 60 * time.Minute
	c.groupsClient.Authorizer = authorizer
	c.msiClient.Authorizer = authorizer
	c.authorizationClient.Authorizer = authorizer
	c.managedClustersClient.Authorizer = authorizer

	return c
}

func downloadFromURL(url string, destination string, retry int) (string, error) {
	f, err := os.Create(destination)
	if err != nil {
		return "", err
	}
	defer f.Close()

	for i := 0; i < retry; i++ {
		log.Printf("downloading %v from %v", destination, url)
		if err := httpRead(url, f); err == nil {
			break
		}
		err = fmt.Errorf("url=%s failed get %v: %v", url, destination, err)
		if i == retry-1 {
			return "", err
		}
		log.Println(err)
		sleep(time.Duration(i) * time.Second)
	}
	f.Chmod(0644)
	return destination, nil
}

func populateAzureCloudConfig(isVMSS bool, credentials Creds, azureEnvironment, resourceGroup, location, outputDir string) error {
	// CLOUD_CONFIG is required when running Azure-specific e2e tests
	// See https://github.com/kubernetes/kubernetes/blob/master/hack/ginkgo-e2e.sh#L113-L118
	cc := map[string]string{
		"cloud":           azureEnvironment,
		"tenantId":        credentials.TenantID,
		"subscriptionId":  credentials.SubscriptionID,
		"aadClientId":     credentials.ClientID,
		"aadClientSecret": credentials.ClientSecret,
		"resourceGroup":   resourceGroup,
		"location":        location,
	}
	if isVMSS {
		cc["vmType"] = vmTypeVMSS
	} else {
		cc["vmType"] = vmTypeStandard
	}

	cloudConfig, err := json.MarshalIndent(cc, "", "    ")
	if err != nil {
		return fmt.Errorf("error creating Azure cloud config: %v", err)
	}

	cloudConfigPath := path.Join(outputDir, "azure.json")
	if err := ioutil.WriteFile(cloudConfigPath, cloudConfig, 0644); err != nil {
		return fmt.Errorf("cannot write Azure cloud config to file: %v", err)
	}
	if err := os.Setenv("CLOUD_CONFIG", cloudConfigPath); err != nil {
		return fmt.Errorf("error setting CLOUD_CONFIG=%s: %v", cloudConfigPath, err)
	}

	return nil
}

func stringPointer(s string) *string {
	return &s
}

func boolPointer(b bool) *bool {
	return &b
}

func toBool(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

func installAzureCLI() error {
	if err := control.FinishRunning(exec.Command("curl", "-sL", "https://packages.microsoft.com/keys/microsoft.asc", "-o", "msft.asc")); err != nil {
		return err
	}

	if err := control.FinishRunning(exec.Command("gpg", "--batch", "--yes", "-o", "/etc/apt/trusted.gpg.d/microsoft.asc.gpg", "--dearmor", "msft.asc")); err != nil {
		return err
	}

	if err := control.FinishRunning(exec.Command("bash", "-c", "echo \"deb [arch=amd64] https://packages.microsoft.com/repos/azure-cli $(lsb_release -cs) main\" | tee /etc/apt/sources.list.d/azure-cli.list")); err != nil {
		return err
	}

	if err := control.FinishRunning(exec.Command("apt-get", "update")); err != nil {
		return err
	}

	if err := control.FinishRunning(exec.Command("apt-get", "install", "-y", "azure-cli")); err != nil {
		return err
	}

	if err := os.Remove("msft.asc"); err != nil {
		return err
	}

	return nil
}
