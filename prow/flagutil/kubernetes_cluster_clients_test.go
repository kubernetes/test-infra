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

package flagutil

import (
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	admissionregistrationv1alpha1 "k8s.io/client-go/kubernetes/typed/admissionregistration/v1alpha1"
	admissionregistrationv1beta1 "k8s.io/client-go/kubernetes/typed/admissionregistration/v1beta1"
	appsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	appsv1beta1 "k8s.io/client-go/kubernetes/typed/apps/v1beta1"
	appsv1beta2 "k8s.io/client-go/kubernetes/typed/apps/v1beta2"
	authenticationv1 "k8s.io/client-go/kubernetes/typed/authentication/v1"
	authenticationv1beta1 "k8s.io/client-go/kubernetes/typed/authentication/v1beta1"
	authorizationv1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
	authorizationv1beta1 "k8s.io/client-go/kubernetes/typed/authorization/v1beta1"
	autoscalingv1 "k8s.io/client-go/kubernetes/typed/autoscaling/v1"
	autoscalingv2beta1 "k8s.io/client-go/kubernetes/typed/autoscaling/v2beta1"
	autoscalingv2beta2 "k8s.io/client-go/kubernetes/typed/autoscaling/v2beta2"
	batchv1 "k8s.io/client-go/kubernetes/typed/batch/v1"
	batchv1beta1 "k8s.io/client-go/kubernetes/typed/batch/v1beta1"
	batchv2alpha1 "k8s.io/client-go/kubernetes/typed/batch/v2alpha1"
	certificatesv1beta1 "k8s.io/client-go/kubernetes/typed/certificates/v1beta1"
	coordinationv1beta1 "k8s.io/client-go/kubernetes/typed/coordination/v1beta1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	eventsv1beta1 "k8s.io/client-go/kubernetes/typed/events/v1beta1"
	extensionsv1beta1 "k8s.io/client-go/kubernetes/typed/extensions/v1beta1"
	networkingv1 "k8s.io/client-go/kubernetes/typed/networking/v1"
	policyv1beta1 "k8s.io/client-go/kubernetes/typed/policy/v1beta1"
	rbacv1 "k8s.io/client-go/kubernetes/typed/rbac/v1"
	rbacv1alpha1 "k8s.io/client-go/kubernetes/typed/rbac/v1alpha1"
	rbacv1beta1 "k8s.io/client-go/kubernetes/typed/rbac/v1beta1"
	schedulingv1alpha1 "k8s.io/client-go/kubernetes/typed/scheduling/v1alpha1"
	schedulingv1beta1 "k8s.io/client-go/kubernetes/typed/scheduling/v1beta1"
	settingsv1alpha1 "k8s.io/client-go/kubernetes/typed/settings/v1alpha1"
	storagev1 "k8s.io/client-go/kubernetes/typed/storage/v1"
	storagev1alpha1 "k8s.io/client-go/kubernetes/typed/storage/v1alpha1"
	storagev1beta1 "k8s.io/client-go/kubernetes/typed/storage/v1beta1"
	"k8s.io/test-infra/prow/kube"

	"k8s.io/test-infra/pkg/flagutil"
)

func TestKubernetesOptions_Validate(t *testing.T) {
	var testCases = []struct {
		name        string
		dryRun      bool
		kubernetes  flagutil.OptionGroup
		expectedErr bool
	}{
		{
			name:        "all ok without dry-run",
			dryRun:      false,
			kubernetes:  &KubernetesOptions{},
			expectedErr: false,
		},
		{
			name:   "all ok with dry-run",
			dryRun: true,
			kubernetes: &KubernetesOptions{
				deckURI: "https://example.com",
			},
			expectedErr: false,
		},
		{
			name:        "missing deck endpoint with dry-run",
			dryRun:      true,
			kubernetes:  &KubernetesOptions{},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.kubernetes.Validate(testCase.dryRun)
			if testCase.expectedErr && err == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && err != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
			}
		})
	}
}

