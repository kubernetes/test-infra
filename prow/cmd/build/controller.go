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
	"errors"
	"fmt"
	"strings"
	"time"

	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pod-utils/clone"
	"k8s.io/test-infra/prow/pod-utils/decorate"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"

	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	prowjobset "k8s.io/test-infra/prow/client/clientset/versioned"
	prowjobscheme "k8s.io/test-infra/prow/client/clientset/versioned/scheme"
	prowjobinfov1 "k8s.io/test-infra/prow/client/informers/externalversions/prowjobs/v1"
	prowjoblisters "k8s.io/test-infra/prow/client/listers/prowjobs/v1"

	buildv1alpha1 "github.com/knative/build/pkg/apis/build/v1alpha1"

	untypedcorev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	"github.com/sirupsen/logrus"
)

const (
	controllerName = "prow-build-crd"
)

type controller struct {
	pjc    prowjobset.Interface
	builds map[string]buildConfig
	totURL string

	pjLister   prowjoblisters.ProwJobLister
	pjInformer cache.SharedIndexInformer

	workqueue workqueue.RateLimitingInterface

	recorder record.EventRecorder
}

// hasSynced returns true when every prowjob and build informer has synced.
func (c *controller) hasSynced() bool {
	if !c.pjInformer.HasSynced() {
		return false // still syncing prowjobs
	}
	for _, cfg := range c.builds {
		if !cfg.informer.Informer().HasSynced() {
			return false // still syncing builds in at least one cluster
		}
	}
	return true // Everyone is synced
}

func newController(kc kubernetes.Interface, pjc prowjobset.Interface, pji prowjobinfov1.ProwJobInformer, buildConfigs map[string]buildConfig, totURL string) *controller {
	// Log to events
	prowjobscheme.AddToScheme(scheme.Scheme)
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	eventBroadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: kc.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, untypedcorev1.EventSource{Component: controllerName})

	// Create struct
	c := &controller{
		pjc:        pjc,
		builds:     buildConfigs,
		pjLister:   pji.Lister(),
		pjInformer: pji.Informer(),
		workqueue:  workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), controllerName),
		recorder:   recorder,
		totURL:     totURL,
	}

	logrus.Info("Setting up event handlers")

	// Reconcile whenever a prowjob changes
	pji.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pj, ok := obj.(prowjobv1.ProwJob)
			if !ok {
				logrus.Warnf("Ignoring bad prowjob add: %v", obj)
				return
			}
			c.enqueueKey(pj.Spec.Cluster, pj)
		},
		UpdateFunc: func(old, new interface{}) {
			pj, ok := new.(prowjobv1.ProwJob)
			if !ok {
				logrus.Warnf("Ignoring bad prowjob update: %v", new)
				return
			}
			c.enqueueKey(pj.Spec.Cluster, pj)
		},
		DeleteFunc: func(obj interface{}) {
			pj, ok := obj.(prowjobv1.ProwJob)
			if !ok {
				logrus.Warnf("Ignoring bad prowjob delete: %v", obj)
				return
			}
			c.enqueueKey(pj.Spec.Cluster, pj)
		},
	})

	for ctx, cfg := range buildConfigs {
		// Reconcile whenever a build changes.
		cfg.informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				c.enqueueKey(ctx, obj)
			},
			UpdateFunc: func(old, new interface{}) {
				c.enqueueKey(ctx, new)
			},
		})
	}

	return c
}

// Run starts threads workers, returning after receiving a stop signal.
func (c *controller) Run(threads int, stop <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer c.workqueue.ShutDown()

	logrus.Info("Starting Build controller")
	logrus.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stop, c.hasSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	logrus.Info("Starting workers")
	for i := 0; i < threads; i++ {
		go wait.Until(c.runWorker, time.Second, stop)
	}

	logrus.Info("Started workers")
	<-stop
	logrus.Info("Shutting down workers")
	return nil
}

// runWorker dequeues to reconcile, until the queue has closed.
func (c *controller) runWorker() {
	for {
		key, shutdown := c.workqueue.Get()
		if shutdown {
			return
		}
		func() {
			defer c.workqueue.Done(key)

			if err := reconcile(c, key.(string)); err != nil {
				runtime.HandleError(fmt.Errorf("failed to reconcile %s: %v", key, err))
				return // Do not forget so we retry later.
			}
			c.workqueue.Forget(key)
		}()
	}
}

