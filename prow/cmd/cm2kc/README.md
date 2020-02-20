# cm2kc (clustermap to kubeconfig)

## Description

`cm2kc` is a CLI tool used to convert a [clustermap file][clustermap docs] to a [kubeconfig file][kubeconfig docs].

## Usage

```shell
bazel run //prow/cmd/cm2kc -- <options>
```

The following is a list of supported options for `cm2kc`:

```console
  -i, --input string    Input clustermap file. (default "/dev/stdin")
  -o, --output string   Output kubeconfig file. (default "/dev/stdout")
```

## Examples

#### Create a kubeconfig file at path `/path/to/kubeconfig.yaml` from a clustermap file at path `/path/to/clustermap.yaml`.

Ensure the *clustermap* file exists at the specified `--input` path:  
 
```yaml
# /path/to/clustermap.yaml

default:
  clientCertificate: fake-default-client-cert
  clientKey: fake-default-client-key
  clusterCaCertificate: fake-default-ca-cert
  endpoint: https://1.2.3.4
build:
  clientCertificate: fake-build-client-cert
  clientKey: fake-build-client-key
  clusterCaCertificate: fake-build-ca-cert
  endpoint: https://5.6.7.8
```

Execute `cm2kc` specifying an `--input` path to the *clustermap* file and an `--output` path to the desired location of the generated *kubeconfig* file: 

```shell
bazel run //prow/cmd/cm2kc -- --input=/path/to/clustermap.yaml --output=/path/to/kubeconfig.yaml
```

The following *kubeconfig* file will be created at the specified `--output` path:  

```yaml
# /path/to/kubeconfig.yaml

apiVersion: v1
clusters:
- name: default
  cluster:
    certificate-authority-data: fake-default-ca-cert
    server: https://1.2.3.4
- name: build
  cluster:
    certificate-authority-data: fake-build-ca-cert
    server: https://5.6.7.8
contexts:
- name: default
  context:
    cluster: default
    user: default
- name: build
  context:
    cluster: build
    user: build
current-context: default
kind: Config
preferences: {}
users:
- name: default
  user:
    client-certificate-data: fake-default-ca-cert
    client-key-data: fake-default-ca-cert
- name: build
  user:
    client-certificate-data: fake-build-ca-cert
    client-key-data: fake-build-ca-cert
```

[clustermap docs]: https://github.com/kubernetes/test-infra/blob/1c7d9a4ae0f2ae1e0c11d8357f47163d18521b84/prow/getting_started_deploy.md#run-test-pods-in-different-clusters
[kubeconfig docs]: https://kubernetes.io/docs/tasks/access-application-cluster/configure-access-multiple-clusters/