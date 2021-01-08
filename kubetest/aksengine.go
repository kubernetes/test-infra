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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"k8s.io/test-infra/kubetest/e2e"
	"k8s.io/test-infra/kubetest/process"
	"k8s.io/test-infra/kubetest/util"

	"github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/Azure/go-autorest/autorest/azure"
	uuid "github.com/satori/go.uuid"
)

const (
	winZipTemplate                = "win-zip-%s.zip"
	k8sNodeTarballTemplate        = "kubernetes-node-linux-amd64-%s.tar.gz"
	azureBlobContainerURLTemplate = "https://%s.blob.core.windows.net/%s"
	azureKubemarkTestPrefix       = "[for azure kubemark test]"
)

var (
	aksResourceName          = flag.String("aksengine-resource-name", "", "Azure Resource Name")
	aksResourceGroupName     = flag.String("aksengine-resourcegroup-name", "", "Azure Resource Group Name")
	aksLocation              = flag.String("aksengine-location", "", "Azure AKS location")
	aksMasterVMSize          = flag.String("aksengine-mastervmsize", "", "Azure Master VM size")
	aksAgentVMSize           = flag.String("aksengine-agentvmsize", "", "Azure Agent VM size")
	aksAdminUsername         = flag.String("aksengine-admin-username", "", "Admin username")
	aksAdminPassword         = flag.String("aksengine-admin-password", "", "Admin password")
	aksAgentPoolCount        = flag.Int("aksengine-agentpoolcount", 0, "Azure Agent Pool Count")
	aksTemplateURL           = flag.String("aksengine-template-url", "", "Azure Template URL.")
	aksDNSPrefix             = flag.String("aksengine-dnsprefix", "", "Azure K8s Master DNS Prefix")
	aksEngineURL             = flag.String("aksengine-download-url", "", "Download URL for AKS engine")
	aksEngineMD5             = flag.String("aksengine-md5-sum", "", "Checksum for aks engine download")
	aksSSHPublicKeyPath      = flag.String("aksengine-public-key", "", "Path to SSH Public Key")
	aksSSHPrivateKeyPath     = flag.String("aksengine-private-key", "", "Path to SSH Private Key")
	aksWinBinaries           = flag.Bool("aksengine-win-binaries", false, "Set to True if you want kubetest to build a custom zip with windows binaries for aks-engine")
	aksCcm                   = flag.Bool("aksengine-ccm", false, "Set to True if you want kubetest to build a custom cloud controller manager for aks-engine")
	aksCnm                   = flag.Bool("aksengine-cnm", false, "Set to True if you want kubetest to build a custom cloud node manager for aks-engine. Require --aksengine-ccm to be true")
	aksCredentialsFile       = flag.String("aksengine-creds", "", "Path to credential file for Azure")
	aksOrchestratorRelease   = flag.String("aksengine-orchestratorRelease", "", "Orchestrator Profile for aks-engine")
	aksWinZipBuildScript     = flag.String("aksengine-winZipBuildScript", "https://raw.githubusercontent.com/Azure/aks-engine/master/scripts/build-windows-k8s.sh", "Build script to create custom zip containing win binaries for aks-engine")
	aksNetworkPlugin         = flag.String("aksengine-networkPlugin", "azure", "Network pluging to use with aks-engine")
	aksAzureEnv              = flag.String("aksengine-azure-env", "AzurePublicCloud", "The target Azure cloud")
	aksIdentitySystem        = flag.String("aksengine-identity-system", "azure_ad", "identity system (default:`azure_ad`, `adfs`)")
	aksCustomCloudURL        = flag.String("aksengine-custom-cloud-url", "", "management portal URL to use in custom Azure cloud (i.e Azure Stack etc)")
	aksDeployCustomK8s       = flag.Bool("aksengine-deploy-custom-k8s", false, "Set to True if you want to deploy custom-built k8s via aks-engine")
	aksCheckParams           = flag.Bool("aksengine-check-params", true, "Set to True if you want to validate your input parameters")
	aksDumpClusterLogs       = flag.Bool("aksengine-dump-cluster-logs", true, "Set to True if you want to dump cluster logs")
	aksNodeProblemDetector   = flag.Bool("aksengine-node-problem-detector", false, "Set to True if you want to enable node problem detector addon")
	runExternalE2EGinkgoTest = flag.Bool("run-external-e2e-ginkgo-test", false, "Set to True if you want external e2e ginkgo tests for the CSI driver")
	testCcm                  = flag.Bool("test-ccm", false, "Set to True if you want kubetest to run e2e tests for ccm")
	testAzureFileCSIDriver   = flag.Bool("test-azure-file-csi-driver", false, "Set to True if you want kubetest to run e2e tests for Azure File CSI driver")
	testAzureDiskCSIDriver   = flag.Bool("test-azure-disk-csi-driver", false, "Set to True if you want kubetest to run e2e tests for Azure Disk CSI driver")
	testBlobCSIDriver        = flag.Bool("test-blob-csi-driver", false, "Set to True if you want kubetest to run e2e tests for Azure Blob Storage CSI driver")
	testSecretStoreCSIDriver = flag.Bool("test-secrets-store-csi-driver", false, "Set to True if you want kubetest to run e2e tests for Secrets Store CSI driver")
	testSMBCSIDriver         = flag.Bool("test-csi-driver-smb", false, "Set to True if you want kubetest to run e2e tests for SMB CSI driver")
	testNFSCSIDriver         = flag.Bool("test-csi-driver-nfs", false, "Set to True if you want kubetest to run e2e tests for NFS CSI driver")
	// Commonly used variables
	k8sVersion                = getImageVersion(util.K8s("kubernetes"))
	cloudProviderAzureVersion = getImageVersion(util.K8sSigs("cloud-provider-azure"))
	imageRegistry             = os.Getenv("REGISTRY")
	k8sNodeTarballDir         = util.K8s("kubernetes", "_output", "release-tars") // contains custom-built kubelet and kubectl

	// kubemark scale tests
	buildWithKubemark          = flag.Bool("build-with-kubemark", false, fmt.Sprintf("%s Enable building clusters with kubemark", azureKubemarkTestPrefix))
	kubemarkBuildScriptURL     = flag.String("kubemark-build-script-url", "", fmt.Sprintf("%s URL to the building script of kubemark and kubemark-external cluster", azureKubemarkTestPrefix))
	kubemarkClusterTemplateURL = flag.String("kubemark-cluster-template-url", "", fmt.Sprintf("%s URL to the aks-engine template of kubemark cluster", azureKubemarkTestPrefix))
	externalClusterTemplateURL = flag.String("external-cluster-template-url", "", fmt.Sprintf("%s URL to the aks-engine template of kubemark external cluster", azureKubemarkTestPrefix))
	hollowNodesDeploymentURL   = flag.String("hollow-nodes-deployment-url", "", fmt.Sprintf("%s URL to the deployment configuration file of hollow nodes", azureKubemarkTestPrefix))
	clusterLoader2BinURL       = flag.String("clusterloader2-bin-url", "", fmt.Sprintf("%s URL to the binary of clusterloader2", azureKubemarkTestPrefix))
	kubemarkLocation           = flag.String("kubemark-location", "southcentralus", fmt.Sprintf("%s The location where the kubemark and external clusters run", azureKubemarkTestPrefix))
	kubemarkSize               = flag.String("kubemark-size", "100", fmt.Sprintf("%s The number of hollow nodes in kubemark cluster", azureKubemarkTestPrefix))
)