// toKey returns context/namespace/name
func toKey(ctx string, obj metav1.ObjectMeta) string {
	return strings.Join([]string{ctx, obj.Namespace, obj.Name}, "/")
}

// fromKey converts toKey back into its parts
func fromKey(key string) (string, string, string, error) {
	parts := strings.Split(key, "/")
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("bad key: %q", key)
	}
	return parts[0], parts[1], parts[2], nil
}

// enqueueKey schedules an item for reconciliation.
func (c *controller) enqueueKey(ctx string, obj interface{}) {
	var meta metav1.ObjectMeta
	switch o := obj.(type) {
	case prowjobv1.ProwJob:
		meta = o.ObjectMeta
	case buildv1alpha1.Build:
		meta = o.ObjectMeta
	default:
		logrus.Warnf("cannot enqueue unknown type %v: %v", o, obj)
		return
	}

	c.workqueue.AddRateLimited(toKey(ctx, meta))
}

type reconciler interface {
	getProwJob(namespace, name string) (*prowjobv1.ProwJob, error)
	getBuild(context, namespace, name string) (*buildv1alpha1.Build, error)
	deleteBuild(context, namespace, name string) error
	createBuild(context, namespace string, b *buildv1alpha1.Build) (*buildv1alpha1.Build, error)
	updateProwJob(namespace string, pj *prowjobv1.ProwJob) (*prowjobv1.ProwJob, error)
	now() metav1.Time
	buildID(prowjobv1.ProwJob) (string, error)
}

func (c *controller) getProwJob(namespace, name string) (*prowjobv1.ProwJob, error) {
	return c.pjLister.ProwJobs(namespace).Get(name)
}

func (c *controller) getBuild(context, namespace, name string) (*buildv1alpha1.Build, error) {
	b, ok := c.builds[context]
	if !ok {
		return nil, errors.New("context not found")
	}
	return b.informer.Lister().Builds(namespace).Get(name)
}
func (c *controller) deleteBuild(context, namespace, name string) error {
	b, ok := c.builds[context]
	if !ok {
		return errors.New("context not found")
	}
	return b.client.BuildV1alpha1().Builds(namespace).Delete(name, &metav1.DeleteOptions{})
}
func (c *controller) createBuild(context, namespace string, b *buildv1alpha1.Build) (*buildv1alpha1.Build, error) {
	bc, ok := c.builds[context]
	if !ok {
		return nil, errors.New("context not found")
	}
	return bc.client.BuildV1alpha1().Builds(namespace).Create(b)
}
func (c *controller) updateProwJob(namespace string, pj *prowjobv1.ProwJob) (*prowjobv1.ProwJob, error) {
	return c.pjc.ProwV1().ProwJobs(namespace).Update(pj)
}

func (c *controller) now() metav1.Time {
	return metav1.Now()
}

func (c *controller) buildID(pj prowjobv1.ProwJob) (string, error) {
	return pjutil.GetBuildID(pj.Spec.Job, c.totURL)
}

var (
	groupVersionKind = schema.GroupVersionKind{
		Group:   prowjobv1.SchemeGroupVersion.Group,
		Version: prowjobv1.SchemeGroupVersion.Version,
		Kind:    "ProwJob",
	}
)