type trackableKubernetesInterface struct {
	name string
}

func (*trackableKubernetesInterface) Discovery() discovery.DiscoveryInterface { return nil }
func (*trackableKubernetesInterface) AdmissionregistrationV1alpha1() admissionregistrationv1alpha1.AdmissionregistrationV1alpha1Interface {
	return nil
}
func (*trackableKubernetesInterface) AdmissionregistrationV1beta1() admissionregistrationv1beta1.AdmissionregistrationV1beta1Interface {
	return nil
}
func (*trackableKubernetesInterface) Admissionregistration() admissionregistrationv1beta1.AdmissionregistrationV1beta1Interface {
	return nil
}
func (*trackableKubernetesInterface) AppsV1beta1() appsv1beta1.AppsV1beta1Interface { return nil }
func (*trackableKubernetesInterface) AppsV1beta2() appsv1beta2.AppsV1beta2Interface { return nil }
func (*trackableKubernetesInterface) AppsV1() appsv1.AppsV1Interface                { return nil }
func (*trackableKubernetesInterface) Apps() appsv1.AppsV1Interface                  { return nil }
func (*trackableKubernetesInterface) AuthenticationV1() authenticationv1.AuthenticationV1Interface {
	return nil
}
func (*trackableKubernetesInterface) Authentication() authenticationv1.AuthenticationV1Interface {
	return nil
}
func (*trackableKubernetesInterface) AuthenticationV1beta1() authenticationv1beta1.AuthenticationV1beta1Interface {
	return nil
}
func (*trackableKubernetesInterface) AuthorizationV1() authorizationv1.AuthorizationV1Interface {
	return nil
}
func (*trackableKubernetesInterface) Authorization() authorizationv1.AuthorizationV1Interface {
	return nil
}
func (*trackableKubernetesInterface) AuthorizationV1beta1() authorizationv1beta1.AuthorizationV1beta1Interface {
	return nil
}
func (*trackableKubernetesInterface) AutoscalingV1() autoscalingv1.AutoscalingV1Interface { return nil }
func (*trackableKubernetesInterface) Autoscaling() autoscalingv1.AutoscalingV1Interface   { return nil }
func (*trackableKubernetesInterface) AutoscalingV2beta1() autoscalingv2beta1.AutoscalingV2beta1Interface {
	return nil
}
func (*trackableKubernetesInterface) AutoscalingV2beta2() autoscalingv2beta2.AutoscalingV2beta2Interface {
	return nil
}
func (*trackableKubernetesInterface) BatchV1() batchv1.BatchV1Interface                   { return nil }
func (*trackableKubernetesInterface) Batch() batchv1.BatchV1Interface                     { return nil }
func (*trackableKubernetesInterface) BatchV1beta1() batchv1beta1.BatchV1beta1Interface    { return nil }
func (*trackableKubernetesInterface) BatchV2alpha1() batchv2alpha1.BatchV2alpha1Interface { return nil }
func (*trackableKubernetesInterface) CertificatesV1beta1() certificatesv1beta1.CertificatesV1beta1Interface {
	return nil
}
func (*trackableKubernetesInterface) Certificates() certificatesv1beta1.CertificatesV1beta1Interface {
	return nil
}
func (*trackableKubernetesInterface) CoordinationV1beta1() coordinationv1beta1.CoordinationV1beta1Interface {
	return nil
}
func (*trackableKubernetesInterface) Coordination() coordinationv1beta1.CoordinationV1beta1Interface {
	return nil
}
func (*trackableKubernetesInterface) CoreV1() corev1.CoreV1Interface                      { return nil }
func (*trackableKubernetesInterface) Core() corev1.CoreV1Interface                        { return nil }
func (*trackableKubernetesInterface) EventsV1beta1() eventsv1beta1.EventsV1beta1Interface { return nil }
func (*trackableKubernetesInterface) Events() eventsv1beta1.EventsV1beta1Interface        { return nil }
func (*trackableKubernetesInterface) ExtensionsV1beta1() extensionsv1beta1.ExtensionsV1beta1Interface {
	return nil
}
func (*trackableKubernetesInterface) Extensions() extensionsv1beta1.ExtensionsV1beta1Interface {
	return nil
}
func (*trackableKubernetesInterface) NetworkingV1() networkingv1.NetworkingV1Interface    { return nil }
func (*trackableKubernetesInterface) Networking() networkingv1.NetworkingV1Interface      { return nil }
func (*trackableKubernetesInterface) PolicyV1beta1() policyv1beta1.PolicyV1beta1Interface { return nil }
func (*trackableKubernetesInterface) Policy() policyv1beta1.PolicyV1beta1Interface        { return nil }
func (*trackableKubernetesInterface) RbacV1() rbacv1.RbacV1Interface                      { return nil }
func (*trackableKubernetesInterface) Rbac() rbacv1.RbacV1Interface                        { return nil }
func (*trackableKubernetesInterface) RbacV1beta1() rbacv1beta1.RbacV1beta1Interface       { return nil }
func (*trackableKubernetesInterface) RbacV1alpha1() rbacv1alpha1.RbacV1alpha1Interface    { return nil }
func (*trackableKubernetesInterface) SchedulingV1alpha1() schedulingv1alpha1.SchedulingV1alpha1Interface {
	return nil
}
func (*trackableKubernetesInterface) SchedulingV1beta1() schedulingv1beta1.SchedulingV1beta1Interface {
	return nil
}
func (*trackableKubernetesInterface) Scheduling() schedulingv1beta1.SchedulingV1beta1Interface {
	return nil
}
func (*trackableKubernetesInterface) SettingsV1alpha1() settingsv1alpha1.SettingsV1alpha1Interface {
	return nil
}
func (*trackableKubernetesInterface) Settings() settingsv1alpha1.SettingsV1alpha1Interface { return nil }
func (*trackableKubernetesInterface) StorageV1beta1() storagev1beta1.StorageV1beta1Interface {
	return nil
}
func (*trackableKubernetesInterface) StorageV1() storagev1.StorageV1Interface { return nil }
func (*trackableKubernetesInterface) Storage() storagev1.StorageV1Interface   { return nil }
func (*trackableKubernetesInterface) StorageV1alpha1() storagev1alpha1.StorageV1alpha1Interface {
	return nil
}