const (
	// AzureStackCloud is a const string reference identifier for Azure Stack cloud
	AzureStackCloud = "AzureStackCloud"
	// ADFSIdentitySystem is a const for ADFS identifier on Azure Stack cloud
	ADFSIdentitySystem = "adfs"
)

const (
	ccmImageName                   = "azure-cloud-controller-manager"
	cnmImageName                   = "azure-cloud-node-manager"
	cnmAddonName                   = "cloud-node-manager"
	nodeProblemDetectorAddonName   = "node-problem-detector"
	hyperkubeImageName             = "hyperkube-amd64"
	kubeAPIServerImageName         = "kube-apiserver-amd64"
	kubeControllerManagerImageName = "kube-controller-manager-amd64"
	kubeSchedulerImageName         = "kube-scheduler-amd64"
	kubeProxyImageName             = "kube-proxy-amd64"
)

const (
	vmTypeVMSS              = "vmss"
	vmTypeStandard          = "standard"
	availabilityProfileVMSS = "VirtualMachineScaleSets"
)

type aksDeploymentMethod int

const (
	// https://github.com/Azure/aks-engine/blob/master/docs/topics/kubernetes-developers.md#kubernetes-116-or-earlier
	customHyperkube aksDeploymentMethod = iota
	// https://github.com/Azure/aks-engine/blob/master/docs/topics/kubernetes-developers.md#kubernetes-117
	customK8sComponents
	noop
)

type Creds struct {
	ClientID           string
	ClientSecret       string
	TenantID           string
	SubscriptionID     string
	StorageAccountName string
	StorageAccountKey  string
}

type Config struct {
	Creds Creds
}

type aksEngineDeployer struct {
	ctx                              context.Context
	credentials                      *Creds
	location                         string
	resourceGroup                    string
	name                             string
	apiModelPath                     string
	dnsPrefix                        string
	templateJSON                     map[string]interface{}
	parametersJSON                   map[string]interface{}
	outputDir                        string
	sshPublicKey                     string
	sshPrivateKeyPath                string
	adminUsername                    string
	adminPassword                    string
	masterVMSize                     string
	agentVMSize                      string
	customHyperkubeImage             string
	aksCustomWinBinariesURL          string
	aksEngineBinaryPath              string
	customCcmImage                   string // custom cloud controller manager (ccm) image
	customCnmImage                   string // custom cloud node manager (cnm) image
	customKubeAPIServerImage         string
	customKubeControllerManagerImage string
	customKubeProxyImage             string
	customKubeSchedulerImage         string
	customKubeBinaryURL              string
	azureEnvironment                 string
	azureIdentitySystem              string
	azureCustomCloudURL              string
	agentPoolCount                   int
	k8sVersion                       string
	networkPlugin                    string
	azureClient                      *AzureClient
	aksDeploymentMethod              aksDeploymentMethod
	useManagedIdentity               bool
	identityName                     string
	azureBlobContainerURL            string
}

// IsAzureStackCloud return true if the cloud is AzureStack
func (c *aksEngineDeployer) isAzureStackCloud() bool {
	return c.azureCustomCloudURL != "" && strings.EqualFold(c.azureEnvironment, AzureStackCloud)
}

// SetCustomCloudProfileEnvironment retrieves the endpoints from Azure Stack metadata endpoint and sets the values for azure.Environment
func (c *aksEngineDeployer) SetCustomCloudProfileEnvironment() error {
	var environmentJSON string
	if c.isAzureStackCloud() {
		env := azure.Environment{}
		env.Name = c.azureEnvironment
		azsFQDNSuffix := strings.Replace(c.azureCustomCloudURL, fmt.Sprintf("https://portal.%s.", c.location), "", -1)
		azsFQDNSuffix = strings.TrimSuffix(azsFQDNSuffix, "/")
		env.ResourceManagerEndpoint = fmt.Sprintf("https://management.%s.%s/", c.location, azsFQDNSuffix)
		metadataURL := fmt.Sprintf("%s/metadata/endpoints?api-version=1.0", strings.TrimSuffix(env.ResourceManagerEndpoint, "/"))

		// Retrieve the metadata
		httpClient := &http.Client{
			Timeout: 30 * time.Second,
		}
		endpointsresp, err := httpClient.Get(metadataURL)
		if err != nil {
			return fmt.Errorf("%s . apimodel invalid: failed to retrieve Azure Stack endpoints from %s", err, metadataURL)
		}
		defer endpointsresp.Body.Close()
		if endpointsresp.StatusCode != 200 {
			return fmt.Errorf("%s . apimodel invalid: failed to retrieve Azure Stack endpoints from %s", err, metadataURL)
		}

		body, err := ioutil.ReadAll(endpointsresp.Body)
		if err != nil {
			return fmt.Errorf("%s . apimodel invalid: failed to read the response from %s", err, metadataURL)
		}

		endpoints := AzureStackMetadataEndpoints{}
		err = json.Unmarshal(body, &endpoints)
		if err != nil {
			return fmt.Errorf("%s . apimodel invalid: failed to parse the response from %s", err, metadataURL)
		}

		if endpoints.GraphEndpoint == "" || endpoints.Authentication == nil || endpoints.Authentication.LoginEndpoint == "" || len(endpoints.Authentication.Audiences) == 0 || endpoints.Authentication.Audiences[0] == "" {
			return fmt.Errorf("%s . apimodel invalid: invalid response from %s", err, metadataURL)
		}

		env.GraphEndpoint = endpoints.GraphEndpoint
		env.ServiceManagementEndpoint = endpoints.Authentication.Audiences[0]
		env.GalleryEndpoint = endpoints.GalleryEndpoint
		env.ActiveDirectoryEndpoint = endpoints.Authentication.LoginEndpoint
		if strings.EqualFold(c.azureIdentitySystem, ADFSIdentitySystem) {
			env.ActiveDirectoryEndpoint = strings.TrimSuffix(env.ActiveDirectoryEndpoint, "/")
			env.ActiveDirectoryEndpoint = strings.TrimSuffix(env.ActiveDirectoryEndpoint, ADFSIdentitySystem)
		}

		env.ManagementPortalURL = endpoints.PortalEndpoint
		env.ResourceManagerVMDNSSuffix = fmt.Sprintf("cloudapp.%s", azsFQDNSuffix)
		env.StorageEndpointSuffix = fmt.Sprintf("%s.%s", c.location, azsFQDNSuffix)
		env.KeyVaultDNSSuffix = fmt.Sprintf("vault.%s.%s", c.location, azsFQDNSuffix)

		bytes, err := json.Marshal(env)
		if err != nil {
			return fmt.Errorf("Could not serialize Environment object - %s", err.Error())
		}
		environmentJSON = string(bytes)

		// Create and update the file.
		tmpFile, err := ioutil.TempFile("", "azurestackcloud.json")
		tmpFileName := tmpFile.Name()
		if err != nil {
			return err
		}

		// Build content for the file
		if err = ioutil.WriteFile(tmpFileName, []byte(environmentJSON), os.ModeAppend); err != nil {
			return err
		}

		os.Setenv("AZURE_ENVIRONMENT_FILEPATH", tmpFileName)
	}
	return nil
}