// reconcile ensures a knative-build prowjob has a corresponding build, updating the prowjob's status as the build progresses.
func reconcile(c reconciler, key string) error {
	ctx, namespace, name, err := fromKey(key)
	if err != nil {
		runtime.HandleError(err)
		return nil
	}

	var wantBuild bool

	// TODO(fejta): only consider prowjob namespace
	pj, err := c.getProwJob(namespace, name)
	switch {
	case apierrors.IsNotFound(err):
		// Do not want build
	case err != nil:
		return fmt.Errorf("get prowjob: %v", err)
	case pj.Spec.Agent != prowjobv1.KnativeBuildAgent:
		// Do not want a build for this job
	case pj.Spec.Cluster != ctx:
		// Build is in wrong cluster, we do not want this build
		logrus.Warnf("%s found in context %s not %s", key, ctx, pj.Spec.Cluster)
	case pj.DeletionTimestamp == nil:
		wantBuild = true
	}

	var haveBuild bool

	// TODO(fejta): make trigger set the expected Namespace for the pod/build.
	b, err := c.getBuild(ctx, namespace, name)
	switch {
	case apierrors.IsNotFound(err):
		// Do not have a build
	case err != nil:
		return fmt.Errorf("get build: %v", err)
	case b.DeletionTimestamp == nil:
		haveBuild = true
	}

	// Should we create or delete this build?
	switch {
	case !wantBuild:
		if !haveBuild {
			logrus.Infof("Observed deleted %s", key)
			return nil
		}
		switch v, ok := b.Labels[kube.CreatedByProw]; {
		case !ok, v != "true": // Not controlled by this
			return nil
		}
		logrus.Infof("Delete builds/%s", key)
		if err = c.deleteBuild(ctx, namespace, name); err != nil {
			return fmt.Errorf("delete build: %v", err)
		}
		return nil
	case finalState(pj.Status.State):
		logrus.Infof("Observed finished %s", key)
		return nil
	case wantBuild && pj.Spec.BuildSpec == nil:
		return errors.New("nil BuildSpec")
	case wantBuild && !haveBuild:
		id, err := c.buildID(*pj)
		if err != nil {
			return fmt.Errorf("failed to get build id: %v", err)
		}
		if b, err = makeBuild(*pj, id); err != nil {
			return fmt.Errorf("make build: %v", err)
		}
		logrus.Infof("Create builds/%s", key)
		if b, err = c.createBuild(ctx, namespace, b); err != nil {
			return fmt.Errorf("create build: %v", err)
		}
	}

	// Ensure prowjob status is correct
	haveState := pj.Status.State
	haveMsg := pj.Status.Description
	wantState, wantMsg := prowJobStatus(b.Status)
	if haveState != wantState || haveMsg != wantMsg {
		npj := pj.DeepCopy()
		if npj.Status.StartTime.IsZero() {
			npj.Status.StartTime = c.now()
		}
		if npj.Status.CompletionTime.IsZero() && finalState(wantState) {
			now := c.now()
			npj.Status.CompletionTime = &now
		}
		npj.Status.State = wantState
		npj.Status.Description = wantMsg
		logrus.Infof("Update prowjobs/%s", key)
		if _, err = c.updateProwJob(namespace, npj); err != nil {
			return fmt.Errorf("update prow status: %v", err)
		}
	}
	return nil
}

// finalState returns true if the prowjob has already finished
func finalState(status prowjobv1.ProwJobState) bool {
	switch status {
	case "", prowjobv1.PendingState, prowjobv1.TriggeredState:
		return false
	}
	return true
}

// description computes the ProwJobStatus description for this condition or falling back to a default if none is provided.
func description(cond buildv1alpha1.BuildCondition, fallback string) string {
	switch {
	case cond.Message != "":
		return cond.Message
	case cond.Reason != "":
		return cond.Reason
	}
	return fallback
}

const (
	descScheduling       = "scheduling"
	descInitializing     = "initializing"
	descRunning          = "running"
	descSucceeded        = "succeeded"
	descFailed           = "failed"
	descUnknown          = "unknown status"
	descMissingCondition = "missing end condition"
)

// prowJobStatus returns the desired state and description based on the build status.
func prowJobStatus(bs buildv1alpha1.BuildStatus) (prowjobv1.ProwJobState, string) {
	started := bs.StartTime
	finished := bs.CompletionTime
	pcond := bs.GetCondition(buildv1alpha1.BuildSucceeded)
	if pcond == nil {
		if !finished.IsZero() {
			return prowjobv1.ErrorState, descMissingCondition
		}
		return prowjobv1.TriggeredState, descScheduling
	}
	cond := *pcond
	switch {
	case cond.Status == untypedcorev1.ConditionTrue:
		return prowjobv1.SuccessState, description(cond, descSucceeded)
	case cond.Status == untypedcorev1.ConditionFalse:
		return prowjobv1.FailureState, description(cond, descFailed)
	case started.IsZero():
		return prowjobv1.TriggeredState, description(cond, descInitializing)
	case finished.IsZero():
		return prowjobv1.PendingState, description(cond, descRunning)
	}
	return prowjobv1.ErrorState, description(cond, descUnknown) // shouldn't happen
}

// TODO(fejta): knative/build convert package should export "workspace", "home", "/workspace"
// https://github.com/knative/build/blob/17e8cf8417e1ef3d29bd465d4f45ad19dd3a3f2c/pkg/builder/cluster/convert/convert.go#L39-L65
const (
	workspaceMountName = "workspace"
	homeMountName      = "home"
	workspaceMountPath = "/workspace"
)

