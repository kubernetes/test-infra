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
	"strings"
	"time"

	"github.com/pelletier/go-toml"
	"k8s.io/test-infra/kubetest/e2e"
	"k8s.io/test-infra/kubetest/process"
	"k8s.io/test-infra/kubetest/util"

	"github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/Azure/go-autorest/autorest/azure"
	uuid "github.com/satori/go.uuid"
)

var (
	// azure specific flags
	acsResourceName        = flag.String("acsengine-resource-name", "", "Azure Resource Name")
	acsResourceGroupName   = flag.String("acsengine-resourcegroup-name", "", "Azure Resource Group Name")
	acsLocation            = flag.String("acsengine-location", "", "Azure ACS location")
	acsMasterVmSize        = flag.String("acsengine-mastervmsize", "", "Azure Master VM size")
	acsAgentVmSize         = flag.String("acsengine-agentvmsize", "", "Azure Agent VM size")
	acsAdminUsername       = flag.String("acsengine-admin-username", "", "Admin username")
	acsAdminPassword       = flag.String("acsengine-admin-password", "", "Admin password")
	acsAgentPoolCount      = flag.Int("acsengine-agentpoolcount", 0, "Azure Agent Pool Count")
	acsTemplateURL         = flag.String("acsengine-template-url", "", "Azure Template URL.")
	acsDnsPrefix           = flag.String("acsengine-dnsprefix", "", "Azure K8s Master DNS Prefix")
	acsEngineURL           = flag.String("acsengine-download-url", "", "Download URL for ACS engine")
	acsEngineMD5           = flag.String("acsengine-md5-sum", "", "Checksum for acs engine download")
	acsSSHPublicKeyPath    = flag.String("acsengine-public-key", "", "Path to SSH Public Key")
	acsWinBinaries         = flag.Bool("acsengine-win-binaries", false, "Set to True if you want kubetest to build a custom zip with windows binaries for aks-engine")
	acsHyperKube           = flag.Bool("acsengine-hyperkube", false, "Set to True if you want kubetest to build a custom hyperkube for aks-engine")
	acsCcm                 = flag.Bool("acsengine-ccm", false, "Set to True if you want kubetest to build a custom cloud controller manager for aks-engine")
	acsCredentialsFile     = flag.String("acsengine-creds", "", "Path to credential file for Azure")
	acsOrchestratorRelease = flag.String("acsengine-orchestratorRelease", "", "Orchestrator Profile for acs-engine")
	acsWinZipBuildScript   = flag.String("acsengine-winZipBuildScript", "https://raw.githubusercontent.com/Azure/acs-engine/master/scripts/build-windows-k8s.sh", "Build script to create custom zip containing win binaries for acs-engine")
	acsNetworkPlugin       = flag.String("acsengine-networkPlugin", "azure", "Network pluging to use with acs-engine")
	acsAzureEnv            = flag.String("acsengine-azure-env", "AzurePublicCloud", "The target Azure cloud")
	acsIdentitySystem      = flag.String("acsengine-identity-system", "azure_ad", "identity system (default:`azure_ad`, `adfs`)")
	acsCustomCloudURL      = flag.String("acsengine-custom-cloud-url", "", "management portal URL to use in custom Azure cloud (i.e Azure Stack etc)")
	testCcm                = flag.Bool("test-ccm", false, "Set to True if you want kubetest to run e2e tests for ccm")
)

