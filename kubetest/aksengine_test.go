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
	"io/ioutil"
	"path"
	"testing"
)

func TestGetDeploymentMethod(t *testing.T) {
	var deploytMethodMap = map[aksDeploymentMethod]string{
		noop:                "noop",
		customHyperkube:     "custom hyperkube",
		customK8sComponents: "custom k8s components",
	}
	testCases := []struct {
		desc                         string
		k8sRelease                   string
		customK8s                    bool
		expectedAKSDeploytmentMethod aksDeploymentMethod
	}{
		{
			desc:                         "k8s 1.16 without custom k8s",
			k8sRelease:                   "1.16",
			customK8s:                    false,
			expectedAKSDeploytmentMethod: noop,
		},
		{
			desc:                         "k8s 1.17 without custom k8s",
			k8sRelease:                   "1.17",
			customK8s:                    false,
			expectedAKSDeploytmentMethod: noop,
		},
		{
			desc:                         "k8s 1.16 with custom k8s",
			k8sRelease:                   "1.16",
			customK8s:                    true,
			expectedAKSDeploytmentMethod: customHyperkube,
		},
		{
			desc:                         "k8s 1.17 with custom k8s",
			k8sRelease:                   "1.17",
			customK8s:                    true,
			expectedAKSDeploytmentMethod: customK8sComponents,
		},
		{
			desc:                         "using k8s release instead of k8s version",
			k8sRelease:                   "1.17.0",
			customK8s:                    true,
			expectedAKSDeploytmentMethod: noop,
		},
		{
			desc:                         "using an invalid k8s version",
			k8sRelease:                   "invalid",
			customK8s:                    true,
			expectedAKSDeploytmentMethod: noop,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			if tc.customK8s {
				aksDeployCustomK8s = boolPointer(true)
			} else {
				aksDeployCustomK8s = boolPointer(false)
			}
			aksDeploymentMethod := getAKSDeploymentMethod(tc.k8sRelease)
			if aksDeploymentMethod != tc.expectedAKSDeploytmentMethod {
				t.Fatalf("Expected '%s' deployment method, but got '%s'", deploytMethodMap[tc.expectedAKSDeploytmentMethod], deploytMethodMap[aksDeploymentMethod])
			}
		})
	}
}

func TestStrictJSON(t *testing.T) {
	testCases := []struct {
		name          string
		apiModel      string
		expectedError bool
	}{
		{
			name: "API model with no unknown fields",
			apiModel: `
{
    "apiVersion": "vlabs",
    "properties": {
        "orchestratorProfile": {
            "orchestratorType": "Kubernetes",
			"orchestratorRelease": "",
			"kubernetesConfig": {
				"containerRuntime": "containerd"
			}
        },
        "masterProfile": {
            "count": 1,
            "dnsPrefix": "",
            "vmSize": "Standard_D2_v3"
        },
        "agentPoolProfiles": [
            {
                "name": "agentpool1",
                "count": 2,
                "vmSize": "Standard_D2s_v3",
                "availabilityProfile": "AvailabilitySet"
            }
        ],
        "linuxProfile": {
            "adminUsername": "azureuser",
            "ssh": {
                "publicKeys": [
                    {
                        "keyData": ""
                    }
                ]
            }
        },
        "servicePrincipalProfile": {
            "clientID": "",
            "secret": ""
        }
    }
}
			`,
			expectedError: false,
		},
		{
			name: "API model with nested kubernetesConfig",
			apiModel: `
{
    "apiVersion": "vlabs",
    "properties": {
        "orchestratorProfile": {
            "orchestratorType": "Kubernetes",
			"orchestratorRelease": "",
			"kubernetesConfig": {
				"kubernetesConfig": {
					"containerRuntime": "containerd"
				}
			}
        },
        "masterProfile": {
            "count": 1,
            "dnsPrefix": "",
            "vmSize": "Standard_D2_v3"
        },
        "agentPoolProfiles": [
            {
                "name": "agentpool1",
                "count": 2,
                "vmSize": "Standard_D2s_v3",
                "availabilityProfile": "AvailabilitySet"
            }
        ],
        "linuxProfile": {
            "adminUsername": "azureuser",
            "ssh": {
                "publicKeys": [
                    {
                        "keyData": ""
                    }
                ]
            }
        },
        "servicePrincipalProfile": {
            "clientID": "",
            "secret": ""
        }
    }
}
			`,
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tempDir, err := ioutil.TempDir("", "")
			if err != nil {
				t.Fatalf("failed to create a temporary directory: %v", err)
			}
			// Write tc.apiModel to a file so aksEngineDeployer can read it
			if err := ioutil.WriteFile(path.Join(tempDir, "kubernetes.json"), []byte(tc.apiModel), 0644); err != nil {
				t.Fatalf("failed to write kubernetes.json: %v", err)
			}
			c := getMockAKSEngineDeployer(tempDir)
			err = c.populateAPIModelTemplate()
			if tc.expectedError && err == nil {
				t.Fatal("expected populateAPIModelTemplate to return an error, but got no error")
			} else if !tc.expectedError && err != nil {
				t.Fatalf("expected populateAPIModelTemplate not to return an error, but got an error: %v", err)
			}
		})
	}
}

func getMockAKSEngineDeployer(outputDir string) aksEngineDeployer {
	// Populate bare minimum fields
	return aksEngineDeployer{
		outputDir:    outputDir,
		apiModelPath: "apiModelPath",
		credentials:  &Creds{},
	}
}
