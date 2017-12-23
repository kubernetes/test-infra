# Simple Leader Election with Kubernetes and Docker
Implementing leader election in Kubernetes 

The first requirement in leader election is the specification of the set of candidates for becoming the leader. Kubernetes already uses Endpoints to represent a replicated set of pods that comprise a service, so we will re-use this same object. (aside: You might have thought that we would use ReplicationControllers, but they are tied to a specific binary, and generally you want to have a single leader even if you are in the process of performing a rolling update)

To perform leader election, we use two properties of all Kubernetes API objects:

ResourceVersions - Every API object has a unique ResourceVersion, and you can use these versions to perform compare-and-swap on Kubernetes objects 
Annotations - Every API object can be annotated with arbitrary key/value pairs to be used by clients. 

Given these primitives, the code to use master election is relatively straightforward, and you can find it here. Let’s run it ourselves. 

```console
$ kubectl run leader-elector --image=gcr.io/google_containers/leader-elector:0.5 --replicas=3 -- --election=example
```

This creates a leader election set with 3 replicas:

```console
$ kubectl get pods
NAME                   READY     STATUS    RESTARTS   AGE
leader-elector-inmr1   1/1       Running   0          13s
leader-elector-qkq00   1/1       Running   0          13s
leader-elector-sgwcq   1/1       Running   0          13s
```

To see which pod was chosen as the leader, you can access the logs of one of the pods, substituting one of your own pod’s names in place of 
`${pod_name}`, (e.g. leader-elector-inmr1 from the above)

```console
$ kubectl logs -f ${pod_name}
```

Alternately, you can inspect the endpoints object directly: 

```console
# ‘example’ is the name of the candidate set from the above kubectl run … command
$ kubectl get endpoints example -o yaml
```

Now to validate that leader election actually works, in a different terminal, run: 

```console
$ kubectl delete pods ${leader-pod-name}
```

This will delete the existing leader. Because the set of pods is being managed by a replication controller, a new pod replaces the one that was deleted, ensuring that the size of the replicated set is still three. Via leader election one of these three pods is selected as the new leader, and you should see the leader failover to a different pod. Because pods in Kubernetes have a grace period before termination, this may take 30-40 seconds.

The leader-election container provides a simple webserver that can serve on any address (e.g. http://localhost:4040). You can test this out by deleting the existing leader election group and creating a new one where you additionally pass in a --http=(host):(port) specification to the leader-elector image. This causes each member of the set to serve information about the leader via a webhook.

```console
# delete the old leader elector group
$ kubectl delete rc leader-elector

# create the new group, note the --http=localhost:4040 flag
$ kubectl run leader-elector --image=gcr.io/google_containers/leader-elector:0.5 --replicas=3 -- --election=example --http=0.0.0.0:4040

# create a proxy to your Kubernetes api server
$ kubectl proxy
```

You can then access:

`http://localhost:8001/api/v1/proxy/namespaces/default/pods/<leader-pod-name>:4040/`
And you will see:

`{"name":"(name-of-leader-here)"}`
Leader election with sidecars 

Ok, that’s great, you can do leader election and find out the leader over HTTP, but how can you use it from your own application? This is where the notion of sidecars come in. In Kubernetes, Pods are made up of one or more containers. Often times, this means that you add sidecar containers to your main application to make up a Pod. (for a much more detailed treatment of this subject see my earlier blog post).

The leader-election container can serve as a sidecar that you can use from your own application. Any container in the Pod that’s interested in who the current master is can simply access http://localhost:4040 and they’ll get back a simple JSON object that contains the name of the current master. Since all containers in a Pod share the same network namespace, there’s no service discovery required!

For example, here is a simple Node.js application that connects to the leader election sidecar and prints out whether or not it is currently the master. The leader election sidecar sets its identifier to `hostname` by default. 

```javascript
const http = require('http');
// This will hold info about the current master
let master = {};

// A callback that is used for our outgoing client requests to the sidecar
const cb = (response) => {
  let data = '';
  response.on('data', (piece) => {
    data = data + piece;
  });
  response.on('end', () => {
    master = JSON.parse(data);
  });
};

// Make an async request to the sidecar at http://localhost:4040
const updateMaster = function updateMaster() {
  const req = http.get({
    host: 'localhost',
    path: '/',
    port: 4040,
  }, cb);

  req.on('error', (e) => {
    console.log(`problem with request: ${e.message}`);
  });
  req.end();
};

updateMaster();

// Set up regular updates
setInterval(updateMaster, 5000);

// set up the web server
const www = http.createServer((request, response) => {
  response.writeHead(200);
  response.end(`Master is ${master.name}`);
});

www.listen(8080);
```

Test the client is running:
```
$ kubectl exec elector-sidecar -c nodejs -- wget -qO- http://localhost:8080
Master is elector-sidecar
```

Of course, you can use this sidecar from any language that you choose that supports HTTP and JSON.
