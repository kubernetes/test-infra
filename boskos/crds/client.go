/*
Copyright 2017 The Kubernetes Authors.

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

package crds

import (
	"flag"
	"fmt"
	"os"

	"k8s.io/test-infra/boskos/common"

	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// KubernetesClientOptions are flag options used to create a kube client.
type KubernetesClientOptions struct {
	inMemory   bool
	kubeConfig string
	namespace  string
}

// AddFlags adds kube client flags to existing FlagSet.
func (o *KubernetesClientOptions) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.kubeConfig, "kubeconfig", "", "absolute path to the kubeConfig file")
	fs.BoolVar(&o.inMemory, "in_memory", false, "Use in memory client instead of CRD")
}

// Validate validates Kubernetes client options.
func (o *KubernetesClientOptions) Validate() error {
	if o.kubeConfig != "" {
		if _, err := os.Stat(o.kubeConfig); err != nil {
			return err
		}
	}
	return nil
}

// Client returns a ClientInterface based on the flags provided.
func (o *KubernetesClientOptions) Client() (ctrlruntimeclient.Client, error) {
	if o.inMemory {
		return fakectrlruntimeclient.NewFakeClient(), nil
	}

	var config *rest.Config
	var err error
	if o.kubeConfig == "" {
		config, err = rest.InClusterConfig()
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", o.kubeConfig)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to construct rest config: %v", err)
	}

	if err = registerResources(config); err != nil {
		return nil, fmt.Errorf("failed to create CRDs: %v", err)
	}

	return ctrlruntimeclient.New(config, ctrlruntimeclient.Options{})
}

// Type defines a Custom Resource Definition (CRD) Type.
type Type struct {
	Kind, ListKind   string
	Singular, Plural string
	Object           Object
	Collection       Collection
}

// Object extends the runtime.Object interface. CRD are just a representation of the actual boskos object
// which should implements the common.Item interface.
type Object interface {
	runtime.Object
	GetName() string
	FromItem(item common.Item)
	ToItem() common.Item
}

// Collection is a list of Object interface.
type Collection interface {
	runtime.Object
	SetItems([]Object)
	GetItems() []Object
}

// registerResources sends a request to create CRDs
func registerResources(config *rest.Config) error {
	c, err := apiextensionsclient.NewForConfig(config)
	if err != nil {
		return err
	}

	resourceCRD := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name: fmt.Sprintf("%s.%s", ResourceType.Plural, group),
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   group,
			Version: version,
			Scope:   apiextensionsv1beta1.NamespaceScoped,
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Singular: ResourceType.Singular,
				Plural:   ResourceType.Plural,
				Kind:     ResourceType.Kind,
				ListKind: ResourceType.ListKind,
			},
		},
	}
	if _, err := c.ApiextensionsV1beta1().CustomResourceDefinitions().Create(resourceCRD); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	dlrcCRD := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name: fmt.Sprintf("%s.%s", DRLCType.Plural, group),
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   group,
			Version: version,
			Scope:   apiextensionsv1beta1.NamespaceScoped,
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Singular: DRLCType.Singular,
				Plural:   DRLCType.Plural,
				Kind:     DRLCType.Kind,
				ListKind: DRLCType.ListKind,
			},
		},
	}
	if _, err := c.ApiextensionsV1beta1().CustomResourceDefinitions().Create(dlrcCRD); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}
