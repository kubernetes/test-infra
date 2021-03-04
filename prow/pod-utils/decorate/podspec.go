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

package decorate

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	coreapi "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/clonerefs"
	"k8s.io/test-infra/prow/entrypoint"
	"k8s.io/test-infra/prow/gcsupload"
	"k8s.io/test-infra/prow/initupload"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pod-utils/clone"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
	"k8s.io/test-infra/prow/pod-utils/wrapper"
	"k8s.io/test-infra/prow/sidecar"
)

const (
	logMountName            = "logs"
	logMountPath            = "/logs"
	artifactsEnv            = "ARTIFACTS"
	artifactsPath           = logMountPath + "/artifacts"
	codeMountName           = "code"
	codeMountPath           = "/home/prow/go"
	gopathEnv               = "GOPATH"
	toolsMountName          = "tools"
	toolsMountPath          = "/tools"
	gcsCredentialsMountName = "gcs-credentials"
	gcsCredentialsMountPath = "/secrets/gcs"
	s3CredentialsMountName  = "s3-credentials"
	s3CredentialsMountPath  = "/secrets/s3-storage"
	outputMountName         = "output"
	outputMountPath         = "/output"
	oauthTokenFilename      = "oauth-token"
)

// Labels returns a string slice with label consts from kube.
func Labels() []string {
	return []string{kube.ProwJobTypeLabel, kube.CreatedByProw, kube.ProwJobIDLabel}
}

// VolumeMounts returns a string set with *MountName consts in it.
func VolumeMounts() sets.String {
	return sets.NewString(logMountName, codeMountName, toolsMountName, gcsCredentialsMountName, s3CredentialsMountName)
}

// VolumeMountsOnTestContainer returns a string set with *MountName consts in it which are applied to the test container.
func VolumeMountsOnTestContainer() sets.String {
	return sets.NewString(logMountName, codeMountName, toolsMountName)
}

// VolumeMountPathsOnTestContainer returns a string set with *MountPath consts in it which are applied to the test container.
func VolumeMountPathsOnTestContainer() sets.String {
	return sets.NewString(logMountPath, codeMountPath, toolsMountPath)
}

// PodUtilsContainerNames returns a string set with pod utility container name consts in it.
func PodUtilsContainerNames() sets.String {
	return sets.NewString(cloneRefsName, initUploadName, entrypointName, sidecarName)
}

// LabelsAndAnnotationsForSpec returns a minimal set of labels to add to prowjobs or its owned resources.
//
// User-provided extraLabels and extraAnnotations values will take precedence over auto-provided values.
func LabelsAndAnnotationsForSpec(spec prowapi.ProwJobSpec, extraLabels, extraAnnotations map[string]string) (map[string]string, map[string]string) {
	jobNameForLabel := spec.Job
	log := logrus.WithFields(logrus.Fields{
		"job": spec.Job,
		"id":  extraLabels[kube.ProwBuildIDLabel],
	})
	if len(jobNameForLabel) > validation.LabelValueMaxLength {
		// TODO(fejta): consider truncating middle rather than end.
		jobNameForLabel = strings.TrimRight(spec.Job[:validation.LabelValueMaxLength], ".-")
		log.WithFields(logrus.Fields{
			"key":       kube.ProwJobAnnotation,
			"value":     spec.Job,
			"truncated": jobNameForLabel,
		}).Info("Cannot use full job name, will truncate.")
	}
	labels := map[string]string{
		kube.CreatedByProw:     "true",
		kube.ProwJobTypeLabel:  string(spec.Type),
		kube.ProwJobAnnotation: jobNameForLabel,
	}
	if spec.Type != prowapi.PeriodicJob && spec.Refs != nil {
		labels[kube.OrgLabel] = spec.Refs.Org
		labels[kube.RepoLabel] = spec.Refs.Repo
		if len(spec.Refs.Pulls) > 0 {
			labels[kube.PullLabel] = strconv.Itoa(spec.Refs.Pulls[0].Number)
		}
	}

	for k, v := range extraLabels {
		labels[k] = v
	}

	// let's validate labels
	for key, value := range labels {
		if errs := validation.IsValidLabelValue(value); len(errs) > 0 {
			// try to use basename of a path, if path contains invalid //
			base := filepath.Base(value)
			if errs := validation.IsValidLabelValue(base); len(errs) == 0 {
				labels[key] = base
				continue
			}
			log.WithFields(logrus.Fields{
				"key":    key,
				"value":  value,
				"errors": errs,
			}).Warn("Removing invalid label")
			delete(labels, key)
		}
	}

	annotations := map[string]string{
		kube.ProwJobAnnotation: spec.Job,
	}
	for k, v := range extraAnnotations {
		annotations[k] = v
	}

	return labels, annotations
}

