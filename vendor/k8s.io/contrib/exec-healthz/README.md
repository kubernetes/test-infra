# Exec healthz server

The exec healthz server is a sidecar container meant to serve as a liveness-exec-over-http bridge. It isolates pods from the idiosyncrasies of container runtime exec implementations.

## How to release:

The `exechealthz` Makefile supports multiple architecures, which means it may cross-compile and build an docker image easily.
If you are releasing a new version, please bump the `TAG` value in the `Makefile` before building the images.

How to build and push all images:
```
# Build for linux/amd64 (default)
$ make push TAG=1.0
$ make push TAG=1.0 ARCH=amd64
# ---> gcr.io/google_containers/exechealthz-amd64:1.0

$ make push-legacy TAG=1.0 ARCH=amd64
# ---> gcr.io/google_containers/exechealthz:1.0 (image with backwards compatible naming)

$ make push TAG=1.0 ARCH=arm
# ---> gcr.io/google_containers/exechealthz-arm:1.0

$ make push TAG=1.0 ARCH=arm64
# ---> gcr.io/google_containers/exechealthz-arm64:1.0

$ make push TAG=1.0 ARCH=ppc64le
# ---> gcr.io/google_containers/exechealthz-ppc64le:1.0
```
Of course, if you don't want to push the images, just run `make` or `make container`

## Examples:

### Run the healthz server directly on localhost:

```sh
$ make server
$ ./exechealthz --cmd "ls /tmp/test"
$ curl http://localhost:8080/healthz
Healthz probe error: Result of last exec: ls: cannot access /tmp/test: No such file or directory
, at 2015-07-08 17:59:45.698036238 -0700 PDT, error exit status 2
$ touch /tmp/test
$ curl http://localhost:8080/healthz
ok
```

### Commands for running healthz server on multiple URLs and commands:

```
$ ./exechealthz --cmd="ls /tmp/test1" --url="/healthz1" --cmd="ls /tmp/test2" --url="/healthz2"
```
The `--url` flag indicates the path healthz server needs to serve on.
Notes: Number of commands and URLs have to be the same (if more than one). URL need to start with "/". URLs and cmds match up based on their orders (first URL to first cmd).

### Run the healthz server in a docker container:

The [docker daemon](https://docs.docker.com/userguide/) needs to be running on your host.
```sh
$ make container PREFIX=mycontainer/test
$ docker run -itP -p 8080:8080 mycontainer/test:0.0 -cmd "ls /tmp/test"
$ curl http://localhost:8080/healthz
Healthz probe error: Result of last exec: ls: cannot access /tmp/test: No such file or directory
, at 2015-07-08 18:00:57.698103532 -0700 PDT, error exit status 2

$ docker ps
CONTAINER ID        IMAGE                  COMMAND                 CREATED             STATUS              PORTS                    NAMES
8e86f8accfa6        mycontainer/test:0.0   "/exechealthz -cm"   27 seconds ago      Up 26 seconds       0.0.0.0:8080->8080/tcp   loving_albattani
$ docker exec -it 8e86f8accfa6 touch /tmp/test
$ curl http://localhost:8080/healthz
ok
```

### Run the healthz server in a kubernetes pod:

You need a running [kubernetes cluster](../../docs/getting-started-guides/README.md).
Create a pod.json that looks like:
```json
{
  "kind": "Pod",
  "apiVersion": "v1",
  "metadata": {
    "name": "simple"
  },
  "spec": {
    "containers": [
      {
        "name": "healthz",
        "image": "gcr.io/google_containers/exechealthz:1.0",
        "args": [
          "-cmd=nslookup localhost"
        ],
        "ports": [
          {
            "containerPort": 8080,
            "protocol": "TCP"
          }
        ]
      }
    ]
  }
}
```

And run the pod on your cluster using kubectl:
```sh
$ kubectl create -f pod.json
pods/simple
$ kubectl get pods -o wide
NAME     READY     STATUS    RESTARTS   AGE  NODE
simple   0/1       Pending   0          3s   node
```

SSH into the node (note that the recommended way to access a server in a container is through a [service](../../docs/services.md), the example that follows is just to illustrate how the kubelet performs an http liveness probe):
```sh
node$ kubectl get pods simple -o json | grep podIP
"podIP": "10.1.0.2",

node$ curl http://10.1.0.2:8080/healthz
ok
```

### Run the healthz server as a sidecar container for liveness probes of another container:
Create a pod.json with 2 containers, one of which is the healthz probe and the other, the container being health checked. The
pod.json example file in this directory does exactly that. If you create the pod the same way you created the pod in the previous
example, the kubelet on the node will periodically perform a health check similar to what you did manually and restart the container
when it fails. Explore [liveness probes](../../examples/liveness/README.md).

## Debugging

You can run exechealthz locally, to poke and prod at it:
```console
$ go build exechealthz.go
$ ./exechealthz -cmd="nslookup google.com > /dev/null" -period=10ms
```

The container exposes pprof handlers on the same port it exposes /healthz (8080 by default). You can get runtime stats as [documented here](https://golang.org/pkg/net/http/pprof/), i.e curl the various pprof handlers:
```console
$ curl http://localhost:8080/debug/pprof/
$ http://localhost:8080/debug/pprof/goroutine?debug=1
$ http://localhost:8080/debug/pprof/heap?debug=1
```

## Limitations:
* Doesn't handle sigterm, which means docker stop on this container can take longer than it needs to.
* Doesn't sanity check the probe command. You should set the -period and -latency parameters of exechealthz appropriately.
* Only ever returns 503 or 200.


[![Analytics](https://kubernetes-site.appspot.com/UA-36037335-10/GitHub/contrib/exec-healthz/README.md?pixel)]()
