/*
Copyright 2020 The Kubernetes Authors.

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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	mathrand "math/rand"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/containerservice/mgmt/2019-10-01/containerservice"
	"github.com/Azure/go-autorest/autorest/azure"
	"golang.org/x/crypto/ssh"
)

const charset = "abcdefghijklmnopqrstuvwxyz" + "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var aksCustomHeaders = flag.String("aks-custom-headers", "", "comma-separated list of key=value tuples for headers to apply to the cluster creation request.")

type aksDeployer struct {
	azureCreds       *Creds
	azureClient      *AzureClient
	azureEnvironment string
	templateURL      string
	outputDir        string
	resourceGroup    string
	resourceName     string
	location         string
	k8sVersion       string
	customHeaders    map[string]string
}

func newAksDeployer() (*aksDeployer, error) {
	if err := validateAksFlags(); err != nil {
		return nil, err
	}

	customHeaders := map[string]string{}
	if *aksCustomHeaders != "" {
		tokens := strings.Split(*aksCustomHeaders, ",")
		for _, token := range tokens {
			parts := strings.Split(token, "=")
			if len(parts) != 2 {
				return nil, fmt.Errorf("incorrectly formatted custom header, use format key=val[,key2=val2]: %s", token)
			}
			customHeaders[parts[0]] = parts[1]
		}
	}

	creds, err := getAzCredentials()
	if err != nil {
		return nil, fmt.Errorf("failed to get azure credentials: %v", err)
	}

	env, err := azure.EnvironmentFromName(*aksAzureEnv)
	if err != nil {
		return nil, fmt.Errorf("failed to determine azure environment: %v", err)
	}

	var client *AzureClient
	if client, err = getAzureClient(env,
		creds.SubscriptionID,
		creds.ClientID,
		creds.TenantID,
		creds.ClientSecret,
	); err != nil {
		return nil, fmt.Errorf("error trying to get Azure Client: %v", err)
	}

	outputDir, err := ioutil.TempDir(os.Getenv("HOME"), "aks")
	if err != nil {
		return nil, fmt.Errorf("error creating tempdir: %v", err)
	}

	a := &aksDeployer{
		azureCreds:       creds,
		azureClient:      client,
		azureEnvironment: *aksAzureEnv,
		templateURL:      *aksTemplateURL,
		outputDir:        outputDir,
		resourceGroup:    *aksResourceGroupName,
		resourceName:     *aksResourceName,
		location:         *aksLocation,
		k8sVersion:       *aksOrchestratorRelease,
		customHeaders:    customHeaders,
	}

	if err := a.dockerLogin(); err != nil {
		return nil, err
	}

	return a, nil
}

func validateAksFlags() error {
	if *aksCredentialsFile == "" {
		return fmt.Errorf("no credentials file path specified")
	}
	if *aksResourceName == "" {
		// Must be short or managed node resource group name will exceed 80 char
		*aksResourceName = "kubetest-" + randString(8)
	}
	if *aksResourceGroupName == "" {
		*aksResourceGroupName = *aksResourceName
	}
	if *aksDNSPrefix == "" {
		*aksDNSPrefix = *aksResourceName
	}
	return nil
}

func (a *aksDeployer) Up() error {
	log.Printf("Creating AKS cluster %v in resource group %v", a.resourceName, a.resourceGroup)
	templateFile, err := downloadFromURL(a.templateURL, path.Join(a.outputDir, "kubernetes.json"), 2)
	if err != nil {
		return fmt.Errorf("error downloading AKS cluster template: %v with error %v", a.templateURL, err)
	}

	template, err := ioutil.ReadFile(templateFile)
	if err != nil {
		return fmt.Errorf("failed to read downloaded cluster template file: %v", err)
	}

	var model containerservice.ManagedCluster
	if err := json.Unmarshal(template, &model); err != nil {
		return fmt.Errorf("failed to unmarshal managedcluster model: %v", err)
	}

	_, sshPublicKey, err := newSSHKeypair(4096)
	if err != nil {
		return fmt.Errorf("failed to generate ssh key for cluster creation: %v", err)
	}

	*(*model.LinuxProfile.SSH.PublicKeys)[0].KeyData = string(sshPublicKey)
	model.ManagedClusterProperties.DNSPrefix = aksDNSPrefix
	model.ManagedClusterProperties.ServicePrincipalProfile.ClientID = &a.azureCreds.ClientID
	model.ManagedClusterProperties.ServicePrincipalProfile.Secret = &a.azureCreds.ClientSecret
	model.Location = &a.location
	model.ManagedClusterProperties.KubernetesVersion = &a.k8sVersion

	log.Printf("Creating Azure resource group: %v for cluster deployment.", a.resourceGroup)
	_, err = a.azureClient.EnsureResourceGroup(context.Background(), a.resourceGroup, a.location, nil)
	if err != nil {
		return fmt.Errorf("could not ensure resource group: %v", err)
	}

	req, err := a.azureClient.managedClustersClient.CreateOrUpdatePreparer(context.Background(), a.resourceGroup, a.resourceName, model)
	if err != nil {
		return fmt.Errorf("failed to prepare cluster creation: %v", err)
	}

	for key, val := range a.customHeaders {
		req.Header[key] = []string{val}
	}

	future, err := a.azureClient.managedClustersClient.CreateOrUpdateSender(req)
	if err != nil {
		return fmt.Errorf("failed to respond to cluster creation: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*25)
	defer cancel()
	if err := future.WaitForCompletionRef(ctx, a.azureClient.managedClustersClient.Client); err != nil {
		return fmt.Errorf("failed long async cluster creation: %v", err)
	}

	credentialList, err := a.azureClient.managedClustersClient.ListClusterAdminCredentials(context.Background(), a.resourceGroup, a.resourceName)
	if err != nil {
		return fmt.Errorf("failed to list kubeconfigs: %v", err)
	}
	if credentialList.Kubeconfigs == nil || len(*credentialList.Kubeconfigs) < 1 {
		return fmt.Errorf("no kubeconfigs available for the aks cluster")
	}

	kubeconfigPath := path.Join(a.outputDir, "kubeconfig")
	if err := ioutil.WriteFile(kubeconfigPath, *(*credentialList.Kubeconfigs)[0].Value, 0644); err != nil {
		return fmt.Errorf("failed to write kubeconfig out")
	}

	managedCluster, err := future.Result(a.azureClient.managedClustersClient)
	if err != nil {
		return fmt.Errorf("failed to extract resulting managed cluster: %v", err)
	}
	masterIP := *managedCluster.ManagedClusterProperties.Fqdn
	if err != nil {
		return fmt.Errorf("failed to get masterIP: %v", err)
	}
	masterInternalIP := masterIP

	if err := os.Setenv("KUBE_MASTER_IP", strings.TrimSpace(string(masterIP))); err != nil {
		return err
	}

	// MASTER_IP variable is required by the clusterloader. It requires to have master ip provided,
	// due to master being unregistered.
	if err := os.Setenv("MASTER_IP", strings.TrimSpace(string(masterIP))); err != nil {
		return err
	}

	// MASTER_INTERNAL_IP variable is needed by the clusterloader2 when running on kubemark clusters.
	if err := os.Setenv("MASTER_INTERNAL_IP", strings.TrimSpace(string(masterInternalIP))); err != nil {
		return err
	}

	if err := os.Setenv("KUBECONFIG", kubeconfigPath); err != nil {
		return err
	}

	log.Printf("Populating Azure cloud config")
	isVMSS := (*managedCluster.ManagedClusterProperties.AgentPoolProfiles)[0].Type == "" || (*managedCluster.ManagedClusterProperties.AgentPoolProfiles)[0].Type == availabilityProfileVMSS
	if err := populateAzureCloudConfig(isVMSS, *a.azureCreds, a.azureEnvironment, a.resourceGroup, a.location, a.outputDir); err != nil {
		return err
	}

	return nil
}

func (a *aksDeployer) IsUp() error { return isUp(a) }

func (a *aksDeployer) DumpClusterLogs(localPath, gcsPath string) error {
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
		logDumpScript, err := downloadFromURL(logDumpURLPrefix+"log-dump.sh", path.Join(a.outputDir, "log-dump.sh"), 2)
		if err != nil {
			return fmt.Errorf("error downloading log dump script: %v", err)
		}
		if err := control.FinishRunning(exec.Command("chmod", "+x", logDumpScript)); err != nil {
			return fmt.Errorf("error changing access permission for %s: %v", logDumpScript, err)
		}
		if _, err := downloadFromURL(logDumpURLPrefix+"log-dump-daemonset.yaml", path.Join(a.outputDir, "log-dump-daemonset.yaml"), 2); err != nil {
			return fmt.Errorf("error downloading log dump manifest: %v", err)
		}

		if err := control.FinishRunning(exec.Command("bash", "-c", logDumpScript)); err != nil {
			return fmt.Errorf("error running log collection script %s: %v", logDumpScript, err)
		}
		return nil
	}

	return logDumper()
}

// NB(alexeldeib): order of execution is when running scalability tests is:
// kubemarkUp -> IsUp -> TestSetup -> Up -> TestSetup
// When executing other tests, the order is:
// Up -> TestSetup
// The kubeconfig must be available during kubemark tests, so we have to set it both in TestSetup and in Up.
// The masterIP and masterInternalIP must be available for all tests.
func (a *aksDeployer) TestSetup() error {

	if err := os.Setenv("KUBEMARK_RESOURCE_GROUP", *aksResourceGroupName); err != nil {
		return err
	}

	if err := os.Setenv("KUBEMARK_RESOURCE_NAME", *aksResourceName); err != nil {
		return err
	}

	if err := os.Setenv("CLOUD_PROVIDER", "aks"); err != nil {
		return err
	}

	return nil
}

func (a *aksDeployer) Down() error {
	log.Printf("Deleting resource group: %v.", a.resourceGroup)
	return a.azureClient.DeleteResourceGroup(context.Background(), a.resourceGroup)
}

func (a *aksDeployer) GetClusterCreated(_ string) (time.Time, error) { return time.Now(), nil }

// KubectlCommand uses the default command configuration.
func (a *aksDeployer) KubectlCommand() (*exec.Cmd, error) { return nil, nil }

func newSSHKeypair(bits int) (private, public []byte, err error) {
	// Private Key generation
	privateKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, nil, err
	}

	// Validate Private Key
	err = privateKey.Validate()
	if err != nil {
		return nil, nil, err
	}

	// Get ASN.1 DER format
	privDER := x509.MarshalPKCS1PrivateKey(privateKey)

	// pem.Block
	privBlock := pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   privDER,
	}

	// Private key in PEM format
	privBytes := pem.EncodeToMemory(&privBlock)

	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, nil, err
	}

	pubBytes := ssh.MarshalAuthorizedKey(publicKey)

	return privBytes, pubBytes, nil
}

func (a *aksDeployer) dockerLogin() error {
	username := ""
	pwd := ""
	server := ""

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
		username = a.azureCreds.ClientID
		pwd = a.azureCreds.ClientSecret
		server = imageRegistry
	}
	cmd := exec.Command("docker", "login", fmt.Sprintf("--username=%s", username), fmt.Sprintf("--password=%s", pwd), server)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed Docker login with output %s\n error: %v", out, err)
	}
	log.Println("Docker login success.")
	return nil
}

func randString(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[mathrand.Intn(len(charset))]
	}
	return string(b)
}