func validateAzureStackCloudProfile() error {
	if *aksLocation == "" {
		return fmt.Errorf("no location specified for Azure Stack")
	}

	if *aksCustomCloudURL == "" {
		return fmt.Errorf("no custom cloud portal URL specified for Azure Stack")
	}

	if !strings.HasPrefix(*aksCustomCloudURL, fmt.Sprintf("https://portal.%s.", *aksLocation)) {
		return fmt.Errorf("custom cloud portal URL needs to start with https://portal.%s. ", *aksLocation)
	}
	return nil
}

func randomAKSEngineLocation() string {
	var AzureLocations = []string{
		"westeurope",
		"westus2",
		"eastus2",
		"southcentralus",
	}

	return AzureLocations[rand.Intn(len(AzureLocations))]
}

func checkParams() error {
	if !*aksCheckParams {
		log.Print("Skipping checkParams")
		return nil
	}

	// Validate flags
	if strings.EqualFold(*aksAzureEnv, AzureStackCloud) {
		if err := validateAzureStackCloudProfile(); err != nil {
			return err
		}
	} else if *aksLocation == "" {
		*aksLocation = randomAKSEngineLocation()
	}
	if *aksCredentialsFile == "" {
		return fmt.Errorf("no credentials file path specified")
	}
	if *aksResourceName == "" {
		*aksResourceName = "kubetest-" + uuid.NewV1().String()
	}
	if *aksResourceGroupName == "" {
		*aksResourceGroupName = *aksResourceName
	}
	if *aksDNSPrefix == "" {
		*aksDNSPrefix = *aksResourceName
	}
	if *aksSSHPublicKeyPath == "" {
		*aksSSHPublicKeyPath = os.Getenv("HOME") + "/.ssh/id_rsa.pub"
	}

	if *aksSSHPrivateKeyPath == "" {
		*aksSSHPrivateKeyPath = os.Getenv("HOME") + "/.ssh/id_rsa"
	}

	if !*buildWithKubemark && *aksTemplateURL == "" {
		return fmt.Errorf("no ApiModel URL specified, *buildWithKubemark=%v\n", *buildWithKubemark)
	}

	if *aksCnm && !*aksCcm {
		return fmt.Errorf("--aksengine-cnm cannot be true without --aksengine-ccm also being true")
	}

	return nil
}

func newAKSEngine() (*aksEngineDeployer, error) {
	if err := checkParams(); err != nil {
		return nil, fmt.Errorf("error creating Azure K8S cluster: %v", err)
	}

	sshKey, err := ioutil.ReadFile(*aksSSHPublicKeyPath)
	if err != nil {
		if os.IsNotExist(err) {
			sshKey = []byte{}
		} else {
			return nil, fmt.Errorf("error reading SSH Key %v %v", *aksSSHPublicKeyPath, err)
		}
	}

	outputDir, err := ioutil.TempDir(os.Getenv("HOME"), "tmp")
	if err != nil {
		return nil, fmt.Errorf("error creating tempdir: %v", err)
	}

	c := aksEngineDeployer{
		ctx:                              context.Background(),
		apiModelPath:                     *aksTemplateURL,
		name:                             *aksResourceName,
		dnsPrefix:                        *aksDNSPrefix,
		location:                         *aksLocation,
		resourceGroup:                    *aksResourceGroupName,
		outputDir:                        outputDir,
		sshPublicKey:                     fmt.Sprintf("%s", sshKey),
		sshPrivateKeyPath:                *aksSSHPrivateKeyPath,
		credentials:                      &Creds{},
		masterVMSize:                     *aksMasterVMSize,
		agentVMSize:                      *aksAgentVMSize,
		adminUsername:                    *aksAdminUsername,
		adminPassword:                    *aksAdminPassword,
		agentPoolCount:                   *aksAgentPoolCount,
		k8sVersion:                       *aksOrchestratorRelease,
		networkPlugin:                    *aksNetworkPlugin,
		azureEnvironment:                 *aksAzureEnv,
		azureIdentitySystem:              *aksIdentitySystem,
		azureCustomCloudURL:              *aksCustomCloudURL,
		customHyperkubeImage:             getDockerImage(hyperkubeImageName),
		customCcmImage:                   getDockerImage(ccmImageName),
		customCnmImage:                   getDockerImage(cnmImageName),
		customKubeAPIServerImage:         getDockerImage(kubeAPIServerImageName),
		customKubeControllerManagerImage: getDockerImage(kubeControllerManagerImageName),
		customKubeProxyImage:             getDockerImage(kubeProxyImageName),
		customKubeSchedulerImage:         getDockerImage(kubeSchedulerImageName),
		aksEngineBinaryPath:              "aks-engine", // use the one in path by default
		aksDeploymentMethod:              getAKSDeploymentMethod(*aksOrchestratorRelease),
		useManagedIdentity:               false,
		identityName:                     "",
	}
	creds, err := getAzCredentials()
	if err != nil {
		return nil, fmt.Errorf("failed to get azure credentials: %v", err)
	}
	c.credentials = creds
	c.azureBlobContainerURL = fmt.Sprintf(azureBlobContainerURLTemplate, c.credentials.StorageAccountName, os.Getenv("AZ_STORAGE_CONTAINER_NAME"))
	c.customKubeBinaryURL = fmt.Sprintf("%s/%s", c.azureBlobContainerURL, fmt.Sprintf(k8sNodeTarballTemplate, k8sVersion))
	c.aksCustomWinBinariesURL = fmt.Sprintf("%s/%s", c.azureBlobContainerURL, fmt.Sprintf(winZipTemplate, k8sVersion))

	err = c.SetCustomCloudProfileEnvironment()
	if err != nil {
		return nil, fmt.Errorf("failed to create custom cloud profile file: %v", err)
	}
	err = c.getAzureClient(c.ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ARM client: %v", err)
	}
	// like kops and gke set KUBERNETES_CONFORMANCE_TEST so the auth is picked up
	// from kubectl instead of bash inference.
	if err := os.Setenv("KUBERNETES_CONFORMANCE_TEST", "yes"); err != nil {
		return nil, err
	}

	if err := c.dockerLogin(); err != nil {
		return nil, err
	}

	return &c, nil
}

func getAKSDeploymentMethod(k8sRelease string) aksDeploymentMethod {
	if !*aksDeployCustomK8s {
		return noop
	}

	// k8sRelease should be in the format of X.XX
	s := strings.Split(k8sRelease, ".")
	if len(s) != 2 {
		return noop
	}

	minor, err := strconv.Atoi(s[1])
	if err != nil {
		return noop
	}

	// Deploy custom-built individual k8s components because
	// there is no hyperkube support in aks-engine for 1.17+
	if minor >= 17 {
		return customK8sComponents
	}
	return customHyperkube
}

