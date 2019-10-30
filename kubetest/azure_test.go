/*
Copyright 2019 The Kubernetes Authors.

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
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"strings"
	"testing"
)

const (
	fakeCreds = `
	[Creds]
	ClientID = "df7269f2-xxxx-xxxx-xxxx-0f12a7d97404"
	ClientSecret = "8c416dc5-xxxx-xxxx-xxxx-d77069e2a255"
	TenantID = "72f988bf-xxxx-xxxx-xxxx-2d7cd011db47"
	SubscriptionID = "b9d2281e-xxxx-xxxx-xxxx-0d50377cdf76"
	StorageAccountName = "TestStorageAccountName"
	StorageAccountKey = "TestStorageAccountKey"
	`
	fakeAPIModelTemplate = `
	{
	    "apiVersion": "vlabs",
	    "location": "",
	    "properties": {
	        "orchestratorProfile": {
	            "orchestratorType": "Kubernetes",
	            "orchestratorRelease": "1.15",
	            "kubernetesConfig": {
	                "useCloudControllerManager": true,
	                "customCcmImage": "gcrio.azureedge.net/google_containers/cloud-controller-manager-amd64:v1.15.0",
	                "customHyperkubeImage": "gcrio.azureedge.net/google_containers/hyperkube-amd64:v1.15.0",
	                "networkPolicy": "none",
	                "cloudProviderRateLimitQPS": 6,
	                "cloudProviderRateLimitBucket": 20,
	                "controllerManagerConfig": {
	                    "--feature-gates": "CSIInlineVolume=true,LocalStorageCapacityIsolation=true,ServiceNodeExclusion=true"
	                },
	                "apiServerConfig": {
	                    "--enable-admission-plugins": "NamespaceLifecycle,LimitRanger,ServiceAccount,DefaultStorageClass,ResourceQuota,AlwaysPullImages"
	                }
	            }
	        },
	        "masterProfile": {
	            "count": 1,
	            "dnsPrefix": "{dnsPrefix}",
	            "vmSize": "Standard_F2"
	        },
	        "agentPoolProfiles": [
	            {
	                "name": "agentpool1",
	                "count": 2,
	                "vmSize": "Standard_F2",
	                "availabilityProfile": "AvailabilitySet",
	                "storageProfile": "ManagedDisks"
	            }
	        ],
	        "linuxProfile": {
	            "adminUsername": "k8s-ci",
	            "ssh": {
	                "publicKeys": [
	                    {
	                        "keyData": "{keyData}"
	                    }
	                ]
	            }
	        },
	        "servicePrincipalProfile": {
	            "clientID": "{servicePrincipalClientID}",
	            "secret": "{servicePrincipalClientSecret}"
	        }
	    }
	}
	`
	expectedAPIModel = `
	{
        "location": "location",
        "name": "name",
        "APIVersion": "vlabs",
        "properties": {
            "orchestratorProfile": {
                "orchestratorType": "Kubernetes",
                "orchestratorRelease": "k8sVersion",
                "kubernetesConfig": {
                    "customWindowsPackageURL": "aksCustomWinBinariesURL",
                    "customHyperkubeImage": "aksCustomHyperKubeURL",
                    "customCcmImage": "aksCustomCcmURL",
                    "useCloudControllerManager": true,
                    "networkPolicy": "none",
                    "cloudProviderRateLimitQPS": 6,
                    "cloudProviderRateLimitBucket": 20,
                    "apiServerConfig": {
                        "--enable-admission-plugins": "NamespaceLifecycle,LimitRanger,ServiceAccount,DefaultStorageClass,ResourceQuota,AlwaysPullImages"
                    },
                    "controllerManagerConfig": {
                        "--feature-gates": "CSIInlineVolume=true,LocalStorageCapacityIsolation=true,ServiceNodeExclusion=true"
                    }
                }
            },
            "masterProfile": {
                "count": 1,
                "distro": "ubuntu",
                "dnsPrefix": "dnsPrefix",
                "vmSize": "masterVMSize"
            },
            "agentPoolProfiles": [
                {
                    "name": "agentpool1",
                    "count": 10,
                    "distro": "ubuntu",
                    "vmSize": "agentVMSize",
                    "availabilityProfile": "AvailabilitySet"
                }
            ],
            "linuxProfile": {
                "adminUsername": "adminUsername",
                "ssh": {
                    "publicKeys": [
                        {
                            "keyData": "sshPublicKey"
                        }
                    ]
                }
            },
            "servicePrincipalProfile": {
                "clientId": "ClientID",
                "secret": "ClientSecret"
            }
        }
    }
	`
)

func TestCheckParams(t *testing.T) {
	testCases := []struct {
		desc          string
		setParams     func()
		assertParams  func(t *testing.T)
		expectedError bool
	}{
		{
			desc: "transitioning from acs flags to aks flags",
			setParams: func() {
				*acsResourceName = "acsResourceName"
				*acsResourceGroupName = "acsResourceGroupName"
				*acsLocation = "acsLocation"
				*acsMasterVmSize = "acsMasterVmSize"
				*acsAgentVmSize = "acsAgentVmSize"
				*acsAdminUsername = "acsAdminUsername"
				*acsAdminPassword = "acsAdminPassword"
				*acsAgentPoolCount = 2
				*acsTemplateURL = "acsTemplateURL"
				*acsDnsPrefix = "acsDnsPrefix"
				*acsEngineURL = "acsEngineURL"
				*acsEngineMD5 = "acsEngineMD5"
				*acsSSHPublicKeyPath = "acsSSHPublicKeyPath"
				*acsWinBinaries = true
				*acsHyperKube = true
				*acsCcm = true
				*acsCredentialsFile = "acsCredentialsFile"
				*acsOrchestratorRelease = "acsOrchestratorRelease"
				*acsWinZipBuildScript = "acsWinZipBuildScript"
				*acsNetworkPlugin = "acsNetworkPlugin"
				*acsAzureEnv = "acsAzureEnv"
				*acsIdentitySystem = "acsIdentitySystem"
				*acsCustomCloudURL = "acsCustomCloudURL"
			},
			assertParams: func(t *testing.T) {
				if *acsResourceName != *aksResourceName {
					t.Errorf("expected aksResourceName to be %s, but got %s", *acsResourceName, *aksResourceName)
				}
				if *acsResourceGroupName != *aksResourceGroupName {
					t.Errorf("expected aksResourceGroupName to be %s, but got %s", *acsResourceGroupName, *aksResourceGroupName)
				}
				if *acsLocation != *aksLocation {
					t.Errorf("expected aksLocation to be %s, but got %s", *acsLocation, *aksLocation)
				}
				if *acsMasterVmSize != *aksMasterVmSize {
					t.Errorf("expected aksMasterVmSize to be %s, but got %s", *acsMasterVmSize, *aksMasterVmSize)
				}
				if *acsAgentVmSize != *aksAgentVmSize {
					t.Errorf("expected aksAgentVmSize to be %s, but got %s", *acsAgentVmSize, *aksAgentVmSize)
				}
				if *acsAdminUsername != *aksAdminUsername {
					t.Errorf("expected aksAdminUsername to be %s, but got %s", *acsAdminUsername, *aksAdminUsername)
				}
				if *acsAdminPassword != *aksAdminPassword {
					t.Errorf("expected aksAdminPassword to be %s, but got %s", *acsAdminPassword, *aksAdminPassword)
				}
				if *acsAgentPoolCount != *aksAgentPoolCount {
					t.Errorf("expected aksAgentPoolCount to be %d, but got %d", *acsAgentPoolCount, *aksAgentPoolCount)
				}
				if *acsTemplateURL != *aksTemplateURL {
					t.Errorf("expected aksTemplateURL to be %s, but got %s", *acsTemplateURL, *aksTemplateURL)
				}
				if *acsDnsPrefix != *aksDnsPrefix {
					t.Errorf("expected aksDnsPrefix to be %s, but got %s", *acsDnsPrefix, *aksDnsPrefix)
				}
				if *acsEngineURL != *aksEngineURL {
					t.Errorf("expected aksEngineURL to be %s, but got %s", *acsEngineURL, *aksEngineURL)
				}
				if *acsEngineMD5 != *aksEngineMD5 {
					t.Errorf("expected aksEngineMD5 to be %s, but got %s", *acsEngineMD5, *aksEngineMD5)
				}
				if *acsSSHPublicKeyPath != *aksSSHPublicKeyPath {
					t.Errorf("expected aksSSHPublicKeyPath to be %s, but got %s", *acsSSHPublicKeyPath, *aksSSHPublicKeyPath)
				}
				if *acsWinBinaries != *aksWinBinaries {
					t.Errorf("expected aksWinBinaries to be %t, but got %t", *acsWinBinaries, *aksWinBinaries)
				}
				if *acsHyperKube != *aksHyperKube {
					t.Errorf("expected aksHyperKube to be %t, but got %t", *acsHyperKube, *aksHyperKube)
				}
				if *acsCcm != *aksCcm {
					t.Errorf("expected aksCcm to be %t, but got %t", *acsCcm, *aksCcm)
				}
				if *acsCredentialsFile != *aksCredentialsFile {
					t.Errorf("expected aksCredentialsFile to be %s, but got %s", *acsCredentialsFile, *aksCredentialsFile)
				}
				if *acsOrchestratorRelease != *aksOrchestratorRelease {
					t.Errorf("expected aksOrchestratorRelease to be %s, but got %s", *acsOrchestratorRelease, *aksOrchestratorRelease)
				}
				if *acsWinZipBuildScript != *aksWinZipBuildScript {
					t.Errorf("expected aksWinZipBuildScript to be %s, but got %s", *acsWinZipBuildScript, *aksWinZipBuildScript)
				}
				if *acsNetworkPlugin != *aksNetworkPlugin {
					t.Errorf("expected aksNetworkPlugin to be %s, but got %s", *acsNetworkPlugin, *aksNetworkPlugin)
				}
				if *acsAzureEnv != *aksAzureEnv {
					t.Errorf("expected aksAzureEnv to be %s, but got %s", *acsAzureEnv, *aksAzureEnv)
				}
				if *acsIdentitySystem != *aksIdentitySystem {
					t.Errorf("expected aksIdentitySystem to be %s, but got %s", *acsIdentitySystem, *aksIdentitySystem)
				}
				if *acsCustomCloudURL != *aksCustomCloudURL {
					t.Errorf("expected aksCustomCloudURL to be %s, but got %s", *acsCustomCloudURL, *aksCustomCloudURL)
				}
			},
			expectedError: false,
		},
		{
			desc: "empty aksCredentialsFile should return an error",
			setParams: func() {
				*aksCredentialsFile = ""
			},
			expectedError: true,
		},
		{
			desc: "aksResourceName should have a 'kubetest' prefix if it was empty",
			setParams: func() {
				*aksResourceName = ""
			},
			assertParams: func(t *testing.T) {
				if !strings.HasPrefix(*aksResourceName, "kubetest-") {
					t.Errorf("expected aksResourceName to have 'kubetest' prefix, but got %s", *aksResourceName)
				}
				if len(*aksResourceName) < 3 || len(*aksResourceName) > 45 {
					t.Errorf("expected the length of aksResourceName to be 45, but got %d", len(*aksResourceName))
				}
			},
			expectedError: false,
		},
		{
			desc: "aksResourceGroupName should be equal to aksResourceName if aksResourceGroupName was empty",
			setParams: func() {
				*aksResourceName = "aksResourceName"
				*aksResourceGroupName = ""
			},
			assertParams: func(t *testing.T) {
				if *aksResourceName != *aksResourceGroupName {
					t.Errorf("expected aksResourceGroupName to be %s , but got %s", *aksResourceName, *aksResourceGroupName)
				}
			},
			expectedError: false,
		},
		{
			desc: "length 100 resource group name should return an error",
			setParams: func() {
				*aksResourceGroupName = "aksResourceGroupNameaksResourceGroupNameaksResourceGroupNameaksResourceGroupNameaksResourceGroupName"
			},
			expectedError: true,
		},
		{
			desc: "length 2 dns prefix should return an error",
			setParams: func() {
				*aksDnsPrefix = "ak"
			},
			expectedError: true,
		},
		{
			desc: "length 48 dns prefix should return an error",
			setParams: func() {
				*aksDnsPrefix = "aksDnsPrefixaksDnsPrefixaksDnsPrefixaksDnsPrefix"
			},
			expectedError: true,
		},
		{
			desc: "empty aksTemplateURL should return an error",
			setParams: func() {
				*aksTemplateURL = ""
			},
			expectedError: true,
		},
		{
			desc: "aksDnsPrefix should be equal to aksResourceName if aksDnsPrefix was empty",
			setParams: func() {
				*aksDnsPrefix = ""
				*aksResourceName = "aksResourceName"
			},
			assertParams: func(t *testing.T) {
				if *aksDnsPrefix != *aksResourceName {
					t.Errorf("expected aksDnsPrefix to be %s, but got %s", *aksResourceName, *aksDnsPrefix)
				}
			},
			expectedError: false,
		},
		{
			desc: "aksSSHPublicKeyPath should be equal to default ssh key if it was empty",
			setParams: func() {
				*aksSSHPublicKeyPath = ""
			},
			assertParams: func(t *testing.T) {
				expectedAksSSHPublicKeyPath := os.Getenv("HOME") + "/.ssh/id_rsa.pub"
				if *aksSSHPublicKeyPath != expectedAksSSHPublicKeyPath {
					t.Errorf("expected aksSSHPublicKeyPath to be %s, but got %s", expectedAksSSHPublicKeyPath, *aksSSHPublicKeyPath)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			setDefaultParams()
			tc.setParams()
			err := checkParams()
			if tc.expectedError && err == nil {
				t.Errorf("expected an error but got no error")
			} else if !tc.expectedError && err != nil {
				t.Errorf("expected no error but got an error: %v", err)
			}
			if !tc.expectedError {
				tc.assertParams(t)
			}
		})
	}
}

func TestGetAzCredentials(t *testing.T) {
	// Prepare mock credentials
	tempAksCredentialsFile, err := ioutil.TempFile("", "azure.toml")
	if err != nil {
		t.Errorf("error when creating temp file: %v", err)
	}
	defer func() {
		err := os.Remove(tempAksCredentialsFile.Name())
		if err != nil {
			t.Errorf("error when removing temp file: %v", err)
		}
	}()
	_, err = tempAksCredentialsFile.Write([]byte(fakeCreds))
	if err != nil {
		t.Errorf("error when writing creds to temp file: %v", err)
	}

	setDefaultParams()
	*aksCredentialsFile = tempAksCredentialsFile.Name()
	c := Cluster{}
	err = c.getAzCredentials()
	if err != nil {
		t.Errorf("error when getting az credentials: %v", err)
	} else if c.credentials.ClientID != "df7269f2-xxxx-xxxx-xxxx-0f12a7d97404" {
		t.Errorf("expected client id to be df7269f2-xxxx-xxxx-xxxx-0f12a7d97404, but got %s", c.credentials.ClientID)
	} else if c.credentials.ClientSecret != "8c416dc5-xxxx-xxxx-xxxx-d77069e2a255" {
		t.Errorf("expected client secret to be 8c416dc5-xxxx-xxxx-xxxx-d77069e2a255, but got %s", c.credentials.ClientSecret)
	} else if c.credentials.TenantID != "72f988bf-xxxx-xxxx-xxxx-2d7cd011db47" {
		t.Errorf("expected tenant id to be 72f988bf-xxxx-xxxx-xxxx-2d7cd011db47, but got %s", c.credentials.TenantID)
	} else if c.credentials.SubscriptionID != "b9d2281e-xxxx-xxxx-xxxx-0d50377cdf76" {
		t.Errorf("expected subscription id to be b9d2281e-xxxx-xxxx-xxxx-0d50377cdf76, but got %s", c.credentials.SubscriptionID)
	} else if c.credentials.StorageAccountName != "TestStorageAccountName" {
		t.Errorf("expected storage account name to be TestStorageAccountName, but got %s", c.credentials.StorageAccountName)
	} else if c.credentials.StorageAccountKey != "TestStorageAccountKey" {
		t.Errorf("expected StorageAccountKey to be TestStorageAccountKey, but got %s", c.credentials.StorageAccountKey)
	}
}

func TestPopulateApiModelTemplate(t *testing.T) {
	// Prepare mock outputDir and api model template
	tempOutputDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Errorf("error when creating temp dir: %v", err)
	}
	defer func() {
		err := os.RemoveAll(tempOutputDir)
		if err != nil {
			t.Errorf("error when removing temp dir: %v", err)
		}
	}()

	tempAPIModelPath := path.Join(tempOutputDir, "kubernetes.json")
	err = ioutil.WriteFile(tempAPIModelPath, []byte(fakeAPIModelTemplate), 0644)
	if err != nil {
		t.Errorf("error when writing kubernetes.json: %v", err)
	}

	setDefaultParams()
	c := Cluster{
		outputDir:    tempOutputDir,
		apiModelPath: tempAPIModelPath,
		credentials: &Creds{
			ClientID:           "ClientID",
			ClientSecret:       "ClientSecret",
			TenantID:           "TenantID",
			SubscriptionID:     "SubscriptionID",
			StorageAccountName: "StorageAccountName",
			StorageAccountKey:  "StorageAccountKey",
		},
		location:                "location",
		name:                    "name",
		k8sVersion:              "k8sVersion",
		dnsPrefix:               "dnsPrefix",
		sshPublicKey:            "sshPublicKey",
		masterVMSize:            "masterVMSize",
		agentVMSize:             "agentVMSize",
		agentPoolCount:          10,
		adminUsername:           "adminUsername",
		aksCustomHyperKubeURL:   "aksCustomHyperKubeURL",
		aksCustomWinBinariesURL: "aksCustomWinBinariesURL",
		aksCustomCcmURL:         "aksCustomCcmURL",
	}
	err = c.populateApiModelTemplate()
	if err != nil {
		t.Errorf("error when populating api model template: %v", err)
	}

	apiModel, err := ioutil.ReadFile(tempAPIModelPath)
	if err != nil {
		t.Errorf("error when reading populated api model: %v", err)
	}
	if !areEqualJSON(string(apiModel), expectedAPIModel) {
		t.Errorf("expected %s to be %s", string(apiModel), expectedAPIModel)
	}
}

func setDefaultParams() {
	// Set acs flags to zero values to discourage tests from using them
	*acsResourceName = ""
	*acsResourceGroupName = ""
	*acsLocation = ""
	*acsMasterVmSize = ""
	*acsAgentVmSize = ""
	*acsAdminUsername = ""
	*acsAdminPassword = ""
	*acsAgentPoolCount = 0
	*acsTemplateURL = ""
	*acsDnsPrefix = ""
	*acsEngineURL = ""
	*acsEngineMD5 = ""
	*acsSSHPublicKeyPath = ""
	*acsWinBinaries = false
	*acsHyperKube = false
	*acsCcm = false
	*acsCredentialsFile = ""
	*acsOrchestratorRelease = ""
	*acsWinZipBuildScript = ""
	*acsNetworkPlugin = ""
	*acsAzureEnv = ""
	*acsIdentitySystem = ""
	*acsCustomCloudURL = ""

	*aksResourceName = "aksResourceName"
	*aksResourceGroupName = "aksResourceGroupName"
	*aksLocation = "aksLocation"
	*aksMasterVmSize = "aksMasterVmSize"
	*aksAgentVmSize = "aksAgentVmSize"
	*aksAdminUsername = "aksAdminUsername"
	*aksAdminPassword = "aksAdminPassword"
	*aksAgentPoolCount = 2
	*aksTemplateURL = "aksTemplateURL"
	*aksDnsPrefix = "aksDnsPrefix"
	*aksEngineURL = "aksEngineURL"
	*aksEngineMD5 = "aksEngineMD5"
	*aksSSHPublicKeyPath = "aksSSHPublicKeyPath"
	*aksWinBinaries = true
	*aksHyperKube = true
	*aksCcm = true
	*aksCredentialsFile = "aksCredentialsFile"
	*aksOrchestratorRelease = "aksOrchestratorRelease"
	*aksWinZipBuildScript = "aksWinZipBuildScript"
	*aksNetworkPlugin = "aksNetworkPlugin"
	*aksAzureEnv = "aksAzureEnv"
	*aksIdentitySystem = "aksIdentitySystem"
	*aksCustomCloudURL = "aksCustomCloudURL"
}

func areEqualJSON(s1, s2 string) bool {
	var o1 interface{}
	var o2 interface{}

	var err error
	err = json.Unmarshal([]byte(s1), &o1)
	if err != nil {
		return false
	}
	err = json.Unmarshal([]byte(s2), &o2)
	if err != nil {
		return false
	}

	return reflect.DeepEqual(o1, o2)
}