func TestContextsToAliases(t *testing.T) {
	var testCases = []struct {
		name             string
		clientsByContext map[string]kubernetes.Interface
		infraContext     string
		clientsByAlias   map[string]kubernetes.Interface
	}{
		{
			name: "no literal default provided means that the infra context becomes default alias",
			clientsByContext: map[string]kubernetes.Interface{
				"first": &trackableKubernetesInterface{name: "first"},
				"infra": &trackableKubernetesInterface{name: "infra"},
			},
			infraContext: "infra",
			clientsByAlias: map[string]kubernetes.Interface{
				"first":                  &trackableKubernetesInterface{name: "first"},
				kube.DefaultClusterAlias: &trackableKubernetesInterface{name: "infra"},
			},
		},
		{
			name: "literal default provided means that there is no change",
			clientsByContext: map[string]kubernetes.Interface{
				"first":                  &trackableKubernetesInterface{name: "first"},
				kube.DefaultClusterAlias: &trackableKubernetesInterface{name: "infra"},
			},
			infraContext: "infra",
			clientsByAlias: map[string]kubernetes.Interface{
				"first":                  &trackableKubernetesInterface{name: "first"},
				kube.DefaultClusterAlias: &trackableKubernetesInterface{name: "infra"},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if actual, expected := contextsToAliases(testCase.clientsByContext, testCase.infraContext), testCase.clientsByAlias; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect clients by alias: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}