// LabelsAndAnnotationsForJob returns a standard set of labels to add to pod/build/etc resources.
func LabelsAndAnnotationsForJob(pj prowapi.ProwJob) (map[string]string, map[string]string) {
	var extraLabels map[string]string
	if extraLabels = pj.ObjectMeta.Labels; extraLabels == nil {
		extraLabels = map[string]string{}
	}
	var extraAnnotations map[string]string
	if extraAnnotations = pj.ObjectMeta.Annotations; extraAnnotations == nil {
		extraAnnotations = map[string]string{}
	}
	extraLabels[kube.ProwJobIDLabel] = pj.ObjectMeta.Name
	extraLabels[kube.ProwBuildIDLabel] = pj.Status.BuildID
	return LabelsAndAnnotationsForSpec(pj.Spec, extraLabels, extraAnnotations)
}

// ProwJobToPod converts a ProwJob to a Pod that will run the tests.
func ProwJobToPod(pj prowapi.ProwJob) (*coreapi.Pod, error) {
	return ProwJobToPodLocal(pj, "")
}

// ProwJobToPodLocal converts a ProwJob to a Pod that will run the tests.
// If an output directory is specified, files are copied to the dir instead of uploading to GCS if
// decoration is configured.
func ProwJobToPodLocal(pj prowapi.ProwJob, outputDir string) (*coreapi.Pod, error) {
	if pj.Spec.PodSpec == nil {
		return nil, fmt.Errorf("prowjob %q lacks a pod spec", pj.Name)
	}

	rawEnv, err := downwardapi.EnvForSpec(downwardapi.NewJobSpec(pj.Spec, pj.Status.BuildID, pj.Name))
	if err != nil {
		return nil, err
	}

	spec := pj.Spec.PodSpec.DeepCopy()
	spec.RestartPolicy = "Never"
	if len(spec.Containers) == 1 {
		spec.Containers[0].Name = kube.TestContainerName
	}

	// if the user has not provided a serviceaccount to use or explicitly
	// requested mounting the default token, we treat the unset value as
	// false, while kubernetes treats it as true if it is unset because
	// it was added in v1.6
	if spec.AutomountServiceAccountToken == nil && spec.ServiceAccountName == "" {
		myFalse := false
		spec.AutomountServiceAccountToken = &myFalse
	}

	if pj.Spec.DecorationConfig == nil {
		for i, container := range spec.Containers {
			spec.Containers[i].Env = append(container.Env, KubeEnv(rawEnv)...)
		}
	} else {
		if err := decorate(spec, &pj, rawEnv, outputDir); err != nil {
			return nil, fmt.Errorf("error decorating podspec: %v", err)
		}
	}

	// If no termination policy is specified, use log fallback so the pod status
	// contains a snippet of the failure, which is helpful when pods are cleaned up
	// or evicted in failure modes. Callers can override by setting explicit policy.
	for i, container := range spec.InitContainers {
		if len(container.TerminationMessagePolicy) == 0 {
			spec.InitContainers[i].TerminationMessagePolicy = coreapi.TerminationMessageFallbackToLogsOnError
		}
	}
	for i, container := range spec.Containers {
		if len(container.TerminationMessagePolicy) == 0 {
			spec.Containers[i].TerminationMessagePolicy = coreapi.TerminationMessageFallbackToLogsOnError
		}
	}

	podLabels, annotations := LabelsAndAnnotationsForJob(pj)
	return &coreapi.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        pj.ObjectMeta.Name,
			Labels:      podLabels,
			Annotations: annotations,
		},
		Spec: *spec,
	}, nil
}

