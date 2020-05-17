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
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	prowjobset "k8s.io/test-infra/prow/client/clientset/versioned"
	prowjobscheme "k8s.io/test-infra/prow/client/clientset/versioned/scheme"
	prowjobinfov1 "k8s.io/test-infra/prow/client/informers/externalversions/prowjobs/v1"
	prowjoblisters "k8s.io/test-infra/prow/client/listers/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pod-utils/decorate"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"

	"github.com/sirupsen/logrus"
	pipelinev1alpha1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	untypedcorev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"knative.dev/pkg/apis"
)

const (
	controllerName = "prow-pipeline-crd"
)

type controller struct {
	config    config.Getter
	pjc       prowjobset.Interface
	pipelines map[string]pipelineConfig
	totURL    string

	pjLister   prowjoblisters.ProwJobLister
	pjInformer cache.SharedIndexInformer

	workqueue workqueue.RateLimitingInterface

	recorder record.EventRecorder

	prowJobsDone  bool
	pipelinesDone map[string]bool
	wait          string
}

type controllerOptions struct {
	kc              kubernetes.Interface
	pjc             prowjobset.Interface
	pji             prowjobinfov1.ProwJobInformer
	pipelineConfigs map[string]pipelineConfig
	totURL          string
	prowConfig      config.Getter
	rl              workqueue.RateLimitingInterface
}

// pjNamespace retruns the prow namespace from configuration
func (c *controller) pjNamespace() string {
	return c.config().ProwJobNamespace
}

// hasSynced returns true when every prowjob and pipeline informer has synced.
func (c *controller) hasSynced() bool {
	if !c.pjInformer.HasSynced() {
		if c.wait != "prowjobs" {
			c.wait = "prowjobs"
			ns := c.pjNamespace()
			if ns == "" {
				ns = "controllers"
			}
			logrus.Infof("Waiting on prowjobs in %s namespace...", ns)
		}
		return false // still syncing prowjobs
	}
	if !c.prowJobsDone {
		c.prowJobsDone = true
		logrus.Info("Synced prow jobs")
	}
	if c.pipelinesDone == nil {
		c.pipelinesDone = map[string]bool{}
	}
	for n, cfg := range c.pipelines {
		if !cfg.informer.Informer().HasSynced() {
			if c.wait != n {
				c.wait = n
				logrus.Infof("Waiting on %s pipelines...", n)
			}
			return false // still syncing pipelines in at least one cluster
		} else if !c.pipelinesDone[n] {
			c.pipelinesDone[n] = true
			logrus.Infof("Synced %s pipelines", n)
		}
	}
	return true // Everyone is synced
}

