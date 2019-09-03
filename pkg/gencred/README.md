# Gencred

## Description

`gencred` is a simple tool used to generate cluster auth credentials (w/ [**cluster-admin**](https://kubernetes.io/docs/reference/access-authn-authz/rbac/#user-facing-roles) permissions) for authenticating to a Kubernetes cluster.
> **NOTE:** since `gencred` creates credentials with `cluster-admin` level access, the kube context used **must** also be bound to the `cluster-admin` role.

## Usage

### Generate a service account token for a cluster. 
✅ **PREFERRED** method.

```go
// Import gencred
import "k8s.io/test-infra/pkg/gencred"

//...

// Create a Kubernetes clientset for interacting with the cluster.
// In this case we are simply using the `current-context` defined in our local `~/.kube/config`.
homedir, _ := os.UserHomeDir()
kubeconfig := filepath.Join(homedir, ".kube", "config")
config, _ := clientcmd.BuildConfigFromFlags("", kubeconfig)
clientset, _ := kubernetes.NewForConfig(config)

// Generate a service account token, as well as return the certificate authority that issued the token.
token, caPEM, err := gencred.CreateClusterServiceAccountCredentials(clientset)
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

### Generate a client key and certificate for a cluster.
❌ **DEPRECATED** method.

```go
// Import gencred
import "k8s.io/test-infra/pkg/gencred"

//...

// Create a Kubernetes clientset for interacting with the cluster.
// In this case we are simply using the `current-context` defined in our local `~/.kube/config`.
homedir, _ := os.UserHomeDir()
kubeconfig := filepath.Join(homedir, ".kube", "config")
config, _ := clientcmd.BuildConfigFromFlags("", kubeconfig)
clientset, _ := kubernetes.NewForConfig(config)

// Generate a client key and certificate, as well as return the certificate authority that issued the certificate.
certPEM, keyPEM, caPEM, err := gencred.CreateClusterCertificateCredentials(clientset)
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