const cloneLogPath = "clone.json"

// CloneLogPath returns the path to the clone log file in the volume mount.
// CloneLogPath returns the path to the clone log file in the volume mount.
func CloneLogPath(logMount coreapi.VolumeMount) string {
	return filepath.Join(logMount.MountPath, cloneLogPath)
}

// Exposed for testing
const (
	entrypointName   = "place-entrypoint"
	initUploadName   = "initupload"
	sidecarName      = "sidecar"
	cloneRefsName    = "clonerefs"
	cloneRefsCommand = "/clonerefs"
)

// cloneEnv encodes clonerefs Options into json and puts it into an environment variable
func cloneEnv(opt clonerefs.Options) ([]coreapi.EnvVar, error) {
	// TODO(fejta): use flags
	cloneConfigEnv, err := clonerefs.Encode(opt)
	if err != nil {
		return nil, err
	}
	return KubeEnv(map[string]string{clonerefs.JSONConfigEnvVar: cloneConfigEnv}), nil
}

// tmpVolume creates an emptyDir volume and mount for a tmp folder
// This is e.g. used by CloneRefs to store the known hosts file
func tmpVolume(name string) (coreapi.Volume, coreapi.VolumeMount) {
	v := coreapi.Volume{
		Name: name,
		VolumeSource: coreapi.VolumeSource{
			EmptyDir: &coreapi.EmptyDirVolumeSource{},
		},
	}

	vm := coreapi.VolumeMount{
		Name:      name,
		MountPath: "/tmp",
		ReadOnly:  false,
	}

	return v, vm
}

func oauthVolume(secret, key string) (coreapi.Volume, coreapi.VolumeMount) {
	return coreapi.Volume{
			Name: secret,
			VolumeSource: coreapi.VolumeSource{
				Secret: &coreapi.SecretVolumeSource{
					SecretName: secret,
					Items: []coreapi.KeyToPath{{
						Key:  key,
						Path: fmt.Sprintf("./%s", oauthTokenFilename),
					}},
				},
			},
		}, coreapi.VolumeMount{
			Name:      secret,
			MountPath: "/secrets/oauth",
			ReadOnly:  true,
		}
}

// sshVolume converts a secret holding ssh keys into the corresponding volume and mount.
//
// This is used by CloneRefs to attach the mount to the clonerefs container.
func sshVolume(secret string) (coreapi.Volume, coreapi.VolumeMount) {
	var sshKeyMode int32 = 0400 // this is octal, so symbolic ref is `u+r`
	name := strings.Join([]string{"ssh-keys", secret}, "-")
	mountPath := path.Join("/secrets/ssh", secret)
	v := coreapi.Volume{
		Name: name,
		VolumeSource: coreapi.VolumeSource{
			Secret: &coreapi.SecretVolumeSource{
				SecretName:  secret,
				DefaultMode: &sshKeyMode,
			},
		},
	}

	vm := coreapi.VolumeMount{
		Name:      name,
		MountPath: mountPath,
		ReadOnly:  true,
	}

	return v, vm
}

// cookiefileVolumes converts a secret holding cookies into the corresponding volume and mount.
//
// Secret can be of the form secret-name/base-name or just secret-name.
// Here secret-name refers to the kubernetes secret volume to mount, and base-name refers to the key in the secret
// where the cookies are stored. The secret-name pattern is equivalent to secret-name/secret-name.
//
// This is used by CloneRefs to attach the mount to the clonerefs container.
// The returned string value is the path to the cookiefile for use with --cookiefile.
func cookiefileVolume(secret string) (coreapi.Volume, coreapi.VolumeMount, string) {
	// Separate secret-name/key-in-secret
	parts := strings.SplitN(secret, "/", 2)
	cookieSecret := parts[0]
	var base string
	if len(parts) == 1 {
		base = parts[0] // Assume key-in-secret == secret-name
	} else {
		base = parts[1]
	}
	var cookiefileMode int32 = 0400 // u+r
	vol := coreapi.Volume{
		Name: "cookiefile",
		VolumeSource: coreapi.VolumeSource{
			Secret: &coreapi.SecretVolumeSource{
				SecretName:  cookieSecret,
				DefaultMode: &cookiefileMode,
			},
		},
	}
	mount := coreapi.VolumeMount{
		Name:      vol.Name,
		MountPath: "/secrets/cookiefile", // append base to flag
		ReadOnly:  true,
	}
	return vol, mount, path.Join(mount.MountPath, base)
}