func newController(opts controllerOptions) (*controller, error) {
	if err := prowjobscheme.AddToScheme(scheme.Scheme); err != nil {
		return nil, err
	}

	// Log to events
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	eventBroadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: opts.kc.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, untypedcorev1.EventSource{Component: controllerName})

	c := &controller{
		config:     opts.prowConfig,
		pjc:        opts.pjc,
		pipelines:  opts.pipelineConfigs,
		pjLister:   opts.pji.Lister(),
		pjInformer: opts.pji.Informer(),
		workqueue:  opts.rl,
		recorder:   recorder,
		totURL:     opts.totURL,
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

	for ctx, cfg := range opts.pipelineConfigs {
		// Reconcile whenever a pipelinerun changes.
		ctx := ctx // otherwise it will change
		cfg.informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
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

	logrus.Info("Starting Pipeline controller")
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

// enqueueKey schedules an item for reconciliation
func (c *controller) enqueueKey(ctx string, obj interface{}) {
	switch o := obj.(type) {
	case *prowjobv1.ProwJob:
		ns := o.Spec.Namespace
		if ns == "" {
			ns = o.Namespace
		}
		c.workqueue.AddRateLimited(toKey(ctx, ns, o.Name))
	case *pipelinev1alpha1.PipelineRun:
		c.workqueue.AddRateLimited(toKey(ctx, o.Namespace, o.Name))
	default:
		logrus.Warnf("cannot enqueue unknown type %T: %v", o, obj)
		return
	}
}

type reconciler interface {
	getProwJob(name string) (*prowjobv1.ProwJob, error)
	patchProwJob(pj *prowjobv1.ProwJob, newpj *prowjobv1.ProwJob) (*prowjobv1.ProwJob, error)
	getPipelineRun(context, namespace, name string) (*pipelinev1alpha1.PipelineRun, error)
	deletePipelineRun(context, namespace, name string) error
	createPipelineRun(context, namespace string, b *pipelinev1alpha1.PipelineRun) (*pipelinev1alpha1.PipelineRun, error)
	pipelineID(prowjobv1.ProwJob) (string, string, error)
	now() metav1.Time
}

func (c *controller) getPipelineConfig(ctx string) (pipelineConfig, error) {
	cfg, ok := c.pipelines[ctx]
	if !ok {
		defaultCtx := kube.DefaultClusterAlias
		defaultCfg, ok := c.pipelines[defaultCtx]
		if !ok {
			return pipelineConfig{}, fmt.Errorf("no cluster configuration found for default context %q", defaultCtx)
		}
		return defaultCfg, nil
	}
	return cfg, nil
}

func (c *controller) getProwJob(name string) (*prowjobv1.ProwJob, error) {
	return c.pjLister.ProwJobs(c.pjNamespace()).Get(name)
}

func (c *controller) patchProwJob(pj *prowjobv1.ProwJob, newpj *prowjobv1.ProwJob) (*prowjobv1.ProwJob, error) {
	logrus.Debugf("patchProwJob(%s)", pj.Name)
	return pjutil.PatchProwjob(c.pjc.ProwV1().ProwJobs(c.pjNamespace()), logrus.NewEntry(logrus.StandardLogger()), *pj, *newpj)
}

func (c *controller) getPipelineRun(context, namespace, name string) (*pipelinev1alpha1.PipelineRun, error) {
	p, err := c.getPipelineConfig(context)
	if err != nil {
		return nil, err
	}
	return p.informer.Lister().PipelineRuns(namespace).Get(name)
}

func (c *controller) deletePipelineRun(context, namespace, name string) error {
	logrus.Debugf("deletePipeline(%s,%s,%s)", context, namespace, name)
	p, err := c.getPipelineConfig(context)
	if err != nil {
		return err
	}
	return p.client.TektonV1alpha1().PipelineRuns(namespace).Delete(name, &metav1.DeleteOptions{})
}

func (c *controller) createPipelineRun(context, namespace string, p *pipelinev1alpha1.PipelineRun) (*pipelinev1alpha1.PipelineRun, error) {
	logrus.Debugf("createPipelineRun(%s,%s,%s)", context, namespace, p.Name)
	pc, err := c.getPipelineConfig(context)
	if err != nil {
		return nil, err
	}
	p, err = pc.client.TektonV1alpha1().PipelineRuns(namespace).Create(p)
	if err != nil {
		return p, err
	}
	// Block until the pipelinerun is in the lister, otherwise we may attempt to create it again
	err = wait.Poll(time.Second, 3*time.Second, func() (bool, error) {
		_, err := c.getPipelineRun(context, namespace, p.Name)
		return err == nil, err
	})
	return p, err
}

func (c *controller) now() metav1.Time {
	return metav1.Now()
}

func (c *controller) pipelineID(pj prowjobv1.ProwJob) (string, string, error) {
	id, err := pjutil.GetBuildID(pj.Spec.Job, c.totURL)
	if err != nil {
		return "", "", err
	}
	pj.Status.BuildID = id
	url, err := pjutil.JobURL(c.config().Plank, pj, logrus.NewEntry(logrus.StandardLogger()))
	if err != nil {
		logrus.WithFields(pjutil.ProwJobFields(&pj)).WithError(err).Error("Error calculating job status url")
	}
	return id, url, nil
}

// reconcile ensures a tekton prowjob has a corresponding pipeline, updating the prowjob's status as the pipeline progresses.
func reconcile(c reconciler, key string) error {
	logrus.Debugf("reconcile: %s\n", key)

	ctx, namespace, name, err := fromKey(key)
	if err != nil {
		runtime.HandleError(err)
		return nil
	}

	var wantPipelineRun bool
	pj, err := c.getProwJob(name)
	newpj := pj.DeepCopy()
	switch {
	case apierrors.IsNotFound(err):
		// Do not want pipeline
	case err != nil:
		return fmt.Errorf("get prowjob: %v", err)
	case pj.Spec.Agent != prowjobv1.TektonAgent:
		// Do not want a pipeline for this job
		// We could look for a pipeline to remove, but it is more efficient to
		// assume this field is immutable.
		return nil
	case pjutil.ClusterToCtx(pj.Spec.Cluster) != ctx:
		// Build is in wrong cluster, we do not want this build
		logrus.Warnf("%s found in context %s not %s", key, ctx, pjutil.ClusterToCtx(pj.Spec.Cluster))
	case pj.DeletionTimestamp == nil:
		wantPipelineRun = true
	}

	var havePipelineRun bool
	p, err := c.getPipelineRun(ctx, namespace, name)
	switch {
	case apierrors.IsNotFound(err):
		// Do not have a pipeline
	case err != nil:
		return fmt.Errorf("get pipelinerun %s: %v", key, err)
	case p.DeletionTimestamp == nil:
		havePipelineRun = true
	}

	var newPipelineRun bool
	switch {
	case !wantPipelineRun:
		if !havePipelineRun {
			if pj != nil && pj.Spec.Agent == prowjobv1.TektonAgent {
				logrus.Infof("Observed deleted: %s", key)
			}
			return nil
		}

		// Skip deleting if the pipeline run is not created by prow
		switch v, ok := p.Labels[kube.CreatedByProw]; {
		case !ok, v != "true":
			return nil
		}
		logrus.Infof("Delete PipelineRun/%s", key)
		if err = c.deletePipelineRun(ctx, namespace, name); err != nil {
			return fmt.Errorf("delete pipelinerun: %v", err)
		}
		return nil
	case finalState(pj.Status.State):
		logrus.Infof("Observed finished: %s", key)
		return nil
	case wantPipelineRun && pj.Spec.PipelineRunSpec == nil:
		return fmt.Errorf("nil PipelineRunSpec in ProwJob/%s", key)
	case wantPipelineRun && !havePipelineRun:
		id, url, err := c.pipelineID(*newpj)
		if err != nil {
			return fmt.Errorf("failed to get pipeline id: %v", err)
		}
		newpj.Status.BuildID = id
		newpj.Status.URL = url
		newPipelineRun = true
		pipelineRun, err := makePipelineRun(*newpj)
		if err != nil {
			return fmt.Errorf("error preparing resources: %v", err)
		}

		logrus.Infof("Create PipelineRun/%s", key)
		p, err = c.createPipelineRun(ctx, namespace, pipelineRun)
		if err != nil {
			jerr := fmt.Errorf("start pipeline: %v", err)
			// Set the prow job in error state to avoid an endless loop when
			// the pipeline cannot be executed (e.g. referenced pipeline does not exist)
			return updateProwJobState(c, key, newPipelineRun, pj, newpj, prowjobv1.ErrorState, jerr.Error())
		}
	}

	if p == nil {
		return fmt.Errorf("no pipelinerun found or created for %q, wantPipelineRun was %v", key, wantPipelineRun)
	}
	wantState, wantMsg := prowJobStatus(p.Status)
	return updateProwJobState(c, key, newPipelineRun, pj, newpj, wantState, wantMsg)
}

func updateProwJobState(c reconciler, key string, newPipelineRun bool, pj *prowjobv1.ProwJob, newpj *prowjobv1.ProwJob, state prowjobv1.ProwJobState, msg string) error {
	haveState := newpj.Status.State
	haveMsg := newpj.Status.Description
	if newPipelineRun || haveState != state || haveMsg != msg {
		if haveState != state && state == prowjobv1.PendingState {
			now := c.now()
			newpj.Status.PendingTime = &now
		}
		if newpj.Status.StartTime.IsZero() {
			newpj.Status.StartTime = c.now()
		}
		if newpj.Status.CompletionTime.IsZero() && finalState(state) {
			now := c.now()
			newpj.Status.CompletionTime = &now
		}
		newpj.Status.State = state
		newpj.Status.Description = msg
		logrus.Infof("Update ProwJob/%s: %s -> %s: %s", key, haveState, state, msg)

		if _, err := c.patchProwJob(pj, newpj); err != nil {
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
func description(cond apis.Condition, fallback string) string {
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

// prowJobStatus returns the desired state and description based on the pipeline status
func prowJobStatus(ps pipelinev1alpha1.PipelineRunStatus) (prowjobv1.ProwJobState, string) {
	started := ps.StartTime
	finished := ps.CompletionTime
	pcond := ps.GetCondition(apis.ConditionSucceeded)
	if pcond == nil {
		if !finished.IsZero() {
			return prowjobv1.ErrorState, descMissingCondition
		}
		return prowjobv1.PendingState, descScheduling
	}
	cond := *pcond
	switch {
	case cond.Status == untypedcorev1.ConditionTrue:
		return prowjobv1.SuccessState, description(cond, descSucceeded)
	case cond.Status == untypedcorev1.ConditionFalse:
		return prowjobv1.FailureState, description(cond, descFailed)
	case started.IsZero():
		return prowjobv1.PendingState, description(cond, descInitializing)
	case cond.Status == untypedcorev1.ConditionUnknown, finished.IsZero():
		return prowjobv1.PendingState, description(cond, descRunning)
	}

	logrus.Warnf("Unknown condition %#v", cond)
	return prowjobv1.ErrorState, description(cond, descUnknown) // shouldn't happen
}

// pipelineMeta builds the pipeline metadata from prow job definition
func pipelineMeta(name string, pj prowjobv1.ProwJob) metav1.ObjectMeta {
	labels, annotations := decorate.LabelsAndAnnotationsForJob(pj)
	return metav1.ObjectMeta{
		Annotations: annotations,
		Name:        name,
		Namespace:   pj.Spec.Namespace,
		Labels:      labels,
	}
}

// makePipelineGitResource creates a pipeline git resource from prow job
func makePipelineGitResource(name string, refs prowjobv1.Refs, pj prowjobv1.ProwJob) *pipelinev1alpha1.PipelineResource {
	// Pick source URL
	var sourceURL string
	switch {
	case refs.CloneURI != "":
		sourceURL = refs.CloneURI
	case refs.RepoLink != "":
		sourceURL = fmt.Sprintf("%s.git", refs.RepoLink)
	default:
		sourceURL = fmt.Sprintf("https://github.com/%s/%s.git", refs.Org, refs.Repo)
	}

	// Pick revision
	var revision string
	switch {
	case len(refs.Pulls) > 0:
		if refs.Pulls[0].SHA != "" {
			revision = refs.Pulls[0].SHA
		} else {
			revision = fmt.Sprintf("pull/%d/head", refs.Pulls[0].Number)
		}
	case refs.BaseSHA != "":
		revision = refs.BaseSHA
	default:
		revision = refs.BaseRef
	}

	pr := pipelinev1alpha1.PipelineResource{
		ObjectMeta: pipelineMeta(name, pj),
		Spec: pipelinev1alpha1.PipelineResourceSpec{
			Type: pipelinev1alpha1.PipelineResourceTypeGit,
			Params: []pipelinev1alpha1.ResourceParam{
				{
					Name:  "url",
					Value: sourceURL,
				},
				{
					Name:  "revision",
					Value: revision,
				},
			},
		},
	}
	return &pr
}

// makePipeline creates a PipelineRun and substitutes ProwJob managed pipeline resources with ResourceSpec instead of ResourceRef
// so that we don't have to take care of potentially dangling created pipeline resources.
func makePipelineRun(pj prowjobv1.ProwJob) (*pipelinev1alpha1.PipelineRun, error) {
	// First validate.
	if pj.Spec.PipelineRunSpec == nil {
		return nil, errors.New("no PipelineSpec defined")
	}
	buildID := pj.Status.BuildID
	if buildID == "" {
		return nil, errors.New("empty BuildID in status")
	}
	if err := config.ValidatePipelineRunSpec(pj.Spec.Type, pj.Spec.ExtraRefs, pj.Spec.PipelineRunSpec); err != nil {
		return nil, fmt.Errorf("invalid pipeline_run_spec: %v", err)
	}

	p := pipelinev1alpha1.PipelineRun{
		ObjectMeta: pipelineMeta(pj.Name, pj),
		Spec:       *pj.Spec.PipelineRunSpec.DeepCopy(),
	}

	// Add parameters instead of env vars.
	env, err := downwardapi.EnvForSpec(downwardapi.NewJobSpec(pj.Spec, buildID, pj.Name))
	if err != nil {
		return nil, err
	}
	for _, key := range sets.StringKeySet(env).List() {
		val := env[key]
		// TODO: make this handle existing values/substitutions.
		p.Spec.Params = append(p.Spec.Params, pipelinev1alpha1.Param{
			Name: key,
			Value: pipelinev1alpha1.ArrayOrString{
				Type:      pipelinev1alpha1.ParamTypeString,
				StringVal: val,
			},
		})
	}

	// Inject resources from prow job.
	for i, res := range p.Spec.Resources {
		refName := res.ResourceRef.Name
		var refs prowjobv1.Refs
		var suffix string
		if refName == config.ProwImplicitGitResource {
			if pj.Spec.Refs == nil {
				return nil, fmt.Errorf("%q requested on a ProwJob without an implicit git ref", config.ProwImplicitGitResource)
			}
			refs = *pj.Spec.Refs
			suffix = "-implicit-ref"
		} else if match := config.ReProwExtraRef.FindStringSubmatch(refName); len(match) == 2 {
			index, _ := strconv.Atoi(match[1]) // We can't error because the regexp only matches digits.
			refs = pj.Spec.ExtraRefs[index]    // ValidatePipelineRunSpec made sure this is safe.
			suffix = fmt.Sprintf("-extra-ref-%d", index)
		} else {
			continue
		}
		// Change resource ref to resource spec
		name := pj.Name + suffix
		resource := makePipelineGitResource(name, refs, pj)
		p.Spec.Resources[i].ResourceRef = nil
		p.Spec.Resources[i].ResourceSpec = &resource.Spec
	}

	return &p, nil
}