var (
	codeMount = untypedcorev1.VolumeMount{
		Name:      workspaceMountName,
		MountPath: "/code-mount", // should be irrelevant
	}
	logMount = untypedcorev1.VolumeMount{
		Name:      homeMountName,
		MountPath: "/var/prow-build-log", // should be irrelevant
	}
)

func buildMeta(pj prowjobv1.ProwJob) metav1.ObjectMeta {
	podLabels, annotations := decorate.LabelsAndAnnotationsForJob(pj)
	return metav1.ObjectMeta{
		Annotations: annotations,
		Name:        pj.Name,
		Namespace:   pj.Namespace, // TODO(fejta): use pj.Spec.Namespace
		Labels:      podLabels,
	}
}

// buildEnv constructs the environment map for the job
func buildEnv(pj prowjobv1.ProwJob, buildID string) (map[string]string, error) {
	return downwardapi.EnvForSpec(downwardapi.NewJobSpec(pj.Spec, buildID, pj.Name))
}

// defaultArguments will append each arg to the template, except where the argument name is already defined.
func defaultArguments(t *buildv1alpha1.TemplateInstantiationSpec, rawEnv map[string]string) {
	keys := sets.String{}
	for _, arg := range t.Arguments {
		keys.Insert(arg.Name)
	}
	for k, v := range rawEnv {
		if keys.Has(k) {
			continue
		}
		t.Arguments = append(t.Arguments, buildv1alpha1.ArgumentSpec{Name: k, Value: v})
	}
}

// defaultEnv adds the map of environment variables to the container, except keys already defined.
func defaultEnv(c *untypedcorev1.Container, rawEnv map[string]string) {
	keys := sets.String{}
	for _, arg := range c.Env {
		keys.Insert(arg.Name)
	}
	for k, v := range rawEnv {
		if keys.Has(k) {
			continue
		}
		c.Env = append(c.Env, untypedcorev1.EnvVar{Name: k, Value: v})
	}
}

// injectEnvironment will add rawEnv to the build steps and/or template arguments.
func injectEnvironment(b *buildv1alpha1.Build, rawEnv map[string]string) {
	for i := range b.Spec.Steps { // Inject environment variables to each step
		defaultEnv(&b.Spec.Steps[i], rawEnv)
	}
	if b.Spec.Template != nil { // Also add it as template arguments
		defaultArguments(b.Spec.Template, rawEnv)
	}
}

func workDir(refs prowjobv1.Refs) buildv1alpha1.ArgumentSpec {
	// workspaceMountName is auto-injected into each step at workspaceMountPath
	return buildv1alpha1.ArgumentSpec{Name: "WORKDIR", Value: clone.PathForRefs(workspaceMountPath, refs)}
}

// injectSource adds the custom source container to call clonerefs correctly.
//
// Does nothing if the build spec predefines Source
func injectSource(b *buildv1alpha1.Build, pj prowjobv1.ProwJob) error {
	if b.Spec.Source != nil {
		return nil
	}
	srcContainer, refs, cloneVolumes, err := decorate.CloneRefs(pj, codeMount, logMount)
	if err != nil {
		return fmt.Errorf("clone source error: %v", err)
	}
	if srcContainer == nil {
		return nil
	}

	b.Spec.Source = &buildv1alpha1.SourceSpec{
		Custom: srcContainer,
	}
	b.Spec.Volumes = append(b.Spec.Volumes, cloneVolumes...)

	wd := workDir(refs[0])
	// Inject correct working directory
	for i := range b.Spec.Steps {
		if b.Spec.Steps[i].WorkingDir != "" {
			continue
		}
		b.Spec.Steps[i].WorkingDir = wd.Value
	}
	if b.Spec.Template != nil {
		// Best we can do for a template is to set WORKDIR
		b.Spec.Template.Arguments = append(b.Spec.Template.Arguments, wd)
	}

	return nil
}

// makeBuild creates a build from the prowjob, using the prowjob's buildspec.
func makeBuild(pj prowjobv1.ProwJob, buildID string) (*buildv1alpha1.Build, error) {
	if pj.Spec.BuildSpec == nil {
		return nil, errors.New("nil BuildSpec")
	}
	b := buildv1alpha1.Build{
		ObjectMeta: buildMeta(pj),
		Spec:       *pj.Spec.BuildSpec,
	}
	rawEnv, err := buildEnv(pj, buildID)
	if err != nil {
		return nil, fmt.Errorf("environment error: %v", err)
	}
	injectEnvironment(&b, rawEnv)
	err = injectSource(&b, pj)

	return &b, nil
}