func (c *aksEngineDeployer) populateAPIModelTemplate() error {
	var err error
	v := AKSEngineAPIModel{}
	if c.apiModelPath != "" {
		// template already exists, read it
		template, err := ioutil.ReadFile(path.Join(c.outputDir, "kubernetes.json"))
		if err != nil {
			return fmt.Errorf("error reading ApiModel template file: %v.", err)
		}
		dec := json.NewDecoder(bytes.NewReader(template))
		// Enforce strict JSON
		dec.DisallowUnknownFields()
		if err := dec.Decode(&v); err != nil {
			return fmt.Errorf("error unmarshaling ApiModel template file: %v", err)
		}
	} else {
		return fmt.Errorf("No template file specified %v", err)
	}

	// replace APIModel template properties from flags
	if c.location != "" {
		v.Location = c.location
	}
	if c.name != "" {
		v.Name = c.name
	}
	if v.Properties.OrchestratorProfile == nil {
		v.Properties.OrchestratorProfile = &OrchestratorProfile{}
	}
	if c.k8sVersion != "" {
		v.Properties.OrchestratorProfile.OrchestratorRelease = c.k8sVersion
	}
	if v.Properties.OrchestratorProfile.KubernetesConfig == nil {
		v.Properties.OrchestratorProfile.KubernetesConfig = &KubernetesConfig{}
	}
	// to support aks-engine validation logic `networkPolicy 'none' is not supported with networkPlugin 'azure'`
	if v.Properties.OrchestratorProfile.KubernetesConfig.NetworkPolicy != "none" && v.Properties.OrchestratorProfile.KubernetesConfig.NetworkPlugin == "" {
		// default NetworkPlugin to Azure if not provided
		v.Properties.OrchestratorProfile.KubernetesConfig.NetworkPlugin = c.networkPlugin
	}
	if c.dnsPrefix != "" {
		v.Properties.MasterProfile.DNSPrefix = c.dnsPrefix
	}
	if c.masterVMSize != "" {
		v.Properties.MasterProfile.VMSize = c.masterVMSize
	}
	if c.agentVMSize != "" {
		for _, agentPool := range v.Properties.AgentPoolProfiles {
			agentPool.VMSize = c.agentVMSize
		}
	}
	if c.agentPoolCount != 0 {
		for _, agentPool := range v.Properties.AgentPoolProfiles {
			agentPool.Count = c.agentPoolCount
		}
	}
	if c.adminUsername != "" {
		v.Properties.LinuxProfile.AdminUsername = c.adminUsername
		if v.Properties.WindowsProfile != nil {
			v.Properties.WindowsProfile.AdminUsername = c.adminUsername
		}
	}
	if c.adminPassword != "" {
		if v.Properties.WindowsProfile != nil {
			v.Properties.WindowsProfile.AdminPassword = c.adminPassword
		}
	}
	v.Properties.LinuxProfile.SSHKeys.PublicKeys = []PublicKey{{
		KeyData: c.sshPublicKey,
	}}

	if !toBool(v.Properties.OrchestratorProfile.KubernetesConfig.UseManagedIdentity) {
		// prevent the nil pointer panic
		v.Properties.ServicePrincipalProfile = &ServicePrincipalProfile{
			ClientID: c.credentials.ClientID,
			Secret:   c.credentials.ClientSecret,
		}
	} else {
		c.useManagedIdentity = true
		if v.Properties.OrchestratorProfile.KubernetesConfig.UserAssignedID != "" {
			c.identityName = v.Properties.OrchestratorProfile.KubernetesConfig.UserAssignedID
		} else {
			c.identityName = c.resourceGroup + "-id"
			v.Properties.OrchestratorProfile.KubernetesConfig.UserAssignedID = c.identityName
		}
	}

	if *aksWinBinaries {
		v.Properties.OrchestratorProfile.KubernetesConfig.CustomWindowsPackageURL = c.aksCustomWinBinariesURL
	}
	if *aksCcm {
		useCloudControllerManager := true
		v.Properties.OrchestratorProfile.KubernetesConfig.UseCloudControllerManager = &useCloudControllerManager
		v.Properties.OrchestratorProfile.KubernetesConfig.CustomCcmImage = c.customCcmImage
	}
	if *aksCnm {
		cnmAddon := KubernetesAddon{
			Name:    cnmAddonName,
			Enabled: boolPointer(true),
			Containers: []KubernetesContainerSpec{
				{
					Name:  cnmAddonName,
					Image: c.customCnmImage,
				},
			},
		}
		appendAddonToAPIModel(&v, cnmAddon)
	}
	if *aksNodeProblemDetector {
		nodeProblemDetectorAddon := KubernetesAddon{
			Name:    nodeProblemDetectorAddonName,
			Enabled: boolPointer(true),
		}
		appendAddonToAPIModel(&v, nodeProblemDetectorAddon)
	}

	// Populate PrivateAzureRegistryServer field if we are using ACR and custom-built k8s components
	if strings.Contains(imageRegistry, "azurecr") && c.aksDeploymentMethod != noop {
		v.Properties.OrchestratorProfile.KubernetesConfig.PrivateAzureRegistryServer = imageRegistry
	}

	switch c.aksDeploymentMethod {
	case customHyperkube:
		v.Properties.OrchestratorProfile.KubernetesConfig.CustomKubeAPIServerImage = ""
		v.Properties.OrchestratorProfile.KubernetesConfig.CustomKubeControllerManagerImage = ""
		v.Properties.OrchestratorProfile.KubernetesConfig.CustomKubeProxyImage = ""
		v.Properties.OrchestratorProfile.KubernetesConfig.CustomKubeSchedulerImage = ""
		v.Properties.OrchestratorProfile.KubernetesConfig.CustomKubeBinaryURL = ""
		v.Properties.OrchestratorProfile.KubernetesConfig.CustomHyperkubeImage = c.customHyperkubeImage
	case customK8sComponents:
		v.Properties.OrchestratorProfile.KubernetesConfig.CustomKubeAPIServerImage = c.customKubeAPIServerImage
		v.Properties.OrchestratorProfile.KubernetesConfig.CustomKubeControllerManagerImage = c.customKubeControllerManagerImage
		v.Properties.OrchestratorProfile.KubernetesConfig.CustomKubeProxyImage = c.customKubeProxyImage
		v.Properties.OrchestratorProfile.KubernetesConfig.CustomKubeSchedulerImage = c.customKubeSchedulerImage
		v.Properties.OrchestratorProfile.KubernetesConfig.CustomKubeBinaryURL = c.customKubeBinaryURL
		v.Properties.OrchestratorProfile.KubernetesConfig.CustomHyperkubeImage = ""
	}

	if c.isAzureStackCloud() {
		v.Properties.CustomCloudProfile.PortalURL = c.azureCustomCloudURL
	}

	if len(v.Properties.AgentPoolProfiles) > 0 {
		// Default to VirtualMachineScaleSets if AvailabilityProfile is empty
		isVMSS := v.Properties.AgentPoolProfiles[0].AvailabilityProfile == "" || v.Properties.AgentPoolProfiles[0].AvailabilityProfile == availabilityProfileVMSS
		if err := populateAzureCloudConfig(isVMSS, *c.credentials, c.azureEnvironment, c.resourceGroup, c.location, c.outputDir); err != nil {
			return err
		}
	}

	apiModel, _ := json.MarshalIndent(v, "", "    ")
	c.apiModelPath = path.Join(c.outputDir, "kubernetes.json")
	err = ioutil.WriteFile(c.apiModelPath, apiModel, 0644)
	if err != nil {
		return fmt.Errorf("cannot write apimodel to file: %v", err)
	}
	return nil
}

