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
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	coreapi "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	prowjobset "k8s.io/test-infra/prow/client/clientset/versioned"
	prowjobscheme "k8s.io/test-infra/prow/client/clientset/versioned/scheme"
	prowjobsetv1 "k8s.io/test-infra/prow/client/clientset/versioned/typed/prowjobs/v1"
	prowjobinfov1 "k8s.io/test-infra/prow/client/informers/externalversions/prowjobs/v1"
	prowjoblisters "k8s.io/test-infra/prow/client/listers/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pod-utils/clone"
	"k8s.io/test-infra/prow/pod-utils/decorate"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
	"k8s.io/test-infra/prow/pod-utils/wrapper"

	buildv1alpha1 "github.com/knative/build/pkg/apis/build/v1alpha1"
	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
)

const (
	controllerName = "prow-build-crd"
)

type pjClient struct {
	pjc prowjobsetv1.ProwJobInterface
}

func (c *pjClient) ReplaceProwJob(name string, pj prowjobv1.ProwJob) (prowjobv1.ProwJob, error) {
	npj, err := c.pjc.Update(&pj)
	if npj != nil {
		return *npj, err
	}
	return prowjobv1.ProwJob{}, err
}

type controller struct {
	config config.Getter
	pjc    prowjobset.Interface
	builds map[string]buildConfig
	totURL string

	pjLister   prowjoblisters.ProwJobLister
	pjInformer cache.SharedIndexInformer

	workqueue workqueue.RateLimitingInterface

	recorder record.EventRecorder

	prowJobsDone bool
	buildsDone   map[string]bool
	wait         string

	useAllowCancellations bool
}

type controllerOptions struct {
	kc                    kubernetes.Interface
	pjc                   prowjobset.Interface
	pji                   prowjobinfov1.ProwJobInformer
	buildConfigs          map[string]buildConfig
	totURL                string
	prowConfig            config.Getter
	rl                    workqueue.RateLimitingInterface
	useAllowCancellations bool
}

func (c controller) pjNamespace() string {
	return c.config().ProwJobNamespace
}

// hasSynced returns true when every prowjob and build informer has synced.
func (c *controller) hasSynced() bool {
	if !c.pjInformer.HasSynced() {
		if c.wait != "prowjobs" {
			c.wait = "prowjobs"
			ns := c.pjNamespace()
			if ns == "" {
				ns = "controller's"
			}
			logrus.Infof("Waiting on prowjobs in %s namespace...", ns)
		}
		return false // still syncing prowjobs
	}
	if !c.prowJobsDone {
		c.prowJobsDone = true
		logrus.Info("Synced prow jobs")
	}
	if c.buildsDone == nil {
		c.buildsDone = map[string]bool{}
	}
	for n, cfg := range c.builds {
		if !cfg.informer.HasSynced() {
			if c.wait != n {
				c.wait = n
				logrus.Infof("Waiting on %s builds...", n)
			}
			return false // still syncing builds in at least one cluster
		} else if !c.buildsDone[n] {
			c.buildsDone[n] = true
			logrus.Infof("Synced %s builds", n)
		}
	}
	return true // Everyone is synced
}

