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
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"k8s.io/test-infra/prow/interrupts"
)

// KubernetesClientOptions are flag options used to create a kube client.
type KubernetesClientOptions struct {
	inMemory   bool
	kubeConfig string
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

	cfg, err := o.Cfg()
	if err != nil {
		return nil, err
	}

	return ctrlruntimeclient.New(cfg, ctrlruntimeclient.Options{})
}

// CacheBackedClient returns a client whose Reader is cache backed. Namespace can be empty
// in which case the client will use all namespaces.
// It blocks until the cache was synced for all types passed in startCacheFor.
func (o *KubernetesClientOptions) CacheBackedClient(namespace string, startCacheFor ...runtime.Object) (ctrlruntimeclient.Client, error) {
	if o.inMemory {
		return fakectrlruntimeclient.NewFakeClient(), nil
	}

	cfg, err := o.Cfg()
	if err != nil {
		return nil, err
	}
	cfg.QPS = 100
	cfg.Burst = 200

	mgr, err := manager.New(cfg, manager.Options{
		LeaderElection:     false,
		Namespace:          namespace,
		MetricsBindAddress: "0",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to construct manager: %v", err)
	}

	// Allocate an informer so our cache actually waits for these types to
	// be synced. Must be done before we start the mgr, else this may block
	// indefinitely if there is an issue.
	for _, t := range startCacheFor {
		if _, err := mgr.GetCache().GetInformer(t); err != nil {
			return nil, fmt.Errorf("failed to get informer for type %T: %v", t, err)
		}
	}

	interrupts.Run(func(ctx context.Context) {
		// Exiting like this is not nice, but the interrupts package
		// doesn't allow us to stop the app. Furthermore, the behaviour
		// of the reading client is undefined after the manager stops,
		// so we should bail ASAP.
		if err := mgr.Start(ctx.Done()); err != nil {
			logrus.WithError(err).Fatal("Mgr failed.")
		}
		logrus.Info("Mgr finished gracefully.")
		os.Exit(0)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	startSyncTime := time.Now()
	if synced := mgr.GetCache().WaitForCacheSync(ctx.Done()); !synced {
		return nil, errors.New("timeout waiting for cache sync")
	}
	logrus.WithField("sync-duration", time.Since(startSyncTime).String()).Info("Cache synced")

	return mgr.GetClient(), nil
}

// Cfg returns the *rest.Config for the configured cluster
func (o *KubernetesClientOptions) Cfg() (*rest.Config, error) {
	var cfg *rest.Config
	var err error
	if o.kubeConfig == "" {
		cfg, err = rest.InClusterConfig()
	} else {
		cfg, err = clientcmd.BuildConfigFromFlags("", o.kubeConfig)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to construct rest config: %v", err)
	}

	return cfg, nil
}

// Type defines a Custom Resource Definition (CRD) Type.
type Type struct {
	Kind, ListKind   string
	Singular, Plural string
	Object           runtime.Object
	Collection       runtime.Object
}