func appendAddonToAPIModel(v *AKSEngineAPIModel, addon KubernetesAddon) {
	// Update the addon if it already exists in the API model
	for i := range v.Properties.OrchestratorProfile.KubernetesConfig.Addons {
		a := &v.Properties.OrchestratorProfile.KubernetesConfig.Addons[i]
		if a.Name == addon.Name {
			a = &addon
			return
		}
	}

	v.Properties.OrchestratorProfile.KubernetesConfig.Addons = append(v.Properties.OrchestratorProfile.KubernetesConfig.Addons, addon)
}

func (c *aksEngineDeployer) getAKSEngine(retry int) error {
	downloadPath := path.Join(os.Getenv("HOME"), "aks-engine.tar.gz")
	f, err := os.Create(downloadPath)
	if err != nil {
		return err
	}
	defer f.Close()

	for i := 0; i < retry; i++ {
		log.Printf("downloading %v from %v.", downloadPath, *aksEngineURL)
		if err := httpRead(*aksEngineURL, f); err == nil {
			break
		}
		err = fmt.Errorf("url=%s failed get %v: %v.", *aksEngineURL, downloadPath, err)
		if i == retry-1 {
			return err
		}
		log.Println(err)
		sleep(time.Duration(i) * time.Second)
	}

	f.Close()
	if *aksEngineMD5 != "" {
		o, err := control.Output(exec.Command("md5sum", f.Name()))
		if err != nil {
			return err
		}
		if strings.Split(string(o), " ")[0] != *aksEngineMD5 {
			return fmt.Errorf("wrong md5 sum for aks-engine.")
		}
	}

	// adding aks-engine binary to the kubernetes dir modifies the tree
	// and dirties the tree. This makes it difficult to diff the CI signal
	// so moving the binary to /tmp folder
	wd := os.TempDir()
	log.Printf("Extracting tar file %v into directory %v .", f.Name(), wd)

	if err = control.FinishRunning(exec.Command("tar", "-xzf", f.Name(), "--strip", "1", "-C", wd)); err != nil {
		return err
	}
	c.aksEngineBinaryPath = path.Join(wd, "aks-engine")
	return nil

}

func (c *aksEngineDeployer) generateARMTemplates() error {
	cmd := exec.Command(c.aksEngineBinaryPath, "generate", c.apiModelPath, "--output-directory", c.outputDir)
	cmd.Dir = os.TempDir()
	if err := control.FinishRunning(cmd); err != nil {
		return fmt.Errorf("failed to generate ARM templates: %v.", err)
	}
	return nil
}

func (c *aksEngineDeployer) loadARMTemplates() error {
	var err error
	template, err := ioutil.ReadFile(path.Join(c.outputDir, "azuredeploy.json"))
	if err != nil {
		return fmt.Errorf("error reading ARM template file: %v.", err)
	}
	c.templateJSON = make(map[string]interface{})
	err = json.Unmarshal(template, &c.templateJSON)
	if err != nil {
		return fmt.Errorf("error unmarshall template %v", err.Error())
	}
	parameters, err := ioutil.ReadFile(path.Join(c.outputDir, "azuredeploy.parameters.json"))
	if err != nil {
		return fmt.Errorf("error reading ARM parameters file: %v", err)
	}
	c.parametersJSON = make(map[string]interface{})
	err = json.Unmarshal(parameters, &c.parametersJSON)
	if err != nil {
		return fmt.Errorf("error unmarshall parameters %v", err.Error())
	}
	c.parametersJSON = c.parametersJSON["parameters"].(map[string]interface{})

	return nil
}

func (c *aksEngineDeployer) getAzureClient(ctx context.Context) error {
	// instantiate Azure Resource Manager Client
	env, err := azure.EnvironmentFromName(c.azureEnvironment)
	if err != nil {
		return err
	}
	var client *AzureClient
	if c.isAzureStackCloud() && strings.EqualFold(c.azureIdentitySystem, ADFSIdentitySystem) {
		if client, err = getAzureClient(env,
			c.credentials.SubscriptionID,
			c.credentials.ClientID,
			c.azureIdentitySystem,
			c.credentials.ClientSecret); err != nil {
			return fmt.Errorf("error trying to get ADFS Azure Client: %v", err)
		}
	} else {
		if client, err = getAzureClient(env,
			c.credentials.SubscriptionID,
			c.credentials.ClientID,
			c.credentials.TenantID,
			c.credentials.ClientSecret); err != nil {
			return fmt.Errorf("error trying to get Azure Client: %v", err)
		}
	}
	c.azureClient = client
	return nil
}

func (c *aksEngineDeployer) createCluster() error {
	var err error
	kubecfgDir, _ := ioutil.ReadDir(path.Join(c.outputDir, "kubeconfig"))
	kubecfg := path.Join(c.outputDir, "kubeconfig", kubecfgDir[0].Name())
	log.Printf("Setting kubeconfig env variable: kubeconfig path: %v.", kubecfg)
	os.Setenv("KUBECONFIG", kubecfg)
	log.Printf("Creating resource group: %v.", c.resourceGroup)

	log.Printf("Creating Azure resource group: %v for cluster deployment.", c.resourceGroup)
	_, err = c.azureClient.EnsureResourceGroup(c.ctx, c.resourceGroup, c.location, nil)
	if err != nil {
		return fmt.Errorf("could not ensure resource group: %v", err)
	}

	log.Printf("Validating deployment ARM templates.")
	if _, err := c.azureClient.ValidateDeployment(
		c.ctx, c.resourceGroup, c.name, &c.templateJSON, &c.parametersJSON,
	); err != nil {
		return fmt.Errorf("ARM template invalid: %v", err)
	}

	log.Printf("Deploying cluster %v in resource group %v.", c.name, c.resourceGroup)
	if _, err := c.azureClient.DeployTemplate(
		c.ctx, c.resourceGroup, c.name, &c.templateJSON, &c.parametersJSON,
	); err != nil {
		return fmt.Errorf("cannot deploy: %v", err)
	}

	if c.useManagedIdentity && c.identityName != "" {
		log.Printf("Assigning 'Owner' role to %s in %s", c.identityName, c.resourceGroup)
		if err := c.azureClient.AssignOwnerRoleToIdentity(c.ctx, c.resourceGroup, c.identityName); err != nil {
			return err
		}
	}
	return nil
}

func (c *aksEngineDeployer) dockerLogin() error {
	cwd, _ := os.Getwd()
	log.Printf("CWD %v", cwd)
	username := ""
	pwd := ""
	server := ""
	var err error

	if !strings.Contains(imageRegistry, "azurecr.io") {
		// if REGISTRY is not ACR, then use docker cred
		log.Println("Attempting Docker login with docker cred.")
		username = os.Getenv("DOCKER_USERNAME")
		passwordFile := os.Getenv("DOCKER_PASSWORD_FILE")
		password, err := ioutil.ReadFile(passwordFile)
		if err != nil {
			return fmt.Errorf("error reading docker password file %v: %v", passwordFile, err)
		}
		pwd = strings.TrimSuffix(string(password), "\n")
	} else {
		// if REGISTRY is ACR, then use azure credential
		log.Println("Attempting Docker login with azure cred.")
		username = c.credentials.ClientID
		pwd = c.credentials.ClientSecret
		server = imageRegistry
	}
	cmd := exec.Command("docker", "login", fmt.Sprintf("--username=%s", username), fmt.Sprintf("--password=%s", pwd), server)
	if err = cmd.Run(); err != nil {
		return fmt.Errorf("failed Docker login with error: %v", err)
	}
	log.Println("Docker login success.")
	return nil
}

