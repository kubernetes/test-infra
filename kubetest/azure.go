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
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/pelletier/go-toml"
	"k8s.io/test-infra/kubetest/util"

	"github.com/Azure/azure-storage-blob-go/2016-05-31/azblob"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/satori/go.uuid"
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
	acsWinBinaries         = flag.Bool("acsengine-win-binaries", false, "Set to True if you want kubetest to build a custom zip with windows binaries for acs-engine")
	acsHyperKube           = flag.Bool("acsengine-hyperkube", false, "Set to True if you want kubetest to build a custom hyperkube for acs-engine")
	acsCredentialsFile     = flag.String("acsengine-creds", "", "Path to credential file for Azure")
	acsOrchestratorRelease = flag.String("acsengine-orchestratorRelease", "", "Orchestrator Profile for acs-engine")
	acsWinZipBuildScript   = flag.String("acsengine-winZipBuildScript", "https://raw.githubusercontent.com/Azure/acs-engine/master/scripts/build-windows-k8s.sh", "Build script to create custom zip containing win binaries for acs-engine")
	acsNetworkPlugin       = flag.String("acsengine-networkPlugin", "azure", "Network pluging to use with acs-engine")
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
	agentPoolCount          int
	k8sVersion              string
	networkPlugin           string
	azureClient             *AzureClient
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

func checkParams() error {
	if *acsCredentialsFile == "" {
		return fmt.Errorf("no credentials file path specified")
	}
	if *acsResourceName == "" {
		*acsResourceName = "kubetest-" + uuid.NewV1().String()
	}
	if *acsResourceGroupName == "" {
		*acsResourceGroupName = *acsResourceName + "-rg"
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
		acsCustomHyperKubeURL:   "",
		acsCustomWinBinariesURL: "",
		acsEngineBinaryPath:     "aks-engine", // use the one in path by default
	}
	c.getAzCredentials()
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
	v.Properties.MasterProfile.Distro = "ubuntu"
	for _, agentPool := range v.Properties.AgentPoolProfiles {
		agentPool.Distro = "ubuntu"
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
	v.Properties.OrchestratorProfile.KubernetesConfig.NetworkPlugin = c.networkPlugin // default Azure
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
	}
	if c.acsCustomWinBinariesURL != "" {
		v.Properties.OrchestratorProfile.KubernetesConfig.CustomWindowsPackageURL = c.acsCustomWinBinariesURL
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
	env, err := azure.EnvironmentFromName("AzurePublicCloud")
	var client *AzureClient
	if client, err = getAzureClient(env,
		c.credentials.SubscriptionID,
		c.credentials.ClientID,
		c.credentials.TenantID,
		c.credentials.ClientSecret); err != nil {
		return fmt.Errorf("error trying to get Azure Client: %v", err)
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

func (c *Cluster) buildHyperKube() error {

	os.Setenv("VERSION", fmt.Sprintf("azure-e2e-%v", os.Getenv("BUILD_ID")))

	cwd, _ := os.Getwd()
	log.Printf("CWD %v", cwd)

	username := os.Getenv("DOCKER_USERNAME")
	password_file := os.Getenv("DOCKER_PASSWORD_FILE")
	password, err := ioutil.ReadFile(password_file)
	if err != nil {
		return fmt.Errorf("error reading docker passoword file %v: %v.", password_file, err)
	}

	log.Println("Attempting Docker login.")
	cmd := exec.Command("docker", "login", fmt.Sprintf("--username=%s", username), fmt.Sprintf("--password=%s", strings.TrimSuffix(string(password), "\n")))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Docker login failed with error: %v.", err)
	}
	log.Println("Docker login success.")
	log.Println("Building hyperkube.")

	pushHyperkube := util.K8s("kubernetes", "hack", "dev-push-hyperkube.sh")
	cmd = exec.Command(pushHyperkube)
	// dev-push-hyperkube will produce a lot of output to stdout. We should capture the output here.
	cmd.Stdout = ioutil.Discard
	if err1 := control.FinishRunning(cmd); err1 != nil {
		return err1
	}
	c.acsCustomHyperKubeURL = fmt.Sprintf("%s/hyperkube-amd64:%s", os.Getenv("REGISTRY"), os.Getenv("VERSION"))

	log.Println("Docker logout.")
	cmd = exec.Command("docker", "logout")
	if err := cmd.Run(); err != nil {
		log.Println("Docker logout failed.")
	}
	log.Printf("Custom hyperkube URL: %v .", c.acsCustomHyperKubeURL)
	return nil
}

func (c *Cluster) uploadZip(zipPath string) error {

	credential := azblob.NewSharedKeyCredential(c.credentials.StorageAccountName, c.credentials.StorageAccountKey)
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

	zipName := fmt.Sprintf("%s.zip", os.Getenv("BUILD_ID"))
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
