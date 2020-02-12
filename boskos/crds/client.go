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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	group   = "boskos.k8s.io"
	version = "v1"
)

// KubernetesClientOptions are flag options used to create a kube client.
type KubernetesClientOptions struct {
	inMemory   bool
	kubeConfig string
	namespace  string
}

// AddFlags adds kube client flags to existing FlagSet.
func (o *KubernetesClientOptions) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.namespace, "namespace", v1.NamespaceDefault, "namespace to install on")
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
func (o *KubernetesClientOptions) Client(t Type) (ClientInterface, error) {
	if o.inMemory {
		return newDummyClient(t), nil
	}
	return o.newCRDClient(t)
}

// newClientFromFlags creates a CRD rest client from provided flags.
func (o *KubernetesClientOptions) newCRDClient(t Type) (*Client, error) {
	config, scheme, err := createRESTConfig(o.kubeConfig, t)
	if err != nil {
		return nil, err
	}

	if err = registerResource(config, t); err != nil {
		return nil, err
	}
	// creates the client
	var restClient *rest.RESTClient
	restClient, err = rest.RESTClientFor(config)
	if err != nil {
		return nil, err
	}
	rc := Client{cl: restClient, ns: o.namespace, t: t,
		codec: runtime.NewParameterCodec(scheme)}
	return &rc, nil
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

// createRESTConfig for cluster API server, pass empty config file for in-cluster
func createRESTConfig(kubeconfig string, t Type) (config *rest.Config, types *runtime.Scheme, err error) {
	if kubeconfig == "" {
		config, err = rest.InClusterConfig()
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	}

	if err != nil {
		return
	}

	version := schema.GroupVersion{
		Group:   group,
		Version: version,
	}

	config.GroupVersion = &version
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON

	types = runtime.NewScheme()
	schemeBuilder := runtime.NewSchemeBuilder(
		func(scheme *runtime.Scheme) error {
			scheme.AddKnownTypes(version, t.Object, t.Collection)
			v1.AddToGroupVersion(scheme, version)
			return nil
		})
	err = schemeBuilder.AddToScheme(types)
	config.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: serializer.NewCodecFactory(types)}

	return
}

// registerResource sends a request to create CRDs and waits for them to initialize
func registerResource(config *rest.Config, t Type) error {
	c, err := apiextensionsclient.NewForConfig(config)
	if err != nil {
		return err
	}

	crd := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name: fmt.Sprintf("%s.%s", t.Plural, group),
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   group,
			Version: version,
			Scope:   apiextensionsv1beta1.NamespaceScoped,
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Singular: t.Singular,
				Plural:   t.Plural,
				Kind:     t.Kind,
				ListKind: t.ListKind,
			},
		},
	}
	if _, err := c.ApiextensionsV1beta1().CustomResourceDefinitions().Create(crd); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

// newDummyClient creates a in memory client representation for testing, such that we do not need to use a kubernetes API Server.
func newDummyClient(t Type) *dummyClient {
	c := &dummyClient{
		t:       t,
		objects: make(map[string]Object),
	}
	return c
}

// ClientInterface is used for testing.
type ClientInterface interface {
	// NewObject instantiates a new object of the type supported by the client
	NewObject() Object
	// NewCollection instantiates a new collection of the type supported by the client
	NewCollection() Collection
	// Create a new object
	Create(obj Object) (Object, error)
	// Update an existing object, fails if object does not exist
	Update(obj Object) (Object, error)
	// Delete an existing object, fails if objects does not exist
	Delete(name string, options *v1.DeleteOptions) error
	// Get an existing object
	Get(name string) (Object, error)
	// LIst existing objects
	List(opts v1.ListOptions) (Collection, error)
}

// dummyClient is used for testing purposes
type dummyClient struct {
	objects map[string]Object
	t       Type
}

// NewObject implements ClientInterface
func (c *dummyClient) NewObject() Object {
	return c.t.Object.DeepCopyObject().(Object)
}

// NewCollection implements ClientInterface
func (c *dummyClient) NewCollection() Collection {
	return c.t.Collection.DeepCopyObject().(Collection)
}

// Create implements ClientInterface
func (c *dummyClient) Create(obj Object) (Object, error) {
	c.objects[obj.GetName()] = obj
	return obj, nil
}

// Update implements ClientInterface
func (c *dummyClient) Update(obj Object) (Object, error) {
	_, ok := c.objects[obj.GetName()]
	if !ok {
		return nil, fmt.Errorf("cannot find object %s", obj.GetName())
	}
	c.objects[obj.GetName()] = obj
	return obj, nil
}

// Delete implements ClientInterface
func (c *dummyClient) Delete(name string, options *v1.DeleteOptions) error {
	_, ok := c.objects[name]
	if ok {
		delete(c.objects, name)
		return nil
	}
	return fmt.Errorf("%s does not exist", name)
}

// Get implements ClientInterface
func (c *dummyClient) Get(name string) (Object, error) {
	obj, ok := c.objects[name]
	if ok {
		return obj, nil
	}
	return nil, fmt.Errorf("could not find %s", name)
}

// List implements ClientInterface
func (c *dummyClient) List(opts v1.ListOptions) (Collection, error) {
	var items []Object
	for _, i := range c.objects {
		items = append(items, i)
	}
	r := c.NewCollection()
	r.SetItems(items)
	return r, nil
}

// Client implements a true CRD rest client
type Client struct {
	cl    *rest.RESTClient
	ns    string
	t     Type
	codec runtime.ParameterCodec
}

// NewObject implements ClientInterface
func (c *Client) NewObject() Object {
	return c.t.Object.DeepCopyObject().(Object)
}

// NewCollection implements ClientInterface
func (c *Client) NewCollection() Collection {
	return c.t.Collection.DeepCopyObject().(Collection)
}

// Create implements ClientInterface
func (c *Client) Create(obj Object) (Object, error) {
	result := c.NewObject()
	err := c.cl.Post().
		Namespace(c.ns).
		Resource(c.t.Plural).
		Name(obj.GetName()).
		Body(obj).
		Do().
		Into(result)
	return result, err
}

// Update implements ClientInterface
func (c *Client) Update(obj Object) (Object, error) {
	result := c.NewObject()
	err := c.cl.Put().
		Namespace(c.ns).
		Resource(c.t.Plural).
		Body(obj).
		Name(obj.GetName()).
		Do().
		Into(result)
	return result, err
}

// Delete implements ClientInterface
func (c *Client) Delete(name string, options *v1.DeleteOptions) error {
	return c.cl.Delete().
		Namespace(c.ns).
		Resource(c.t.Plural).
		Name(name).
		Body(options).
		Do().
		Error()
}

// Get implements ClientInterface
func (c *Client) Get(name string) (Object, error) {
	result := c.NewObject()
	err := c.cl.Get().
		Namespace(c.ns).
		Resource(c.t.Plural).
		Name(name).
		Do().
		Into(result)
	return result, err
}

// List implements ClientInterface
func (c *Client) List(opts v1.ListOptions) (Collection, error) {
	result := c.NewCollection()
	err := c.cl.Get().
		Namespace(c.ns).
		Resource(c.t.Plural).
		VersionedParams(&opts, c.codec).
		Do().
		Into(result)
	return result, err
}