const (
	// AzureStackCloud is a const string reference identifier for Azure Stack cloud
	AzureStackCloud = "AzureStackCloud"
	// ADFSIdentitySystem is a const for ADFS identifier on Azure Stack cloud
	ADFSIdentitySystem = "adfs"
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

type Cluster struct {
	ctx                     context.Context
	credentials             *Creds
	location                string
	resourceGroup           string
	name                    string
	apiModelPath            string
	dnsPrefix               string
	templateJSON            map[string]interface{}
	parametersJSON          map[string]interface{}
	outputDir               string
	sshPublicKey            string
	adminUsername           string
	adminPassword           string
	masterVMSize            string
	agentVMSize             string
	acsCustomHyperKubeURL   string
	acsCustomWinBinariesURL string
	acsEngineBinaryPath     string
	acsCustomCcmURL         string
	azureEnvironment        string
	azureIdentitySystem     string
	azureCustomCloudURL     string
	agentPoolCount          int
	k8sVersion              string
	networkPlugin           string
	azureClient             *AzureClient
}

// IsAzureStackCloud return true if the cloud is AzureStack
func (c *Cluster) isAzureStackCloud() bool {
	return c.azureCustomCloudURL != "" && strings.EqualFold(c.azureEnvironment, AzureStackCloud)
}

// SetCustomCloudProfileEnvironment retrieves the endpoints from Azure Stack metadata endpoint and sets the values for azure.Environment
func (c *Cluster) SetCustomCloudProfileEnvironment() error {
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
		if err != nil || endpointsresp.StatusCode != 200 {
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

func (c *Cluster) getAzCredentials() error {
	content, err := ioutil.ReadFile(*acsCredentialsFile)
	log.Printf("Reading credentials file %v", *acsCredentialsFile)
	if err != nil {
		return fmt.Errorf("error reading credentials file %v %v", *acsCredentialsFile, err)
	}
	config := Config{}
	err = toml.Unmarshal(content, &config)
	c.credentials = &config.Creds
	if err != nil {
		return fmt.Errorf("error parsing credentials file %v %v", *acsCredentialsFile, err)
	}
	return nil
}

func validateAzureStackCloudProfile() error {
	if *acsLocation == "" {
		return fmt.Errorf("no location specified for Azure Stack")
	}

	if *acsCustomCloudURL == "" {
		return fmt.Errorf("no custom cloud portal URL specified for Azure Stack")
	}

	if !strings.HasPrefix(*acsCustomCloudURL, fmt.Sprintf("https://portal.%s.", *acsLocation)) {
		return fmt.Errorf("custom cloud portal URL needs to start with https://portal.%s. ", *acsLocation)
	}
	return nil
}

func randomAcsEngineLocation() string {
	var AzureLocations = []string{
		"westeurope",
		"westus2",
		"eastus2",
		"southcentralus",
	}

	return AzureLocations[rand.Intn(len(AzureLocations))]
}

func checkParams() error {
	if strings.EqualFold(*acsAzureEnv, AzureStackCloud) {
		if err := validateAzureStackCloudProfile(); err != nil {
			return err
		}
	} else if *acsLocation == "" {
		*acsLocation = randomAcsEngineLocation()
	}
	if *acsCredentialsFile == "" {
		return fmt.Errorf("no credentials file path specified")
	}
	if *acsResourceName == "" {
		*acsResourceName = "kubetest-" + uuid.NewV1().String()
	}
	if *acsResourceGroupName == "" {
		*acsResourceGroupName = *acsResourceName
	}
	if *acsDnsPrefix == "" {
		*acsDnsPrefix = *acsResourceName
	}
	if *acsSSHPublicKeyPath == "" {
		*acsSSHPublicKeyPath = os.Getenv("HOME") + "/.ssh/id_rsa.pub"
	}
	if *acsTemplateURL == "" {
		return fmt.Errorf("no ApiModel URL specified.")
	}
	return nil
}

func newAcsEngine() (*Cluster, error) {
	if err := checkParams(); err != nil {
		return nil, fmt.Errorf("error creating Azure K8S cluster: %v", err)
	}

	tempdir, _ := ioutil.TempDir(os.Getenv("HOME"), "acs")
	sshKey, err := ioutil.ReadFile(*acsSSHPublicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("error reading SSH Key %v %v", *acsSSHPublicKeyPath, err)
	}
	c := Cluster{
		ctx:                     context.Background(),
		apiModelPath:            *acsTemplateURL,
		name:                    *acsResourceName,
		dnsPrefix:               *acsDnsPrefix,
		location:                *acsLocation,
		resourceGroup:           *acsResourceGroupName,
		outputDir:               tempdir,
		sshPublicKey:            fmt.Sprintf("%s", sshKey),
		credentials:             &Creds{},
		masterVMSize:            *acsMasterVmSize,
		agentVMSize:             *acsAgentVmSize,
		adminUsername:           *acsAdminUsername,
		adminPassword:           *acsAdminPassword,
		agentPoolCount:          *acsAgentPoolCount,
		k8sVersion:              *acsOrchestratorRelease,
		networkPlugin:           *acsNetworkPlugin,
		azureEnvironment:        *acsAzureEnv,
		azureIdentitySystem:     *acsIdentitySystem,
		azureCustomCloudURL:     *acsCustomCloudURL,
		acsCustomHyperKubeURL:   "",
		acsCustomWinBinariesURL: "",
		acsCustomCcmURL:         "",
		acsEngineBinaryPath:     "aks-engine", // use the one in path by default
	}
	c.getAzCredentials()
	err = c.SetCustomCloudProfileEnvironment()
	if err != nil {
		return nil, fmt.Errorf("failed to create custom cloud profile file: %v", err)
	}
	err = c.getARMClient(c.ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ARM client: %v", err)
	}
	// like kops and gke set KUBERNETES_CONFORMANCE_TEST so the auth is picked up
	// from kubectl instead of bash inference.
	if err := os.Setenv("KUBERNETES_CONFORMANCE_TEST", "yes"); err != nil {
		return nil, err
	}

	return &c, nil
}

func (c *Cluster) populateApiModelTemplate() error {
	var err error
	v := AcsEngineAPIModel{}
	if c.apiModelPath != "" {
		// template already exists, read it
		template, err := ioutil.ReadFile(path.Join(c.outputDir, "kubernetes.json"))
		if err != nil {
			return fmt.Errorf("error reading ApiModel template file: %v.", err)
		}
		err = json.Unmarshal(template, &v)
		if err != nil {
			return fmt.Errorf("error unmarshaling ApiModel template file: %v", err)
		}
	} else {
		return fmt.Errorf("No template file specified %v", err)
	}

	// set default distro so we do not use prebuilt os image
	if v.Properties.MasterProfile.Distro == "" {
		v.Properties.MasterProfile.Distro = "ubuntu"
	}
	for _, agentPool := range v.Properties.AgentPoolProfiles {
		if agentPool.Distro == "" {
			agentPool.Distro = "ubuntu"
		}
	}
	// replace APIModel template properties from flags
	if c.location != "" {
		v.Location = c.location
	}
	if c.name != "" {
		v.Name = c.name
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
	v.Properties.ServicePrincipalProfile.ClientID = c.credentials.ClientID
	v.Properties.ServicePrincipalProfile.Secret = c.credentials.ClientSecret

	if c.acsCustomHyperKubeURL != "" {
		v.Properties.OrchestratorProfile.KubernetesConfig.CustomHyperkubeImage = c.acsCustomHyperKubeURL
		if strings.Contains(os.Getenv("REGISTRY"), "azurecr") {
			v.Properties.OrchestratorProfile.KubernetesConfig.PrivateAzureRegistryServer = os.Getenv("REGISTRY")
		}
	}
	if c.acsCustomWinBinariesURL != "" {
		v.Properties.OrchestratorProfile.KubernetesConfig.CustomWindowsPackageURL = c.acsCustomWinBinariesURL
	}
	if c.acsCustomCcmURL != "" {
		useCloudControllerManager := true
		v.Properties.OrchestratorProfile.KubernetesConfig.UseCloudControllerManager = &useCloudControllerManager
		v.Properties.OrchestratorProfile.KubernetesConfig.CustomCcmImage = c.acsCustomCcmURL
	}

	if c.isAzureStackCloud() {
		v.Properties.CustomCloudProfile.PortalURL = c.azureCustomCloudURL
	}
	apiModel, _ := json.MarshalIndent(v, "", "    ")
	c.apiModelPath = path.Join(c.outputDir, "kubernetes.json")
	err = ioutil.WriteFile(c.apiModelPath, apiModel, 0644)
	if err != nil {
		return fmt.Errorf("cannot write apimodel to file: %v", err)
	}
	return nil
}

func (c *Cluster) getAcsEngine(retry int) error {
	downloadPath := path.Join(os.Getenv("HOME"), "aks-engine.tar.gz")
	f, err := os.Create(downloadPath)
	if err != nil {
		return err
	}
	defer f.Close()

	for i := 0; i < retry; i++ {
		log.Printf("downloading %v from %v.", downloadPath, *acsEngineURL)
		if err := httpRead(*acsEngineURL, f); err == nil {
			break
		}
		err = fmt.Errorf("url=%s failed get %v: %v.", *acsEngineURL, downloadPath, err)
		if i == retry-1 {
			return err
		}
		log.Println(err)
		sleep(time.Duration(i) * time.Second)
	}

	f.Close()
	if *acsEngineMD5 != "" {
		o, err := control.Output(exec.Command("md5sum", f.Name()))
		if err != nil {
			return err
		}
		if strings.Split(string(o), " ")[0] != *acsEngineMD5 {
			return fmt.Errorf("wrong md5 sum for acs-engine.")
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("unable to get current directory: %v .", err)
	}
	log.Printf("Extracting tar file %v into directory %v .", f.Name(), cwd)

	if err = control.FinishRunning(exec.Command("tar", "-xzf", f.Name(), "--strip", "1")); err != nil {
		return err
	}
	c.acsEngineBinaryPath = path.Join(cwd, "aks-engine")
	return nil

}

func (c Cluster) generateARMTemplates() error {
	if err := control.FinishRunning(exec.Command(c.acsEngineBinaryPath, "generate", c.apiModelPath, "--output-directory", c.outputDir)); err != nil {
		return fmt.Errorf("failed to generate ARM templates: %v.", err)
	}
	return nil
}

func (c *Cluster) loadARMTemplates() error {
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

func (c *Cluster) getARMClient(ctx context.Context) error {
	// instantiate Azure Resource Manager Client
	env, err := azure.EnvironmentFromName(c.azureEnvironment)
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

func (c *Cluster) createCluster() error {
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
	return nil

}

func (c *Cluster) dockerLogin() error {
	cwd, _ := os.Getwd()
	log.Printf("CWD %v", cwd)
	cmd := &exec.Cmd{}
	username := ""
	pwd := ""
	server := ""
	var err error

	if !strings.Contains(os.Getenv("REGISTRY"), "azurecr.io") {
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
		server = os.Getenv("REGISTRY")
	}
	cmd = exec.Command("docker", "login", fmt.Sprintf("--username=%s", username), fmt.Sprintf("--password=%s", pwd), server)
	if err = cmd.Run(); err != nil {
		return fmt.Errorf("failed Docker login with error: %v", err)
	}
	log.Println("Docker login success.")
	return nil
}
func dockerLogout() error {
	log.Println("Docker logout.")
	cmd := exec.Command("docker", "logout")
	return cmd.Run()
}

func (c *Cluster) buildCcm() error {

	image := fmt.Sprintf("%v/azure-cloud-controller-manager:%v-%v", os.Getenv("REGISTRY"), os.Getenv("BUILD_ID"), uuid.NewV1().String()[:8])
	if err := c.dockerLogin(); err != nil {
		return err
	}
	log.Println("Building ccm.")
	projectPath := util.K8s("cloud-provider-azure")
	log.Printf("projectPath %v", projectPath)
	cmd := exec.Command("docker", "build", "-t", image, ".")
	cmd.Dir = projectPath
	if err := control.FinishRunning(cmd); err != nil {
		return err
	}

	cmd = exec.Command("docker", "push", image)
	cmd.Stdout = ioutil.Discard
	if err := control.FinishRunning(cmd); err != nil {
		return err
	}
	c.acsCustomCcmURL = image
	if err := dockerLogout(); err != nil {
		log.Println("Docker logout failed.")
		return err
	}
	log.Printf("Custom cloud controller manager URL: %v .", c.acsCustomCcmURL)
	return nil
}

func (c *Cluster) buildHyperKube() error {

	os.Setenv("VERSION", fmt.Sprintf("azure-e2e-%v-%v", os.Getenv("BUILD_ID"), uuid.NewV1().String()[:8]))
	if err := c.dockerLogin(); err != nil {
		return err
	}
	log.Println("Building and pushing hyperkube.")
	pushHyperkube := util.K8s("kubernetes", "hack", "dev-push-hyperkube.sh")
	cmd := exec.Command(pushHyperkube)
	// dev-push-hyperkube will produce a lot of output to stdout. We should capture the output here.
	cmd.Stdout = ioutil.Discard
	if err := control.FinishRunning(cmd); err != nil {
		return err
	}
	c.acsCustomHyperKubeURL = fmt.Sprintf("%s/hyperkube-amd64:%s", os.Getenv("REGISTRY"), os.Getenv("VERSION"))
	if err := dockerLogout(); err != nil {
		log.Println("Docker logout failed.")
		return err
	}
	log.Printf("Custom hyperkube URL: %v .", c.acsCustomHyperKubeURL)
	return nil
}

func (c *Cluster) uploadZip(zipPath string) error {

	credential, err := azblob.NewSharedKeyCredential(c.credentials.StorageAccountName, c.credentials.StorageAccountKey)
	if err != nil {
		return fmt.Errorf("new shared key credential: %v", err)
	}
	p := azblob.NewPipeline(credential, azblob.PipelineOptions{})

	var containerName string = os.Getenv("AZ_STORAGE_CONTAINER_NAME")

	URL, _ := url.Parse(
		fmt.Sprintf("https://%s.blob.core.windows.net/%s", c.credentials.StorageAccountName, containerName))

	containerURL := azblob.NewContainerURL(*URL, p)
	file, err := os.Open(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open file %v . Error %v", zipPath, err)
	}
	blobURL := containerURL.NewBlockBlobURL(filepath.Base(file.Name()))
	_, err1 := azblob.UploadFileToBlockBlob(context.Background(), file, blobURL, azblob.UploadToBlockBlobOptions{})
	file.Close()
	if err1 != nil {
		return err1
	}
	blobURLString := blobURL.URL()
	c.acsCustomWinBinariesURL = blobURLString.String()
	log.Printf("Custom win binaries url: %v", c.acsCustomWinBinariesURL)
	return nil
}

func getApiModelTemplate(url string, downloadPath string, retry int) (string, error) {

	f, err := os.Create(downloadPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	for i := 0; i < retry; i++ {
		log.Printf("downloading %v from %v.", downloadPath, url)
		if err := httpRead(url, f); err == nil {
			break
		}
		err = fmt.Errorf("url=%s failed get %v: %v.", url, downloadPath, err)
		if i == retry-1 {
			return "", err
		}
		log.Println(err)
		sleep(time.Duration(i) * time.Second)
	}
	f.Chmod(0644)
	return downloadPath, nil

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

func (c *Cluster) buildWinZip() error {

	zipName := fmt.Sprintf("%s%s.zip", os.Getenv("BUILD_ID"), uuid.NewV1().String()[:8])
	buildFolder := path.Join(os.Getenv("HOME"), "winbuild")
	zipPath := path.Join(os.Getenv("HOME"), zipName)
	log.Printf("Building %s", zipName)
	buildScriptPath, err := getZipBuildScript(*acsWinZipBuildScript, 2)
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
	if err := c.uploadZip(zipPath); err != nil {
		return err
	}
	return nil
}

func (c Cluster) Up() error {

	var err error
	if *acsCcm == true {
		err = c.buildCcm()
		if err != nil {
			return fmt.Errorf("error building cloud controller manager %v", err)
		}
	}
	if *acsHyperKube == true {
		err = c.buildHyperKube()
		if err != nil {
			return fmt.Errorf("error building hyperkube %v", err)
		}
	}
	if *acsWinBinaries == true {
		err = c.buildWinZip()
		if err != nil {
			return fmt.Errorf("error building windowsZipFile %v", err)
		}
	}
	if c.apiModelPath != "" {
		templateFile, err := getApiModelTemplate(c.apiModelPath, path.Join(c.outputDir, "kubernetes.json"), 2)
		if err != nil {
			return fmt.Errorf("error downloading ApiModel template: %v with error %v", c.apiModelPath, err)
		}
		c.apiModelPath = templateFile
	}

	err = c.populateApiModelTemplate()
	if err != nil {
		return fmt.Errorf("failed to populate acs-engine apimodel template: %v", err)
	}

	if *acsEngineURL != "" {
		err = c.getAcsEngine(2)
		if err != nil {
			return fmt.Errorf("failed to get ACS Engine binary: %v", err)
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

func (c Cluster) Down() error {
	log.Printf("Deleting resource group: %v.", c.resourceGroup)
	return c.azureClient.DeleteResourceGroup(c.ctx, c.resourceGroup)
}

func (c Cluster) DumpClusterLogs(localPath, gcsPath string) error {
	return nil
}

func (c Cluster) GetClusterCreated(clusterName string) (time.Time, error) {
	return time.Time{}, errors.New("not implemented")
}

func (c Cluster) TestSetup() error {

	// set env vars required by the ccm e2e tests
	if *testCcm == true {
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

func (c Cluster) IsUp() error {
	return isUp(c)
}

func (_ Cluster) KubectlCommand() (*exec.Cmd, error) { return nil, nil }

// BuildTester returns a standard ginkgo-script tester or a custom one if testCcm is enabled
func (c *Cluster) BuildTester(o *e2e.BuildTesterOptions) (e2e.Tester, error) {
	if *testCcm != true {
		return &GinkgoScriptTester{}, nil
	}
	log.Printf("running go tests directly")
	return &GinkgoCustomTester{}, nil
}

// GinkgoCustomTester implements Tester by calling a custom ginkgo script
type GinkgoCustomTester struct {
}

// Run executes custom ginkgo script
func (t *GinkgoCustomTester) Run(control *process.Control, testArgs []string) error {
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
	projectPath := util.K8s("cloud-provider-azure")
	cmd.Dir = projectPath
	testErr := control.FinishRunning(cmd)
	return testErr
}