func newController(opts controllerOptions) (*controller, error) {
	if err := prowjobscheme.AddToScheme(scheme.Scheme); err != nil {
		return nil, fmt.Errorf("add prow job scheme: %v", err)
	}
	// Log to events
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	eventBroadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: opts.kc.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, coreapi.EventSource{Component: controllerName})

	// Create struct
	c := &controller{
		builds:                opts.buildConfigs,
		config:                opts.prowConfig,
		pjc:                   opts.pjc,
		pjInformer:            opts.pji.Informer(),
		pjLister:              opts.pji.Lister(),
		recorder:              recorder,
		totURL:                opts.totURL,
		workqueue:             opts.rl,
		useAllowCancellations: opts.useAllowCancellations,
	}

	logrus.Info("Setting up event handlers")

	// Reconcile whenever a prowjob changes
	opts.pji.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pj, ok := obj.(*prowjobv1.ProwJob)
			if !ok {
				logrus.Warnf("Ignoring bad prowjob add: %v", obj)
				return
			}
			c.enqueueKey(pjutil.ClusterToCtx(pj.Spec.Cluster), pj)
		},
		UpdateFunc: func(old, new interface{}) {
			pj, ok := new.(*prowjobv1.ProwJob)
			if !ok {
				logrus.Warnf("Ignoring bad prowjob update: %v", new)
				return
			}
			c.enqueueKey(pjutil.ClusterToCtx(pj.Spec.Cluster), pj)
		},
		DeleteFunc: func(obj interface{}) {
			pj, ok := obj.(*prowjobv1.ProwJob)
			if !ok {
				logrus.Warnf("Ignoring bad prowjob delete: %v", obj)
				return
			}
			c.enqueueKey(pjutil.ClusterToCtx(pj.Spec.Cluster), pj)
		},
	})

	for ctx, cfg := range opts.buildConfigs {
		// Reconcile whenever a build changes.
		ctx := ctx // otherwise it will change
		cfg.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				c.enqueueKey(ctx, obj)
			},
			UpdateFunc: func(old, new interface{}) {
				c.enqueueKey(ctx, new)
			},
			DeleteFunc: func(obj interface{}) {
				c.enqueueKey(ctx, obj)
			},
		})
	}

	return c, nil
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
func toKey(ctx, namespace, name string) string {
	return strings.Join([]string{ctx, namespace, name}, "/")
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
	switch o := obj.(type) {
	case *prowjobv1.ProwJob:
		c.workqueue.AddRateLimited(toKey(ctx, o.Spec.Namespace, o.Name))
	case *buildv1alpha1.Build:
		c.workqueue.AddRateLimited(toKey(ctx, o.Namespace, o.Name))
	default:
		logrus.Warnf("cannot enqueue unknown type %T: %v", o, obj)
		return
	}
}

type reconciler interface {
	getProwJob(name string) (*prowjobv1.ProwJob, error)
	getBuild(context, namespace, name string) (*buildv1alpha1.Build, error)
	deleteBuild(context, namespace, name string) error
	createBuild(context, namespace string, b *buildv1alpha1.Build) (*buildv1alpha1.Build, error)
	updateProwJob(pj *prowjobv1.ProwJob) (*prowjobv1.ProwJob, error)
	now() metav1.Time
	buildID(prowjobv1.ProwJob) (string, string, error)
	defaultBuildTimeout() time.Duration
	terminateDupProwJobs(ctx string, namespace string) error
}

func (c *controller) getProwJob(name string) (*prowjobv1.ProwJob, error) {
	return c.pjLister.ProwJobs(c.pjNamespace()).Get(name)
}

func (c *controller) getProwJobs(namespace string) ([]prowjobv1.ProwJob, error) {
	result := []prowjobv1.ProwJob{}
	jobs, err := c.pjLister.ProwJobs(namespace).List(labels.Everything())
	if err != nil {
		return result, err
	}
	for _, job := range jobs {
		if job.Spec.Agent == prowjobv1.KnativeBuildAgent {
			result = append(result, *job)
		}
	}
	return result, nil
}

func (c *controller) updateProwJob(pj *prowjobv1.ProwJob) (*prowjobv1.ProwJob, error) {
	logrus.Debugf("updateProwJob(%s)", pj.Name)
	return c.pjc.ProwV1().ProwJobs(c.pjNamespace()).Update(pj)
}

func (c *controller) getBuild(contextName, namespace, name string) (*buildv1alpha1.Build, error) {
	b, ok := c.builds[contextName]
	if !ok {
		return nil, errors.New("context not found")
	}
	build := &buildv1alpha1.Build{}
	nn := types.NamespacedName{Namespace: namespace, Name: name}
	return build, b.client.Get(context.TODO(), nn, build)
}
func (c *controller) deleteBuild(contextName, namespace, name string) error {
	logrus.Debugf("deleteBuild(%s,%s,%s)", contextName, namespace, name)
	b, ok := c.builds[contextName]
	if !ok {
		return errors.New("context not found")
	}
	buildObj := &buildv1alpha1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	return b.client.Delete(context.TODO(), buildObj)
}
func (c *controller) createBuild(contextName, namespace string, b *buildv1alpha1.Build) (*buildv1alpha1.Build, error) {
	logrus.Debugf("createBuild(%s,%s,%s)", contextName, namespace, b.Name)
	bc, ok := c.builds[contextName]
	if !ok {
		return nil, errors.New("context not found")
	}
	return b, bc.client.Create(context.TODO(), b)
}