func dockerPush(images ...string) error {
	for _, image := range images {
		log.Printf("Pushing docker image %s", image)

		cmd := exec.Command("docker", "push", image)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to push %s: %v", image, err)
		}
	}

	return nil
}

func getDockerImage(imageName string) string {
	imageVersion := k8sVersion
	switch imageName {
	case ccmImageName, cnmImageName:
		imageVersion = cloudProviderAzureVersion
	}
	return fmt.Sprintf("%s/%s:%s", imageRegistry, imageName, imageVersion)
}

func areAllDockerImagesExist(images ...string) bool {
	for _, image := range images {
		cmd := exec.Command("docker", "pull", image)
		if err := cmd.Run(); err != nil {
			log.Printf("%s does not exist", image)
			return false
		}
		log.Printf("Reusing %s", image)
	}
	return true
}

func isURLExist(url string) bool {
	cmd := exec.Command("curl", "-s", "-o", "/dev/null", "-s", "-f", url)
	if err := cmd.Run(); err != nil {
		log.Printf("%s does not exist", url)
		return false
	}
	log.Printf("Reusing %s", url)
	return true
}

// getImageVersion returns the image version based on the project's latest git commit
func getImageVersion(projectPath string) string {
	cmd := exec.Command("git", "describe", "--tags", "--always", "--dirty")
	cmd.Dir = projectPath
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func (c *aksEngineDeployer) buildAzureCloudComponents() error {
	log.Println("Building cloud controller manager and cloud node manager.")

	// Set environment variables for building cloud components' images
	if err := os.Setenv("IMAGE_REGISTRY", imageRegistry); err != nil {
		return err
	}
	if err := os.Setenv("IMAGE_TAG", cloudProviderAzureVersion); err != nil {
		return err
	}

	cmd := exec.Command("make", "-C", util.K8sSigs("cloud-provider-azure"), "image", "push")
	cmd.Stdout = ioutil.Discard
	if err := control.FinishRunning(cmd); err != nil {
		return err
	}

	log.Printf("Custom cloud controller manager image: %s", c.customCcmImage)

	if *aksCnm {
		c.customCnmImage = getDockerImage(cnmImageName)
		log.Printf("Custom cloud node manager image: %s", c.customCnmImage)
	}

	return nil
}

func (c *aksEngineDeployer) buildHyperkube() error {
	var pushCmd *exec.Cmd
	os.Setenv("VERSION", k8sVersion)
	log.Println("Building hyperkube.")

	if _, err := os.Stat(util.K8s("kubernetes", "cmd", "hyperkube")); err == nil {
		// cmd/hyperkube binary still exists in repo
		cmd := exec.Command("make", "-C", util.K8s("kubernetes"), "WHAT=cmd/hyperkube")
		cmd.Stdout = ioutil.Discard
		if err := control.FinishRunning(cmd); err != nil {
			return err
		}
		hyperkubeBin := util.K8s("kubernetes", "_output", "bin", "hyperkube")
		pushCmd = exec.Command("make", "-C", util.K8s("kubernetes", "cluster", "images", "hyperkube"), "push", fmt.Sprintf("HYPERKUBE_BIN=%s", hyperkubeBin))
	} else if os.IsNotExist(err) {
		pushCmd = exec.Command("make", "-C", util.K8s("kubernetes", "cluster", "images", "hyperkube"), "push")
	}

	log.Println("Pushing hyperkube.")
	pushCmd.Stdout = ioutil.Discard
	if err := control.FinishRunning(pushCmd); err != nil {
		return err
	}

	log.Printf("Custom hyperkube image: %s", c.customHyperkubeImage)
	return nil
}

func (c *aksEngineDeployer) uploadToAzureStorage(filePath string) (string, error) {
	credential, err := azblob.NewSharedKeyCredential(c.credentials.StorageAccountName, c.credentials.StorageAccountKey)
	if err != nil {
		return "", fmt.Errorf("new shared key credential: %v", err)
	}
	p := azblob.NewPipeline(credential, azblob.PipelineOptions{})
	URL, _ := url.Parse(c.azureBlobContainerURL)

	containerURL := azblob.NewContainerURL(*URL, p)
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file %v . Error %v", filePath, err)
	}
	defer file.Close()

	blobURL := containerURL.NewBlockBlobURL(filepath.Base(file.Name()))
	blobURLString := blobURL.URL()
	if _, err = azblob.UploadFileToBlockBlob(context.Background(), file, blobURL, azblob.UploadToBlockBlobOptions{}); err != nil {
		// 'BlobHasBeenModified' conflict happens when two concurrent jobs are trying to upload files with the same name
		// Simply ignore the error and return the blob URL since at least one job will successfully upload the file to Azure storage
		if strings.Contains(err.Error(), "BlobHasBeenModified") {
			return blobURLString.String(), nil
		}
		return "", err
	}
	log.Printf("Uploaded %s to %s", filePath, blobURLString.String())
	return blobURLString.String(), nil
}