// CloneRefs constructs the container and volumes necessary to clone the refs requested by the ProwJob.
//
// The container checks out repositories specified by the ProwJob Refs to `codeMount`.
// A log of what it checked out is written to `clone.json` in `logMount`.
//
// The container may need to mount SSH keys and/or cookiefiles in order to access private refs.
// CloneRefs returns a list of volumes containing these secrets required by the container.
func CloneRefs(pj prowapi.ProwJob, codeMount, logMount coreapi.VolumeMount) (*coreapi.Container, []prowapi.Refs, []coreapi.Volume, error) {
	if pj.Spec.DecorationConfig == nil {
		return nil, nil, nil, nil
	}
	if skip := pj.Spec.DecorationConfig.SkipCloning; skip != nil && *skip {
		return nil, nil, nil, nil
	}
	var cloneVolumes []coreapi.Volume
	var refs []prowapi.Refs // Do not return []*prowapi.Refs which we do not own
	if pj.Spec.Refs != nil {
		refs = append(refs, *pj.Spec.Refs)
	}
	for _, r := range pj.Spec.ExtraRefs {
		refs = append(refs, r)
	}
	if len(refs) == 0 { // nothing to clone
		return nil, nil, nil, nil
	}
	if codeMount.Name == "" || codeMount.MountPath == "" {
		return nil, nil, nil, fmt.Errorf("codeMount must set Name and MountPath")
	}
	if logMount.Name == "" || logMount.MountPath == "" {
		return nil, nil, nil, fmt.Errorf("logMount must set Name and MountPath")
	}

	var cloneMounts []coreapi.VolumeMount
	var sshKeyPaths []string
	for _, secret := range pj.Spec.DecorationConfig.SSHKeySecrets {
		volume, mount := sshVolume(secret)
		cloneMounts = append(cloneMounts, mount)
		sshKeyPaths = append(sshKeyPaths, mount.MountPath)
		cloneVolumes = append(cloneVolumes, volume)
	}

	var oauthMountPath string
	if pj.Spec.DecorationConfig.OauthTokenSecret != nil {
		oauthVolume, oauthMount := oauthVolume(pj.Spec.DecorationConfig.OauthTokenSecret.Name, pj.Spec.DecorationConfig.OauthTokenSecret.Key)
		cloneMounts = append(cloneMounts, oauthMount)
		oauthMountPath = filepath.Join(oauthMount.MountPath, oauthTokenFilename)
		cloneVolumes = append(cloneVolumes, oauthVolume)
	}

	volume, mount := tmpVolume("clonerefs-tmp")
	cloneMounts = append(cloneMounts, mount)
	cloneVolumes = append(cloneVolumes, volume)

	var cloneArgs []string
	var cookiefilePath string

	if cp := pj.Spec.DecorationConfig.CookiefileSecret; cp != "" {
		v, vm, vp := cookiefileVolume(cp)
		cloneMounts = append(cloneMounts, vm)
		cloneVolumes = append(cloneVolumes, v)
		cookiefilePath = vp
		cloneArgs = append(cloneArgs, "--cookiefile="+cookiefilePath)
	}

	env, err := cloneEnv(clonerefs.Options{
		CookiePath:       cookiefilePath,
		GitRefs:          refs,
		GitUserEmail:     clonerefs.DefaultGitUserEmail,
		GitUserName:      clonerefs.DefaultGitUserName,
		HostFingerprints: pj.Spec.DecorationConfig.SSHHostFingerprints,
		KeyFiles:         sshKeyPaths,
		Log:              CloneLogPath(logMount),
		SrcRoot:          codeMount.MountPath,
		OauthTokenFile:   oauthMountPath,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("clone env: %v", err)
	}

	container := coreapi.Container{
		Name:         cloneRefsName,
		Image:        pj.Spec.DecorationConfig.UtilityImages.CloneRefs,
		Command:      []string{cloneRefsCommand},
		Args:         cloneArgs,
		Env:          env,
		VolumeMounts: append([]coreapi.VolumeMount{logMount, codeMount}, cloneMounts...),
	}

	if pj.Spec.DecorationConfig.Resources != nil && pj.Spec.DecorationConfig.Resources.CloneRefs != nil {
		container.Resources = *pj.Spec.DecorationConfig.Resources.CloneRefs
	}
	return &container, refs, cloneVolumes, nil
}

func processLog(log coreapi.VolumeMount, prefix string) string {
	if prefix == "" {
		return filepath.Join(log.MountPath, "process-log.txt")
	}
	return filepath.Join(log.MountPath, fmt.Sprintf("%s-log.txt", prefix))
}

func markerFile(log coreapi.VolumeMount, prefix string) string {
	if prefix == "" {
		return filepath.Join(log.MountPath, "marker-file.txt")
	}
	return filepath.Join(log.MountPath, fmt.Sprintf("%s-marker.txt", prefix))
}

func metadataFile(log coreapi.VolumeMount, prefix string) string {
	ad := artifactsDir(log)
	if prefix == "" {
		return filepath.Join(ad, "metadata.json")
	}
	return filepath.Join(ad, fmt.Sprintf("%s-metadata.json", prefix))
}

func artifactsDir(log coreapi.VolumeMount) string {
	return filepath.Join(log.MountPath, "artifacts")
}

func entrypointLocation(tools coreapi.VolumeMount) string {
	return filepath.Join(tools.MountPath, "entrypoint")
}

// InjectEntrypoint will make the entrypoint binary in the tools volume the container's entrypoint, which will output to the log volume.
func InjectEntrypoint(c *coreapi.Container, timeout, gracePeriod time.Duration, prefix, previousMarker string, exitZero bool, log, tools coreapi.VolumeMount) (*wrapper.Options, error) {
	wrapperOptions := &wrapper.Options{
		Args:          append(c.Command, c.Args...),
		ContainerName: c.Name,
		ProcessLog:    processLog(log, prefix),
		MarkerFile:    markerFile(log, prefix),
		MetadataFile:  metadataFile(log, prefix),
	}
	// TODO(fejta): use flags
	entrypointConfigEnv, err := entrypoint.Encode(entrypoint.Options{
		ArtifactDir:    artifactsDir(log),
		GracePeriod:    gracePeriod,
		Options:        wrapperOptions,
		Timeout:        timeout,
		AlwaysZero:     exitZero,
		PreviousMarker: previousMarker,
	})
	if err != nil {
		return nil, err
	}

	c.Command = []string{entrypointLocation(tools)}
	c.Args = nil
	c.Env = append(c.Env, KubeEnv(map[string]string{entrypoint.JSONConfigEnvVar: entrypointConfigEnv})...)
	c.VolumeMounts = append(c.VolumeMounts, log, tools)
	return wrapperOptions, nil
}

// PlaceEntrypoint will copy entrypoint from the entrypoint image to the tools volume
func PlaceEntrypoint(config *prowapi.DecorationConfig, toolsMount coreapi.VolumeMount) coreapi.Container {
	container := coreapi.Container{
		Name:         entrypointName,
		Image:        config.UtilityImages.Entrypoint,
		Command:      []string{"/bin/cp"},
		Args:         []string{"/entrypoint", entrypointLocation(toolsMount)},
		VolumeMounts: []coreapi.VolumeMount{toolsMount},
	}
	if config.Resources != nil && config.Resources.PlaceEntrypoint != nil {
		container.Resources = *config.Resources.PlaceEntrypoint
	}
	return container
}

func BlobStorageOptions(dc prowapi.DecorationConfig, localMode bool) ([]coreapi.Volume, []coreapi.VolumeMount, gcsupload.Options) {
	opt := gcsupload.Options{
		// TODO: pass the artifact dir here too once we figure that out
		GCSConfiguration: dc.GCSConfiguration,
		DryRun:           false,
	}
	if localMode {
		opt.LocalOutputDir = outputMountPath
		// The GCS credentials are not needed for local mode.
		return nil, nil, opt
	}

	var volumes []coreapi.Volume
	var mounts []coreapi.VolumeMount
	if dc.GCSCredentialsSecret != nil && *dc.GCSCredentialsSecret != "" {
		volumes = append(volumes, coreapi.Volume{
			Name: gcsCredentialsMountName,
			VolumeSource: coreapi.VolumeSource{
				Secret: &coreapi.SecretVolumeSource{
					SecretName: *dc.GCSCredentialsSecret,
				},
			},
		})
		mounts = append(mounts, coreapi.VolumeMount{
			Name:      gcsCredentialsMountName,
			MountPath: gcsCredentialsMountPath,
		})
		opt.StorageClientOptions.GCSCredentialsFile = fmt.Sprintf("%s/service-account.json", gcsCredentialsMountPath)
	}
	if dc.S3CredentialsSecret != nil && *dc.S3CredentialsSecret != "" {
		volumes = append(volumes, coreapi.Volume{
			Name: s3CredentialsMountName,
			VolumeSource: coreapi.VolumeSource{
				Secret: &coreapi.SecretVolumeSource{
					SecretName: *dc.S3CredentialsSecret,
				},
			},
		})
		mounts = append(mounts, coreapi.VolumeMount{
			Name:      s3CredentialsMountName,
			MountPath: s3CredentialsMountPath,
		})
		opt.StorageClientOptions.S3CredentialsFile = fmt.Sprintf("%s/service-account.json", s3CredentialsMountPath)
	}

	return volumes, mounts, opt
}

func InitUpload(config *prowapi.DecorationConfig, gcsOptions gcsupload.Options, blobStorageMounts []coreapi.VolumeMount, cloneLogMount *coreapi.VolumeMount, outputMount *coreapi.VolumeMount, encodedJobSpec string) (*coreapi.Container, error) {
	// TODO(fejta): remove encodedJobSpec
	initUploadOptions := initupload.Options{
		Options: &gcsOptions,
	}
	var mounts []coreapi.VolumeMount
	if cloneLogMount != nil {
		initUploadOptions.Log = CloneLogPath(*cloneLogMount)
		mounts = append(mounts, *cloneLogMount)
	}
	mounts = append(mounts, blobStorageMounts...)
	if outputMount != nil {
		mounts = append(mounts, *outputMount)
	}
	// TODO(fejta): use flags
	initUploadConfigEnv, err := initupload.Encode(initUploadOptions)
	if err != nil {
		return nil, fmt.Errorf("could not encode initupload configuration as JSON: %v", err)
	}
	container := &coreapi.Container{
		Name:    initUploadName,
		Image:   config.UtilityImages.InitUpload,
		Command: []string{"/initupload"}, // TODO(fejta): remove this, use image's entrypoint and delete /initupload symlink
		Env: KubeEnv(map[string]string{
			downwardapi.JobSpecEnv:      encodedJobSpec,
			initupload.JSONConfigEnvVar: initUploadConfigEnv,
		}),
		VolumeMounts: mounts,
	}
	if config.Resources != nil && config.Resources.InitUpload != nil {
		container.Resources = *config.Resources.InitUpload
	}
	return container, nil
}

// LogMountAndVolume returns the canonical volume and mount used to persist container logs.
func LogMountAndVolume() (coreapi.VolumeMount, coreapi.Volume) {
	return coreapi.VolumeMount{
			Name:      logMountName,
			MountPath: logMountPath,
		}, coreapi.Volume{
			Name: logMountName,
			VolumeSource: coreapi.VolumeSource{
				EmptyDir: &coreapi.EmptyDirVolumeSource{},
			},
		}
}

// CodeMountAndVolume returns the canonical volume and mount used to share code under test
func CodeMountAndVolume() (coreapi.VolumeMount, coreapi.Volume) {
	return coreapi.VolumeMount{
			Name:      codeMountName,
			MountPath: codeMountPath,
		}, coreapi.Volume{
			Name: codeMountName,
			VolumeSource: coreapi.VolumeSource{
				EmptyDir: &coreapi.EmptyDirVolumeSource{},
			},
		}
}

// ToolsMountAndVolume returns the canonical volume and mount used to propagate the entrypoint
func ToolsMountAndVolume() (coreapi.VolumeMount, coreapi.Volume) {
	return coreapi.VolumeMount{
			Name:      toolsMountName,
			MountPath: toolsMountPath,
		}, coreapi.Volume{
			Name: toolsMountName,
			VolumeSource: coreapi.VolumeSource{
				EmptyDir: &coreapi.EmptyDirVolumeSource{},
			},
		}
}

func decorate(spec *coreapi.PodSpec, pj *prowapi.ProwJob, rawEnv map[string]string, outputDir string) error {
	// TODO(fejta): we should pass around volume names rather than forcing particular mount paths.

	rawEnv[artifactsEnv] = artifactsPath
	rawEnv[gopathEnv] = codeMountPath // TODO(fejta): remove this once we can assume go modules
	logMount, logVolume := LogMountAndVolume()
	codeMount, codeVolume := CodeMountAndVolume()
	toolsMount, toolsVolume := ToolsMountAndVolume()

	// The output volume is only used if outputDir is specified, indicating the pod-utils should
	// copy files instead of uploading to GCS.
	localMode := outputDir != ""
	var outputMount *coreapi.VolumeMount
	var outputVolume *coreapi.Volume
	if localMode {
		outputMount = &coreapi.VolumeMount{
			Name:      outputMountName,
			MountPath: outputMountPath,
		}
		outputVolume = &coreapi.Volume{
			Name: outputMountName,
			VolumeSource: coreapi.VolumeSource{
				HostPath: &coreapi.HostPathVolumeSource{
					Path: outputDir,
				},
			},
		}
	}

	blobStorageVolumes, blobStorageMounts, blobStorageOptions := BlobStorageOptions(*pj.Spec.DecorationConfig, localMode)

	cloner, refs, cloneVolumes, err := CloneRefs(*pj, codeMount, logMount)
	if err != nil {
		return fmt.Errorf("create clonerefs container: %v", err)
	}
	var cloneLogMount *coreapi.VolumeMount
	if cloner != nil {
		spec.InitContainers = append([]coreapi.Container{*cloner}, spec.InitContainers...)
		cloneLogMount = &logMount
	}

	encodedJobSpec := rawEnv[downwardapi.JobSpecEnv]
	initUpload, err := InitUpload(pj.Spec.DecorationConfig, blobStorageOptions, blobStorageMounts, cloneLogMount, outputMount, encodedJobSpec)
	if err != nil {
		return fmt.Errorf("create initupload container: %v", err)
	}
	spec.InitContainers = append(
		spec.InitContainers,
		*initUpload,
		PlaceEntrypoint(pj.Spec.DecorationConfig, toolsMount),
	)
	for i, container := range spec.Containers {
		spec.Containers[i].Env = append(container.Env, KubeEnv(rawEnv)...)
	}

	const (
		previous = ""
		exitZero = false
	)
	var wrappers []wrapper.Options

	for i, container := range spec.Containers {
		prefix := container.Name
		if len(spec.Containers) == 1 {
			prefix = ""
		}
		wrapperOptions, err := InjectEntrypoint(&spec.Containers[i], pj.Spec.DecorationConfig.Timeout.Get(), pj.Spec.DecorationConfig.GracePeriod.Get(), prefix, previous, exitZero, logMount, toolsMount)
		if err != nil {
			return fmt.Errorf("wrap container: %v", err)
		}
		wrappers = append(wrappers, *wrapperOptions)
	}

	sidecar, err := Sidecar(pj.Spec.DecorationConfig, blobStorageOptions, blobStorageMounts, logMount, outputMount, encodedJobSpec, !RequirePassingEntries, !IgnoreInterrupts, wrappers...)
	if err != nil {
		return fmt.Errorf("create sidecar: %v", err)
	}

	spec.Volumes = append(spec.Volumes, logVolume, toolsVolume)
	spec.Volumes = append(spec.Volumes, blobStorageVolumes...)
	if outputVolume != nil {
		spec.Volumes = append(spec.Volumes, *outputVolume)
	}

	if len(refs) > 0 {
		for i, container := range spec.Containers {
			spec.Containers[i].WorkingDir = DetermineWorkDir(codeMount.MountPath, refs)
			spec.Containers[i].VolumeMounts = append(container.VolumeMounts, codeMount)
		}
		spec.Volumes = append(spec.Volumes, append(cloneVolumes, codeVolume)...)
	}

	spec.Containers = append(spec.Containers, *sidecar)

	if spec.TerminationGracePeriodSeconds == nil && pj.Spec.DecorationConfig.GracePeriod != nil {
		// Unless the user's asked for something specific, we want to set the grace period on the Pod to
		// a reasonable value, as the overall grace period for the Pod must encompass both the time taken
		// to gracefully terminate the test process *and* the time taken to process and upload the resulting
		// artifacts to the cloud. As a reasonable rule of thumb, assume a 80/20 split between these tasks.
		gracePeriodSeconds := int64(pj.Spec.DecorationConfig.GracePeriod.Seconds()) * 5 / 4
		spec.TerminationGracePeriodSeconds = &gracePeriodSeconds
	}

	defaultSA := pj.Spec.DecorationConfig.DefaultServiceAccountName
	if spec.ServiceAccountName == "" && defaultSA != nil {
		spec.ServiceAccountName = *defaultSA
	}

	return nil
}

// DetermineWorkDir determines the working directory to use for a given set of refs to clone
func DetermineWorkDir(baseDir string, refs []prowapi.Refs) string {
	for _, ref := range refs {
		if ref.WorkDir {
			return clone.PathForRefs(baseDir, ref)
		}
	}
	return clone.PathForRefs(baseDir, refs[0])
}

const (
	// RequirePassingEntries causes sidecar to return an error if any entry fails. Otherwise it exits cleanly so long as it can complete.
	RequirePassingEntries = true
	// IgnoreInterrupts causes sidecar to ignore interrupts and hope that the test process exits cleanly before starting an upload.
	IgnoreInterrupts = true
)

func Sidecar(config *prowapi.DecorationConfig, gcsOptions gcsupload.Options, blobStorageMounts []coreapi.VolumeMount, logMount coreapi.VolumeMount, outputMount *coreapi.VolumeMount, encodedJobSpec string, requirePassingEntries, ignoreInterrupts bool, wrappers ...wrapper.Options) (*coreapi.Container, error) {
	gcsOptions.Items = append(gcsOptions.Items, artifactsDir(logMount))
	sidecarConfigEnv, err := sidecar.Encode(sidecar.Options{
		GcsOptions:       &gcsOptions,
		Entries:          wrappers,
		EntryError:       requirePassingEntries,
		IgnoreInterrupts: ignoreInterrupts,
	})
	if err != nil {
		return nil, err
	}
	mounts := []coreapi.VolumeMount{logMount}
	mounts = append(mounts, blobStorageMounts...)
	if outputMount != nil {
		mounts = append(mounts, *outputMount)
	}

	container := &coreapi.Container{
		Name:    sidecarName,
		Image:   config.UtilityImages.Sidecar,
		Command: []string{"/sidecar"}, // TODO(fejta): remove, use image's entrypoint
		Env: KubeEnv(map[string]string{
			sidecar.JSONConfigEnvVar: sidecarConfigEnv,
			downwardapi.JobSpecEnv:   encodedJobSpec, // TODO: shouldn't need this?
		}),
		VolumeMounts: mounts,
	}
	if config.Resources != nil && config.Resources.Sidecar != nil {
		container.Resources = *config.Resources.Sidecar
	}
	return container, nil
}

// KubeEnv transforms a mapping of environment variables
// into their serialized form for a PodSpec, sorting by
// the name of the env vars
func KubeEnv(environment map[string]string) []coreapi.EnvVar {
	var keys []string
	for key := range environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var kubeEnvironment []coreapi.EnvVar
	for _, key := range keys {
		kubeEnvironment = append(kubeEnvironment, coreapi.EnvVar{
			Name:  key,
			Value: environment[key],
		})
	}

	return kubeEnvironment
}