func (c *controller) now() metav1.Time {
	return metav1.Now()
}

func (c *controller) buildID(pj prowjobv1.ProwJob) (string, string, error) {
	id, err := pjutil.GetBuildID(pj.Spec.Job, c.totURL)
	if err != nil {
		return "", "", err
	}
	pj.Status.BuildID = id
	url := pjutil.JobURL(c.config().Plank, pj, logrus.NewEntry(logrus.StandardLogger()))
	return id, url, nil
}

func (c *controller) defaultBuildTimeout() time.Duration {
	return c.config().DefaultJobTimeout.Duration
}

func (c *controller) allowCancellations() bool {
	return c.useAllowCancellations && c.config().Plank.AllowCancellations
}

func (c *controller) terminateDupProwJobs(ctx string, namespace string) error {
	pjClient := &pjClient{
		pjc: c.pjc.ProwV1().ProwJobs(c.config().ProwJobNamespace),
	}
	log := logrus.NewEntry(logrus.StandardLogger()).WithField("aborter", "build")

	jobs, err := c.getProwJobs(namespace)
	if err != nil {
		return err
	}
	return pjutil.TerminateOlderJobs(pjClient, log, jobs, func(toCancel prowjobv1.ProwJob) error {
		if c.allowCancellations() {
			if err := c.deleteBuild(ctx, namespace, toCancel.GetName()); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("deleting build: %v", err)
			}
		}
		return nil
	})
}