func getZipBuildScript(buildScriptURL string, retry int) (string, error) {
	downloadPath := path.Join(os.Getenv("HOME"), "build-win-zip.sh")
	f, err := os.Create(downloadPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	for i := 0; i < retry; i++ {
		log.Printf("downloading %v from %v.", downloadPath, buildScriptURL)
		if err := httpRead(buildScriptURL, f); err == nil {
			break
		}
		err = fmt.Errorf("url=%s failed get %v: %v.", buildScriptURL, downloadPath, err)
		if i == retry-1 {
			return "", err
		}
		log.Println(err)
		sleep(time.Duration(i) * time.Second)
	}
	f.Chmod(0744)
	return downloadPath, nil
}

func (c *aksEngineDeployer) buildWinZip() error {
	zipName := fmt.Sprintf(winZipTemplate, k8sVersion)
	buildFolder := path.Join(os.Getenv("HOME"), "winbuild")
	zipPath := path.Join(os.Getenv("HOME"), zipName)
	log.Printf("Building %s", zipName)
	buildScriptPath, err := getZipBuildScript(*aksWinZipBuildScript, 2)
	if err != nil {
		return err
	}
	// the build script for the windows binaries will produce a lot of output. Capture it here.
	cmd := exec.Command(buildScriptPath, "-u", zipName, "-z", buildFolder)
	cmd.Stdout = ioutil.Discard
	if err := control.FinishRunning(cmd); err != nil {
		return err
	}
	log.Printf("Uploading %s", zipPath)
	if c.aksCustomWinBinariesURL, err = c.uploadToAzureStorage(zipPath); err != nil {
		return err
	}
	return nil
}

func (c *aksEngineDeployer) Up() error {
	if *buildWithKubemark {
		if err := c.setCred(); err != nil {
			log.Printf("error during setting up azure credentials: %v", err)
			return err
		}

		cmd := exec.Command("curl", "-o", "build-kubemark.sh", *kubemarkBuildScriptURL)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to get build-kubemark.sh from %v: %v", *kubemarkBuildScriptURL, err)
		}

		cmd = exec.Command("bash", "build-kubemark.sh",
			"--kubemark-cluster-template-url", *kubemarkClusterTemplateURL,
			"--external-cluster-template-url", *externalClusterTemplateURL,
			"--hollow-nodes-deployment-url", *hollowNodesDeploymentURL,
			"--clusterloader2-bin-url", *clusterLoader2BinURL,
			"--kubemark-size", *kubemarkSize,
			"--location", *kubemarkLocation)

		if err := control.FinishRunning(cmd); err != nil {
			return fmt.Errorf("failed to build up kubemark environment: %v", err)
		}

		log.Println("kubemark test finished")
		return nil
	}

	var err error
	if c.apiModelPath != "" {
		templateFile, err := downloadFromURL(c.apiModelPath, path.Join(c.outputDir, "kubernetes.json"), 2)
		if err != nil {
			return fmt.Errorf("error downloading ApiModel template: %v with error %v", c.apiModelPath, err)
		}
		c.apiModelPath = templateFile
	}

	err = c.populateAPIModelTemplate()
	if err != nil {
		return fmt.Errorf("failed to populate aks-engine apimodel template: %v", err)
	}

	if *aksEngineURL != "" {
		err = c.getAKSEngine(2)
		if err != nil {
			return fmt.Errorf("failed to get AKS Engine binary: %v", err)
		}
	}
	err = c.generateARMTemplates()
	if err != nil {
		return fmt.Errorf("failed to generate ARM templates: %v", err)
	}
	err = c.loadARMTemplates()
	if err != nil {
		return fmt.Errorf("error loading ARM templates: %v", err)
	}
	err = c.createCluster()
	if err != nil {
		return fmt.Errorf("error creating cluster: %v", err)
	}

	return nil
}

func (c *aksEngineDeployer) Build(b buildStrategy) error {
	if c.aksDeploymentMethod == customHyperkube && !areAllDockerImagesExist(c.customHyperkubeImage) {
		// Build k8s without any special environment variables
		if err := b.Build(); err != nil {
			return err
		}
		if err := c.buildHyperkube(); err != nil {
			return fmt.Errorf("error building hyperkube %v", err)
		}
	} else if c.aksDeploymentMethod == customK8sComponents &&
		(!areAllDockerImagesExist(c.customKubeAPIServerImage,
			c.customKubeControllerManagerImage,
			c.customKubeProxyImage,
			c.customKubeSchedulerImage) || !isURLExist(c.customKubeBinaryURL)) {
		// Environment variables for creating custom k8s images
		if err := os.Setenv("KUBE_DOCKER_REGISTRY", imageRegistry); err != nil {
			return err
		}
		if err := os.Setenv("KUBE_DOCKER_IMAGE_TAG", k8sVersion); err != nil {
			return err
		}

		if err := b.Build(); err != nil {
			return err
		}

		if err := dockerPush(
			c.customKubeAPIServerImage,
			c.customKubeControllerManagerImage,
			c.customKubeProxyImage,
			c.customKubeSchedulerImage,
		); err != nil {
			return err
		}

		oldK8sNodeTarball := filepath.Join(k8sNodeTarballDir, "kubernetes-node-linux-amd64.tar.gz")
		if _, err := os.Stat(oldK8sNodeTarball); os.IsNotExist(err) {
			return fmt.Errorf("%s does not exist", oldK8sNodeTarball)
		}

		// Rename the tarball so that uploaded tarball won't get overwritten by other jobs
		newK8sNodeTarball := filepath.Join(k8sNodeTarballDir, fmt.Sprintf(k8sNodeTarballTemplate, k8sVersion))
		log.Printf("Renaming %s to %s", oldK8sNodeTarball, newK8sNodeTarball)
		if err := os.Rename(oldK8sNodeTarball, newK8sNodeTarball); err != nil {
			return fmt.Errorf("error renaming %s to %s: %v", oldK8sNodeTarball, newK8sNodeTarball, err)
		}

		var err error
		if c.customKubeBinaryURL, err = c.uploadToAzureStorage(newK8sNodeTarball); err != nil {
			return err
		}
	} else if (!*testCcm && !*testAzureDiskCSIDriver && !*testAzureFileCSIDriver && !*testBlobCSIDriver && !*testSecretStoreCSIDriver && !*testSMBCSIDriver && !*testNFSCSIDriver && !strings.EqualFold(string(b), "none")) || *runExternalE2EGinkgoTest {
		// Only build the required components to run upstream e2e tests
		for _, component := range []string{"WHAT='test/e2e/e2e.test'", "WHAT=cmd/kubectl", "ginkgo"} {
			cmd := exec.Command("make", component)
			cmd.Dir = util.K8s("kubernetes")
			err := control.FinishRunning(cmd)
			if err != nil {
				return err
			}
		}
	}

	if *aksCcm && !areAllDockerImagesExist(c.customCcmImage, c.customCnmImage) {
		if err := c.buildAzureCloudComponents(); err != nil {
			return fmt.Errorf("error building Azure cloud components: %v", err)
		}
	}
	if *aksWinBinaries && !isURLExist(c.aksCustomWinBinariesURL) {
		if err := c.buildWinZip(); err != nil {
			return fmt.Errorf("error building windowsZipFile %v", err)
		}
	}

	return nil
}

func (c *aksEngineDeployer) Down() error {
	log.Printf("Deleting resource group: %v.", c.resourceGroup)
	return c.azureClient.DeleteResourceGroup(c.ctx, c.resourceGroup)
}

