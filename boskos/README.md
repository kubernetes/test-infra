# WIP boskos


## Background
[βοσκός](https://en.wiktionary.org/wiki/%CE%B2%CE%BF%CF%83%CE%BA%CF%8C%CF%82) - shepherd in greek!

boskos is a resource manager service, that handles and manages different kind of resources and transition between different states.

## Introduction

Boskos is inited with a config of resources, one JSON entry per line, from `-config` flag

A resource object looks like
```
type Resource struct {
	Type       string    `json:"type"`
	Name       string    `json:"name"`
	State      string    `json:"state"`
	Owner      string    `json:"owner"`
	LastUpdate time.Time `json:"lastupdate"`
}
```

Type can be GCPProject, cluster, or even a dota2 server, anything that you want to be a group of resources.
Name is a unique identifier of the resource.
State is a string that tells the current status of the resource.

## API

1. 	URL: /start
	Desc: Use /start when you want to get hold of some resource.
	Method: GET/POST
	URL Params: 
		Required: type=[string] : type of requested resource
		Required: state=[string] : state of the requested resource
		Required: owner=[string] : requester of the resource
	Return: error code or 200 with a valid Resource JSON object.
	Example: /start?type=project&state=free&owner=senlu

2.	URL: /done
	Desc: use /done when you finish use some resource.
	Method: GET/POST
	URL Params:
		Required: name=[string] : name of finished resource
		Required: state=[string] : dest state
	Return: status code
	Example: /done?name=k8s-jkns-foo&state=dirty

3.	URL: /update
	Desc: Update resource last-update timestamp.
	Method: GET/POST
	URL Params:
		Required: name=[string] : name of target resource
	Return: status code

4.	URL: /reset
	Desc: Reset a group of expired resource to certain state.
	Method: GET/POST
	URL Params:
		Required: type=[string] : type of resource in interest
		Required: state=[string] : original state
		Required: dest=[string] : dest state, for expired resource
		Required: expire=[durationStr*]
			*durationStr is any string can be parsed by [time.ParseDuration()](https://golang.org/pkg/time/#ParseDuration)
		Return: error code, or a list of [Owner:Resource] pairs.

5.	URL: /list
	Method: GET/POST
	URL Params: 
		Required: type=[string] : type of requested resource

	Return: error code or JSON object with: 
			Count of projects in each state
			Sum of state moved to after /done
			Count of projects with each owner (or without an owner)
			A sample object will look like:
			{
				“type” : “project”
				“Status”: 
				{
					“total”   : 35 
					“free”    : 20
					“dirty”   : 10
					“injured” : 5
				}
	
				“Moved to”:
				{
					“free”:    600
					“dirty”:   500
					“injured”: 100
				}

				“Owner”:
				{
					“fejta” : 1
					“Senlu” : 1
					“sig-testing” : 20
					“Janitor” : 10
					“None” : 20
				}
			}

```

## Local test:
1. Shoot it up with a fake resources.json, with `go run boskos.go -config=/path/to/resources.json`

1. Punt it with local requests:
```
curl 'http://127.0.0.1:8080/start?type=project&state=free&owner=user'
```

## K8s test:
1. Create and navigate to your own cluster

1. `make deployment`

1. `make service`

1. `kubectl create configmap projects --from-file=config=projects`

1. `kubectl describe svc boskos` to make sure boskos is running

1. Test from another pod within the cluster
```
kubectl run curl --image=radial/busyboxplus:curl -i --tty
Waiting for pod default/curl-XXXXX to be running, status is Pending, pod ready: false
If you don't see a command prompt, try pressing enter.
[ root@curl-XXXXX:/ ]$ curl 'http://boskos/request?type=project&state=free&owner=user'
````
