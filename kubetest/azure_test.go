package main

import (
	"testing"
)

var deploytMethodMap = map[aksDeploymentMethod]string{
	normal:              "normal",
	customHyperkube:     "custom hyperkube",
	customK8sComponents: "custom k8s components",
}

func TestGetDeploymentMethod(t *testing.T) {
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
			expectedAKSDeploytmentMethod: normal,
		},
		{
			desc:                         "k8s 1.17 without custom k8s",
			k8sRelease:                   "1.17",
			customK8s:                    false,
			expectedAKSDeploytmentMethod: normal,
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
			expectedAKSDeploytmentMethod: normal,
		},
		{
			desc:                         "using an invalid k8s version",
			k8sRelease:                   "invalid",
			customK8s:                    true,
			expectedAKSDeploytmentMethod: normal,
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