func (c *aksEngineDeployer) DumpClusterLogs(localPath, gcsPath string) error {
	if !*aksDumpClusterLogs {
		log.Print("Skipping DumpClusterLogs")
		return nil
	}

	if err := os.Setenv("ARTIFACTS", localPath); err != nil {
		return err
	}

	logDumper := func() error {
		// Extract log dump script and manifest from cloud-provider-azure repo
		const logDumpURLPrefix string = "https://raw.githubusercontent.com/kubernetes-sigs/cloud-provider-azure/master/hack/log-dump/"
		logDumpScript, err := downloadFromURL(logDumpURLPrefix+"log-dump.sh", path.Join(c.outputDir, "log-dump.sh"), 2)
		if err != nil {
			return fmt.Errorf("error downloading log dump script: %v", err)
		}
		if err := control.FinishRunning(exec.Command("chmod", "+x", logDumpScript)); err != nil {
			return fmt.Errorf("error changing access permission for %s: %v", logDumpScript, err)
		}
		if _, err := downloadFromURL(logDumpURLPrefix+"log-dump-daemonset.yaml", path.Join(c.outputDir, "log-dump-daemonset.yaml"), 2); err != nil {
			return fmt.Errorf("error downloading log dump manifest: %v", err)
		}

		if err := control.FinishRunning(exec.Command("bash", "-c", logDumpScript)); err != nil {
			return fmt.Errorf("error running log collection script %s: %v", logDumpScript, err)
		}
		return nil
	}

	logDumperWindows := func() error {
		const winLogDumpScriptUrl string = "https://raw.githubusercontent.com/kubernetes-sigs/windows-testing/master/scripts/win-ci-logs-collector.sh"
		winLogDumpScript, err := downloadFromURL(winLogDumpScriptUrl, path.Join(c.outputDir, "win-ci-logs-collector.sh"), 2)

		masterFQDN := fmt.Sprintf("%s.%s.cloudapp.azure.com", c.dnsPrefix, c.location)
		if err != nil {
			return fmt.Errorf("error downloading windows logs dump script: %v", err)
		}
		if err := control.FinishRunning(exec.Command("chmod", "+x", winLogDumpScript)); err != nil {
			return fmt.Errorf("error changing permission for script %s: %v", winLogDumpScript, err)
		}
		if err := control.FinishRunning(exec.Command("bash", "-c", fmt.Sprintf("%s %s %s %s", winLogDumpScript, masterFQDN, c.outputDir, c.sshPrivateKeyPath))); err != nil {
			return fmt.Errorf("error while running Windows log collector script: %v", err)
		}
		return nil
	}

	var errors []string
	if err := logDumper(); err != nil {
		errors = append(errors, err.Error())
	}
	if err := logDumperWindows(); err != nil {
		// don't log error since logDumperWindows failed is expected on non-Windows cluster
		//errors = append(errors, err.Error())
	}
	if len(errors) != 0 {
		return fmt.Errorf(strings.Join(errors, "\n"))
	}
	return nil
}

func (c *aksEngineDeployer) GetClusterCreated(clusterName string) (time.Time, error) {
	return time.Time{}, errors.New("not implemented")
}

func (c *aksEngineDeployer) setCred() error {
	if err := os.Setenv("K8S_AZURE_TENANTID", c.credentials.TenantID); err != nil {
		return err
	}
	if err := os.Setenv("K8S_AZURE_SUBSID", c.credentials.SubscriptionID); err != nil {
		return err
	}
	if err := os.Setenv("K8S_AZURE_SPID", c.credentials.ClientID); err != nil {
		return err
	}
	if err := os.Setenv("K8S_AZURE_SPSEC", c.credentials.ClientSecret); err != nil {
		return err
	}
	if err := os.Setenv("K8S_AZURE_LOCATION", c.location); err != nil {
		return err
	}

	return nil
}

func (c *aksEngineDeployer) TestSetup() error {
	// set env vars required by the ccm e2e tests
	if *testCcm {
		if err := c.setCred(); err != nil {
			log.Printf("error during setting up azure credentials: %v", err)
			return err
		}
	} else if *testAzureFileCSIDriver || *testAzureDiskCSIDriver || *testBlobCSIDriver || *testSMBCSIDriver || *testNFSCSIDriver {
		// Set env vars required by CSI driver e2e jobs.
		// tenantId, subscriptionId, aadClientId, and aadClientSecret will be obtained from AZURE_CREDENTIAL
		if err := os.Setenv("AZURE_RESOURCE_GROUP", c.resourceGroup); err != nil {
			return err
		}
		if err := os.Setenv("AZURE_LOCATION", c.location); err != nil {
			return err
		}
	}

	// Download repo-list that defines repositories for Windows test images.
	downloadUrl, ok := os.LookupEnv("KUBE_TEST_REPO_LIST_DOWNLOAD_LOCATION")
	if !ok {
		// Env value for downloadUrl is not set, nothing to do
		log.Printf("KUBE_TEST_REPO_LIST_DOWNLOAD_LOCATION not set. Using default test image repos.")
		return nil
	}

	downloadPath := path.Join(os.Getenv("HOME"), "repo-list")
	f, err := os.Create(downloadPath)
	if err != nil {
		return err
	}
	defer f.Close()

	log.Printf("downloading %v from %v.", downloadPath, downloadUrl)
	err = httpRead(downloadUrl, f)

	if err != nil {
		return fmt.Errorf("url=%s failed get %v: %v.", downloadUrl, downloadPath, err)
	}
	f.Chmod(0744)
	if err := os.Setenv("KUBE_TEST_REPO_LIST", downloadPath); err != nil {
		return err
	}
	return nil
}

func (c *aksEngineDeployer) IsUp() error {
	return isUp(c)
}

func (c *aksEngineDeployer) KubectlCommand() (*exec.Cmd, error) {
	return exec.Command("kubectl"), nil
}

// BuildTester returns a standard ginkgo-script tester or a custom one if testCcm is enabled
func (c *aksEngineDeployer) BuildTester(o *e2e.BuildTesterOptions) (e2e.Tester, error) {
	if *testCcm {
		return &GinkgoCCMTester{}, nil
	}

	var csiDriverName string
	if *testAzureDiskCSIDriver {
		csiDriverName = "azuredisk-csi-driver"
	} else if *testAzureFileCSIDriver {
		csiDriverName = "azurefile-csi-driver"
	} else if *testBlobCSIDriver {
		csiDriverName = "blob-csi-driver"
	} else if *testSecretStoreCSIDriver {
		csiDriverName = "secrets-store-csi-driver"
	} else if *testSMBCSIDriver {
		csiDriverName = "csi-driver-smb"
	} else if *testNFSCSIDriver {
		csiDriverName = "csi-driver-nfs"
	}
	if csiDriverName != "" {
		if *runExternalE2EGinkgoTest {
			t := e2e.NewGinkgoTester(o)
			if o.StorageTestDriverPath != "" {
				t.StorageTestDriver = filepath.Join(util.K8sSigs(csiDriverName), o.StorageTestDriverPath)
			}
			return t, nil
		} else {
			return &GinkgoCSIDriverTester{
				driverName: csiDriverName,
			}, nil
		}
	}

	// Run e2e tests from upstream k8s repo
	return &GinkgoScriptTester{}, nil
}

// GinkgoCCMTester implements Tester by running E2E tests for Azure CCM
type GinkgoCCMTester struct {
}

// Run executes custom ginkgo script
func (t *GinkgoCCMTester) Run(control *process.Control, testArgs []string) error {
	artifactsDir, ok := os.LookupEnv("ARTIFACTS")
	if !ok {
		artifactsDir = filepath.Join(os.Getenv("WORKSPACE"), "_artifacts")
	}
	log.Printf("artifactsDir %v", artifactsDir)
	// set CCM_JUNIT_REPORT_DIR for ccm e2e test to use the same dir
	if err := os.Setenv("CCM_JUNIT_REPORT_DIR", artifactsDir); err != nil {
		return err
	}
	cmd := exec.Command("make", "test-ccm-e2e")
	projectPath := util.K8sSigs("cloud-provider-azure")
	cmd.Dir = projectPath
	testErr := control.FinishRunning(cmd)
	return testErr
}

// GinkgoCSIDriverTester implements Tester by running E2E tests for Azure-related CSI drivers
type GinkgoCSIDriverTester struct {
	driverName string
}

// Run executes custom ginkgo script
func (t *GinkgoCSIDriverTester) Run(control *process.Control, testArgs []string) error {
	cmd := exec.Command("make", "e2e-test")
	cmd.Dir = util.K8sSigs(t.driverName)
	return control.FinishRunning(cmd)
}