// reconcile ensures a knative-build prowjob has a corresponding build, updating the prowjob's status as the build progresses.
func reconcile(c reconciler, key string) error {
	ctx, namespace, name, err := fromKey(key)
	if err != nil {
		runtime.HandleError(err)
		return nil
	}

	if err := c.terminateDupProwJobs(ctx, namespace); err != nil {
		logrus.WithError(err).Warn("Cannot terminate duplicated prow jobs")
	}

	var wantBuild bool

	pj, err := c.getProwJob(name)
	switch {
	case apierrors.IsNotFound(err):
		// Do not want build
	case err != nil:
		return fmt.Errorf("get prowjob: %v", err)
	case pj.Spec.Agent != prowjobv1.KnativeBuildAgent:
		// Do not want a build for this job
	case pjutil.ClusterToCtx(pj.Spec.Cluster) != ctx:
		// Build is in wrong cluster, we do not want this build
		logrus.Warnf("%s found in context %s not %s", key, ctx, pjutil.ClusterToCtx(pj.Spec.Cluster))
	case pj.DeletionTimestamp == nil:
		wantBuild = true
	}

	var haveBuild bool

	b, err := c.getBuild(ctx, namespace, name)
	switch {
	case apierrors.IsNotFound(err):
		// Do not have a build
	case err != nil:
		return fmt.Errorf("get build %s: %v", key, err)
	case b.DeletionTimestamp == nil:
		haveBuild = true
	}

	var newBuildID bool
	// Should we create or delete this build?
	switch {
	case !wantBuild:
		if !haveBuild {
			if pj != nil && pj.Spec.Agent == prowjobv1.KnativeBuildAgent {
				logrus.Infof("Observed deleted %s", key)
			}
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
		id, url, err := c.buildID(*pj)
		if err != nil {
			return fmt.Errorf("failed to get build id: %v", err)
		}
		pj.Status.BuildID = id
		pj.Status.URL = url
		newBuildID = true
		if b, err = makeBuild(*pj, c.defaultBuildTimeout()); err != nil {
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
	if newBuildID || haveState != wantState || haveMsg != wantMsg {
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
		if _, err = c.updateProwJob(npj); err != nil {
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
func description(cond duckv1alpha1.Condition, fallback string) string {
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
	case cond.Status == coreapi.ConditionTrue:
		return prowjobv1.SuccessState, description(cond, descSucceeded)
	case cond.Status == coreapi.ConditionFalse:
		return prowjobv1.FailureState, description(cond, descFailed)
	case started.IsZero():
		return prowjobv1.TriggeredState, description(cond, descInitializing)
	case cond.Status == coreapi.ConditionUnknown, finished.IsZero():
		return prowjobv1.PendingState, description(cond, descRunning)
	}
	logrus.Warnf("Unknown condition %#v", cond)
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
	codeMount = coreapi.VolumeMount{
		Name:      workspaceMountName,
		MountPath: "/code-mount", // should be irrelevant
	}
	logMount = coreapi.VolumeMount{
		Name:      homeMountName,
		MountPath: "/var/prow-build-log", // should be irrelevant
	}
)

func buildMeta(pj prowjobv1.ProwJob) metav1.ObjectMeta {
	podLabels, annotations := decorate.LabelsAndAnnotationsForJob(pj)
	return metav1.ObjectMeta{
		Annotations: annotations,
		Name:        pj.Name,
		Namespace:   pj.Spec.Namespace,
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
	for _, k := range sets.StringKeySet(rawEnv).List() { // deterministic ordering
		if keys.Has(k) {
			continue
		}
		t.Arguments = append(t.Arguments, buildv1alpha1.ArgumentSpec{Name: k, Value: rawEnv[k]})
	}
}

// defaultEnv adds the map of environment variables to the container, except keys already defined.
func defaultEnv(c *coreapi.Container, rawEnv map[string]string) {
	keys := sets.String{}
	for _, arg := range c.Env {
		keys.Insert(arg.Name)
	}
	for _, k := range sets.StringKeySet(rawEnv).List() { // deterministic ordering
		if keys.Has(k) {
			continue
		}
		c.Env = append(c.Env, coreapi.EnvVar{Name: k, Value: rawEnv[k]})
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
// Returns true if it added this container
//
// Does nothing if the build spec predefines Source
func injectSource(b *buildv1alpha1.Build, pj prowjobv1.ProwJob) (bool, error) {
	if b.Spec.Source != nil {
		return false, nil
	}
	srcContainer, refs, cloneVolumes, err := decorate.CloneRefs(pj, codeMount, logMount)
	if err != nil {
		return false, fmt.Errorf("clone source error: %v", err)
	}
	if srcContainer == nil {
		return false, nil
	} else {
		srcContainer.Name = "" // knative-build requirement
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

	return true, nil
}

func tools() (coreapi.Volume, coreapi.VolumeMount) {
	const toolsName = "entrypoint-tools"
	toolsVolume := coreapi.Volume{
		Name: toolsName,
		VolumeSource: coreapi.VolumeSource{
			EmptyDir: &coreapi.EmptyDirVolumeSource{},
		},
	}

	toolsMount := coreapi.VolumeMount{
		Name:      toolsName,
		MountPath: "/entrypoint-tools",
	}
	return toolsVolume, toolsMount
}

func decorateSteps(steps []coreapi.Container, dc prowjobv1.DecorationConfig, toolsMount coreapi.VolumeMount) ([]wrapper.Options, error) {
	const alwaysPass = true
	var entries []wrapper.Options
	for i := range steps {
		if steps[i].Name == "" {
			steps[i].Name = fmt.Sprintf("step-%d", i)
		}
		var previousMarker string
		if i > 0 {
			previousMarker = entries[i-1].MarkerFile
		}
		// TODO(fejta): consider refactoring entrypoint to accept --expire=time.Now.Add(dc.Timeout) so we timeout each step correctly (assuming a good clock)
		opt, err := decorate.InjectEntrypoint(&steps[i], dc.Timeout.Get(), dc.GracePeriod.Get(), steps[i].Name, previousMarker, alwaysPass, logMount, toolsMount)
		if err != nil {
			return nil, fmt.Errorf("inject entrypoint into %s: %v", steps[i].Name, err)
		}
		entries = append(entries, *opt)
	}
	return entries, nil
}

// injectedSteps returns initial containers, a final container and an additional volume.
func injectedSteps(encodedJobSpec string, dc prowjobv1.DecorationConfig, injectedSource bool, toolsMount coreapi.VolumeMount, entries []wrapper.Options) ([]coreapi.Container, *coreapi.Container, *coreapi.Volume, error) {
	// localMode parameter is false and outputMount is nil because we don't have a local decoration
	// mode for "agent: build" jobs yet. (We need a mkpod equivalent for builds first.)
	gcsVol, gcsMount, gcsOptions := decorate.GCSOptions(dc, false)

	sidecar, err := decorate.Sidecar(dc.UtilityImages.Sidecar, gcsOptions, gcsMount, logMount, nil, encodedJobSpec, decorate.RequirePassingEntries, entries...)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("inject sidecar: %v", err)
	}

	var cloneLogMount *coreapi.VolumeMount
	if injectedSource {
		cloneLogMount = &logMount
	}
	initUpload, err := decorate.InitUpload(dc.UtilityImages.InitUpload, gcsOptions, gcsMount, cloneLogMount, nil, encodedJobSpec)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("inject initupload: %v", err)
	}

	placer := decorate.PlaceEntrypoint(dc.UtilityImages.Entrypoint, toolsMount)

	return []coreapi.Container{placer, *initUpload}, sidecar, gcsVol, nil
}

// determineTimeout decides the timeout value used for build
func determineTimeout(spec *buildv1alpha1.BuildSpec, dc *prowjobv1.DecorationConfig, defaultTimeout time.Duration) time.Duration {
	switch {
	case spec.Timeout != nil:
		return spec.Timeout.Duration
	case dc != nil && dc.Timeout.Get() > 0:
		return dc.Timeout.Duration
	default:
		return defaultTimeout
	}
}

func injectTimeout(spec *buildv1alpha1.BuildSpec, dc *prowjobv1.DecorationConfig, defaultTimeout time.Duration) {
	spec.Timeout = &metav1.Duration{Duration: determineTimeout(spec, dc, defaultTimeout)}
}

func decorateBuild(spec *buildv1alpha1.BuildSpec, encodedJobSpec string, dc prowjobv1.DecorationConfig, injectedSource bool) error {
	toolsVolume, toolsMount := tools()

	entries, err := decorateSteps(spec.Steps, dc, toolsMount)
	if err != nil {
		return fmt.Errorf("decorate steps: %v", err)
	}

	befores, after, gcsVol, err := injectedSteps(encodedJobSpec, dc, injectedSource, toolsMount, entries)
	if err != nil {
		return fmt.Errorf("add injected steps: %v", err)
	}

	spec.Steps = append(befores, spec.Steps...)
	spec.Steps = append(spec.Steps, *after)
	spec.Volumes = append(spec.Volumes, toolsVolume)
	if gcsVol != nil {
		// This check isn't strictly necessary until/unless we add a local mode for build jobs.
		// /shrug
		spec.Volumes = append(spec.Volumes, *gcsVol)
	}
	return nil
}

// makeBuild creates a build from the prowjob, using the prowjob's buildspec.
func makeBuild(pj prowjobv1.ProwJob, defaultTimeout time.Duration) (*buildv1alpha1.Build, error) {
	if pj.Spec.BuildSpec == nil {
		return nil, errors.New("nil BuildSpec in spec")
	}
	buildID := pj.Status.BuildID
	if buildID == "" {
		return nil, errors.New("empty BuildID in status")
	}
	b := buildv1alpha1.Build{
		ObjectMeta: buildMeta(pj),
		Spec:       *pj.Spec.BuildSpec.DeepCopy(),
	}
	rawEnv, err := buildEnv(pj, buildID)
	if err != nil {
		return nil, fmt.Errorf("environment error: %v", err)
	}
	injectEnvironment(&b, rawEnv)
	injectedSource, err := injectSource(&b, pj)
	if err != nil {
		return nil, fmt.Errorf("inject source: %v", err)
	}
	injectTimeout(&b.Spec, pj.Spec.DecorationConfig, defaultTimeout)
	if pj.Spec.DecorationConfig != nil {
		if b.Spec.Template != nil {
			return nil, errors.New("cannot decorate Build using BuildTemplate")
		}
		encodedJobSpec := rawEnv[downwardapi.JobSpecEnv]
		err = decorateBuild(&b.Spec, encodedJobSpec, *pj.Spec.DecorationConfig, injectedSource)
		if err != nil {
			return nil, fmt.Errorf("decorate build: %v", err)
		}
	}

	return &b, nil
}
