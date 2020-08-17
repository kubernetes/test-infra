# Kubernetes Alert Runbooks

As Rob Ewaschuk [puts it](https://docs.google.com/document/d/199PqyG3UsyXlwieHaqbGiWVa8eMWi8zzAn0YfcApr8Q/edit#):
> Playbooks (or runbooks) are an important part of an alerting system; it's best to have an entry for each alert or family of alerts that catch a symptom, which can further explain what the alert means and how it might be addressed.

It is a recommended practice that you add an annotation of "runbook" to every prometheus alert with a link to a clear description of it's meaning and suggested remediation or mitigation. While some problems will require private and custom solutions, most common problems have common solutions. In practice, you'll want to automate many of the procedures (rather than leaving them in a wiki), but even a self-correcting problem should provide an explanation as to what happened and why to observers.

Matthew Skelton & Rob Thatcher have an excellent [run book template](https://github.com/SkeltonThatcher/run-book-template). This template will help teams to fully consider most aspects of reliably operating most interesting software systems, if only to confirm that "this section definitely does not apply here" - a valuable realization.

This page collects this repositories alerts and begins the process of describing what they mean and how it might be addressed. Links from alerts to this page are added [automatically](https://github.com/kubernetes-monitoring/kubernetes-mixin/blob/master/alerts/add-runbook-links.libsonnet).

### Group Name: "kubernetes-absent"
##### Alert Name: "KubeAPIDown"
+ *Message*: `KubeAPI has disappeared from Prometheus target discovery.`
+ *Severity*: critical
##### Alert Name: "KubeControllerManagerDown"
+ *Message*: `KubeControllerManager has disappeared from Prometheus target discovery.`
+ *Severity*: critical
+ *Runbook*: [Link](https://coreos.com/tectonic/docs/latest/troubleshooting/controller-recovery.html#recovering-a-controller-manager)
##### Alert Name: KubeSchedulerDown
+ *Message*: `KubeScheduler has disappeared from Prometheus target discovery`
+ *Severity*: critical
+ *Runbook*: [Link](https://coreos.com/tectonic/docs/latest/troubleshooting/controller-recovery.html#recovering-a-scheduler)
##### Alert Name: KubeletDown
+ *Message*: `Kubelet has disappeared from Prometheus target discovery.`
+ *Severity*: critical
### Group Name: kubernetes-apps
##### Alert Name: KubePodCrashLooping
+ *Message*: `{{ $labels.namespace }}/{{ $labels.pod }} ({{ $labels.container }}) is restarting {{ printf \"%.2f\" $value }} / second`
+ *Severity*: critical
##### Alert Name: "KubePodNotReady"
+ *Message*: `{{ $labels.namespace }}/{{ $labels.pod }} is not ready.`
+ *Severity*: critical
##### Alert Name: "KubeDeploymentGenerationMismatch"
+ *Message*: `Deployment {{ $labels.namespace }}/{{ $labels.deployment }} generation mismatch`
+ *Severity*: critical
##### Alert Name: "KubeDeploymentReplicasMismatch"
+ *Message*: `Deployment {{ $labels.namespace }}/{{ $labels.deployment }} replica mismatch`
+ *Severity*: critical
##### Alert Name: "KubeStatefulSetReplicasMismatch"
+ *Message*: `StatefulSet {{ $labels.namespace }}/{{ $labels.statefulset }} replica mismatch`
+ *Severity*: critical
##### Alert Name: "KubeStatefulSetGenerationMismatch"
+ *Message*: `StatefulSet {{ $labels.namespace }}/{{ $labels.statefulset }} generation mismatch`
+ *Severity*: critical
##### Alert Name: "KubeDaemonSetRolloutStuck"
+ *Message*: `Only {{$value}}% of desired pods scheduled and ready for daemon set {{$labels.namespace}}/{{$labels.daemonset}}`
+ *Severity*: critical
##### Alert Name: "KubeDaemonSetNotScheduled"
+ *Message*: `A number of pods of daemonset {{$labels.namespace}}/{{$labels.daemonset}} are not scheduled.`
+ *Severity*: warning

##### Alert Name: "KubeDaemonSetMisScheduled"
+ *Message*: `A number of pods of daemonset {{$labels.namespace}}/{{$labels.daemonset}} are running where they are not supposed to run.`
+ *Severity*: warning

##### Alert Name: "KubeCronJobRunning"
+ *Message*: `CronJob {{ $labels.namespace }}/{{ $labels.cronjob }} is taking more than 1h to complete.`
+ *Severity*: warning
+ *Action*: Check the cronjob using `kubectl describe cronjob <cronjob>` and look at the pod logs using `kubectl logs <pod>` for further information.

##### Alert Name: "KubeJobCompletion"
+ *Message*: `Job {{ $labels.namespace }}/{{ $labels.job_name }} is taking more than 1h to complete.`
+ *Severity*: warning
+ *Action*: Check the job using `kubectl describe job <job>` and look at the pod logs using `kubectl logs <pod>` for further information.

##### Alert Name: "KubeJobFailed"
+ *Message*: `Job {{ $labels.namespace }}/{{ $labels.job_name }} failed to complete.`
+ *Severity*: warning
+ *Action*: Check the job using `kubectl describe job <job>` and look at the pod logs using `kubectl logs <pod>` for further information.

### Group Name: "kubernetes-resources"
##### Alert Name: "KubeCPUOvercommit"
+ *Message*: `Overcommited CPU resource requests on Pods, cannot tolerate node failure.`
+ *Severity*: warning
##### Alert Name: "KubeMemOvercommit"
+ *Message*: `Overcommited Memory resource requests on Pods, cannot tolerate node failure.`
+ *Severity*: warning
##### Alert Name: "KubeCPUOvercommit"
+ *Message*: `Overcommited CPU resource request quota on Namespaces.`
+ *Severity*: warning
##### Alert Name: "KubeMemOvercommit"
+ *Message*: `Overcommited Memory resource request quota on Namespaces.`
+ *Severity*: warning
##### Alert Name: "KubeQuotaExceeded"
+ *Message*: `{{ printf \"%0.0f\" $value }}% usage of {{ $labels.resource }} in namespace {{ $labels.namespace }}.`
+ *Severity*: warning
### Group Name: "kubernetes-storage"
##### Alert Name: "KubePersistentVolumeUsageCritical"
+ *Message*: `The persistent volume claimed by {{ $labels.persistentvolumeclaim }} in namespace {{ $labels.namespace }} has {{ printf \"%0.0f\" $value }}% free.`
+ *Severity*: critical
##### Alert Name: "KubePersistentVolumeFullInFourDays"
+ *Message*: `Based on recent sampling, the persistent volume claimed by {{ $labels.persistentvolumeclaim }} in namespace {{ $labels.namespace }} is expected to fill up within four days.`
+ *Severity*: critical
### Group Name: "kubernetes-system"
##### Alert Name: "KubeNodeNotReady"
+ *Message*: `{{ $labels.node }} has been unready for more than an hour"`
+ *Severity*: warning
##### Alert Name: "KubeVersionMismatch"
+ *Message*: `There are {{ $value }} different versions of Kubernetes components running.`
+ *Severity*: warning
##### Alert Name: "KubeClientErrors"
+ *Message*: `Kubernetes API server client '{{ $labels.job }}/{{ $labels.instance }}' is experiencing {{ printf \"%0.0f\" $value }}% errors.'`
+ *Severity*: warning
##### Alert Name: "KubeClientErrors"
+ *Message*: `Kubernetes API server client '{{ $labels.job }}/{{ $labels.instance }}' is experiencing {{ printf \"%0.0f\" $value }} errors / sec.'`
+ *Severity*: warning
##### Alert Name: "KubeletTooManyPods"
+ *Message*: `Kubelet {{$labels.instance}} is running {{$value}} pods, close to the limit of 110.`
+ *Severity*: warning
##### Alert Name: "KubeAPILatencyHigh"
+ *Message*: `The API server has a 99th percentile latency of {{ $value }} seconds for {{$labels.verb}} {{$labels.resource}}.`
+ *Severity*: warning
##### Alert Name: "KubeAPILatencyHigh"
+ *Message*: `The API server has a 99th percentile latency of {{ $value }} seconds for {{$labels.verb}} {{$labels.resource}}.`
+ *Severity*: critical
##### Alert Name: "KubeAPIErrorsHigh"
+ *Message*: `API server is erroring for {{ $value }}% of requests.`
+ *Severity*: critical
##### Alert Name: "KubeAPIErrorsHigh"
+ *Message*: `API server is erroring for {{ $value }}% of requests.`
+ *Severity*: warning
##### Alert Name: "KubeClientCertificateExpiration"
+ *Message*: `A client certificate used to authenticate to the apiserver is expiring in less than 7 days.`
+ *Severity*: warning
##### Alert Name: "KubeClientCertificateExpiration"
+ *Message*: `A client certificate used to authenticate to the apiserver is expiring in less than 1 day.`
+ *Severity*: critical

## Other Kubernetes Runbooks and troubleshooting
+ [Troubleshoot Clusters ](https://kubernetes.io/docs/tasks/debug-application-cluster/debug-cluster/)
+ [Cloud.gov Kubernetes Runbook ](https://cloud.gov/docs/ops/runbook/troubleshooting-kubernetes/)
+ [Recover a Broken Cluster](https://codefresh.io/Kubernetes-Tutorial/recover-broken-kubernetes-cluster/)
