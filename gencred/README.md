# Gencred

## Description

`gencred` is a simple tool used to generate cluster auth credentials (w/ [**cluster-admin**](https://kubernetes.io/docs/reference/access-authn-authz/rbac/#user-facing-roles) permissions) for authenticating to a Kubernetes cluster.
> **NOTE:** since `gencred` creates credentials with `cluster-admin` level access, the kube context used **must** also be bound to the `cluster-admin` role.

## Usage

### Script

Run using Bazel:

```console
$ bazel run //gencred -- <options>
```

Run using Golang:

```console
$ go run k8s.io/test-infra/gencred <options>
```

The following is a list of supported options for the `gencred` CLI. All options are *optional*. 

```console
  -c, --certificate      Authorize with a client certificate and key.
      --context string   The name of the kubeconfig context to use.
  -n, --name string      Context name for the kubeconfig entry. (default "build")
  -o, --output string    Output path for generated kubeconfig file. (default "/dev/stdout")
      --overwrite        Overwrite (rather than merge) output file if exists.
  -s, --serviceaccount   Authorize with a service account. (default true)
```

Create a kubeconfig entry with context name `mycluster` using `serviceaccount` authorization and output to a file `config.yaml`.
> `serviceaccount` authorization is the *default* if neither `-s, --serviceaccount` nor `-c, --certificate` is specified.
 
```console
$ gencred --context my-current-context --name mycluster --output ./config.yaml --serviceaccount
```

The kubeconfig contents will be `output` to  `./config.yaml`:

```yaml
apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: fake-ca-data
    server: https://1.2.3.4
  name: mycluster
contexts:
- context:
    cluster: mycluster
    user: mycluster
  name: mycluster
current-context: mycluster
kind: Config
preferences: {}
users:
- name: mycluster
  user:
    token: fake-token
```

Create a kubeconfig entry with **default** context name `build` using `certificate` authorization and output to the **default** `stdout`.

```console
$ gencred --context my-current-context --certificate
```

The kubeconfig contents will be `output` to `stdout`:

```yaml
apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: fake-ca-data
    server: https://1.2.3.4
  name: build
contexts:
- context:
    cluster: build
    user: build
  name: build
current-context: build
kind: Config
preferences: {}
users:
- name: build
  user:
    client-certificate-data: fake-cert-data
    client-key-data: fake-key-data
```

Specify the `--overwrite` flag to *replace* the `output` file if it exists.

```console
$ gencred --context my-current-context --output ./existing.yaml --overwrite
```

Omit the `--overwrite` flag to *merge* the `output` file if it exists.
> Entries from the *existing* file take precedence on conflicts.

```console
$ gencred --context my-current-context --name oldcluster --output ./existing.yaml
$ gencred --context my-current-context --name newcluster --output ./existing.yaml
```

The kubeconfig contents will be `output` to  `./existing.yaml`:

```yaml
apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: fake-ca-data
    server: https://1.2.3.4
  name: oldcluster
- cluster:
    certificate-authority-data: fake-ca-data
    server: https://1.2.3.4
  name: newcluster
contexts:
- context:
    cluster: oldcluster
    user: oldcluster
  name: oldcluster
- context:
    cluster: newcluster
    user: newcluster
  name: newcluster
users:
- name: oldcluster
  user:
    client-certificate-data: fake-cert-data
    client-key-data: fake-key-data
- name: newcluster
  user:
    client-certificate-data: fake-cert-data
    client-key-data: fake-key-data
```

#### Merging into a kubeconfig in a Kubernetes secret.
If you store kubeconfig files in kubernetes secrets to allow pods to access other kubernetes clusters (like many of Prow's components require) consider using [`merge_kubeconfig_secret.py`](/gencred/merge_kubeconfig_secret.py) to merge the kubeconfig produced by `gencred` into the secret.

```shell
# Generate a kubeconfig.yaml as described and shown above.
./merge_kubeconfig_secret.py --auto --context=my-kube-context kubeconfig.yaml
# Note: The first time the script is used you may be prompted to rerun it with --src-key specified.
# Finish by updating references (e.g. `--kubeconfig` flags in Prow deployment files) to point to the updated secret key. The script will indicate which key was updated in its output.
```

The script exposes optional flags to override the secret namespace, name, keys, and pruning behavior. Run `./merge_kubeconfig_secret.py --help` to view all options.

### Library

#### Generate a service account token for a cluster. 
✅ **PREFERRED** method.

```go
// Import serviceaccount
import "k8s.io/test-infra/gencred/pkg/serviceaccount"

//...

// Create a Kubernetes clientset for interacting with the cluster.
// In this case we are simply using the `current-context` defined in our local `~/.kube/config`.
homedir, _ := os.UserHomeDir()
kubeconfig := filepath.Join(homedir, ".kube", "config")
config, _ := clientcmd.BuildConfigFromFlags("", kubeconfig)
clientset, _ := kubernetes.NewForConfig(config)

// Generate a service account token, as well as return the certificate authority that issued the token.
token, caPEM, err := serviceaccount.CreateClusterServiceAccountCredentials(clientset)
```  

`token` will contain the **service account access token** and `caPEM` will contain the **server certificate authority**.

```go
import "encoding/base64"

//...

// Cast the `token` to a string to use in a kubeconfig.
accessToken := string(token)
// Base64 encode the `caPEM` to use in a kubeconfig.
ca := base64.StdEncoding.EncodeToString(caPEM)

fmt.Println("token:", accessToken)
fmt.Println("ca:", ca)
```

```text
token: eyJhbGciOiJSUzI1NiIsImtpZCI6IiJ9.eyJpc3Mit...
ca: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURER...
```

#### Generate a client key and certificate for a cluster.
❌ **DEPRECATED** method.

```go
// Import certificate
import "k8s.io/test-infra/gencred/pkg/certificate"

//...

// Create a Kubernetes clientset for interacting with the cluster.
// In this case we are simply using the `current-context` defined in our local `~/.kube/config`.
homedir, _ := os.UserHomeDir()
kubeconfig := filepath.Join(homedir, ".kube", "config")
config, _ := clientcmd.BuildConfigFromFlags("", kubeconfig)
clientset, _ := kubernetes.NewForConfig(config)

// Generate a client key and certificate, as well as return the certificate authority that issued the certificate.
certPEM, keyPEM, caPEM, err := certificate.CreateClusterCertificateCredentials(clientset)
```  

`certPEM` will contain the **client certificate**, `keyPEM` will contain the **client key**, and `caPEM` will contain the **server certificate authority**.

```go
import "encoding/base64"

//...

// Base64 encode the `certPEM`, `keyPEM`, and `caPEM` to use in a kubeconfig.
cert := base64.StdEncoding.EncodeToString(certPEM)
key := base64.StdEncoding.EncodeToString(keyPEM)
ca := base64.StdEncoding.EncodeToString(caPEM)

fmt.Println("cert:", cert)
fmt.Println("key:", key)
fmt.Println("ca:", ca)
```

```text
cert: LS0tLS1CRUdJTiBFQyBQUklWQVRFIEtFWS0tLS0tCk1...
key: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNL...
ca: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURER...
```

#### Caveats to using client certificates:
* The use of [x509 client certificate](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#x509-client-certs) with [super-user](https://kubernetes.io/docs/reference/access-authn-authz/rbac/#user-facing-roles) privileges for cluster authentication/authorization has several drawbacks:
    - Certificates **cannot** be revoked ([kubernetes/kubernetes#60917](https://github.com/kubernetes/kubernetes/issues/60917))
    - Authorization roles are essentially *global* and thus **cannot** be tweaked at the node level.
    - Unless setup with near expiry and explicit rotation, certificates are *long-lived* and **increase** the risk of exposure.

* Client certificate authentication will be deprecated in future versions of Prow  ([kubernetes/test-infra#13972](https://github.com/kubernetes/test-infra/issues/13972)).