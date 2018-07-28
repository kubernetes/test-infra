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
	_ "errors"
	_ "flag"
	_ "fmt"
	_ "os"
	_ "os/signal"
	_ "reflect"
	_ "syscall"
	_ "time"

	_ "k8s.io/test-infra/prow/apis/prowjobs/v1"
	_ "k8s.io/test-infra/prow/client/clientset/versioned"
	_ "k8s.io/test-infra/prow/client/clientset/versioned/scheme"
	_ "k8s.io/test-infra/prow/client/informers/externalversions"
	_ "k8s.io/test-infra/prow/client/informers/externalversions/prowjobs/v1"
	_ "k8s.io/test-infra/prow/client/listers/prowjobs/v1"
	_ "k8s.io/test-infra/prow/logrusutil"

	_ "github.com/knative/build/pkg/apis/build/v1alpha1"
	_ "github.com/knative/build/pkg/client/clientset/versioned"
	_ "github.com/knative/build/pkg/client/informers/externalversions"
	_ "github.com/knative/build/pkg/client/informers/externalversions/build/v1alpha1"
	_ "github.com/knative/build/pkg/client/listers/build/v1alpha1"

	_ "k8s.io/api/core/v1"
	_ "k8s.io/apimachinery/pkg/api/errors"
	_ "k8s.io/apimachinery/pkg/apis/meta/v1"
	_ "k8s.io/apimachinery/pkg/runtime/schema"
	_ "k8s.io/apimachinery/pkg/util/runtime"
	_ "k8s.io/apimachinery/pkg/util/wait"
	_ "k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/kubernetes/typed/core/v1"
	_ "k8s.io/client-go/tools/cache"
	_ "k8s.io/client-go/tools/clientcmd"
	_ "k8s.io/client-go/tools/record"
	_ "k8s.io/client-go/util/workqueue"

	"github.com/sirupsen/logrus"
)

func main() {
	logrus.Fatalf("see next pr")
}
