# WIP boskos


## Background
[βοσκός](https://en.wiktionary.org/wiki/%CE%B2%CE%BF%CF%83%CE%BA%CF%8C%CF%82) - shepherd in greek!

boskos is a resource manager service, that handles and manages different kind of resources and transition between different states.

## Introduction

Boskos is inited with a config of resources, one JSON entry per line, from `-config`

A resource object looks like
```go
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

1. 	URL: /acquire
	Desc: Use /acquire when you want to get hold of some resource.
	Method: POST
	URL Params: 
		Required: type=[string]  : type of requested resource
		Required: state=[string] : current state of the requested resource
		Required: dest=[string] : destination state of the requested resource
		Required: owner=[string] : requester of the resource
	Return: error code or 200 with a valid Resource JSON object
	Example: /acquire?type=gce-project&state=free&dest=busy&owner=user

2.	URL: /release
	Desc: use /release when you finish use some resource. Owner need to match current owner.
	Method: POST
	URL Params:
		Required: name=[string]  : name of finished resource
		Required: owner=[string] : owner of the resource
		Required: dest=[string]  : destination state of the released resource
	Return: status code
	Example: /release?name=k8s-jkns-foo&dest=dirty&owner=user

3.	URL: /update
	Desc: Update resource last-update timestamp. Owner need to match current owner.
	Method: POST
	URL Params:
		Required: name=[string]  : name of target resource
		Required: owner=[string] : owner of the resource
		Required: state=[string] : current state of the resource
	Return: status code
	Example: /update?name=k8s-jkns-foo&state=free&owner=user

4.	URL: /reset
	Desc: Reset a group of expired resource to certain state.
	Method: POST
	URL Params:
		Required: type=[string] : type of resource in interest
		Required: state=[string] : current state of the expired resource
		Required: dest=[string] : destination state of the expired resource
		Required: expire=[durationStr*] resource has not been updated since before {expire}. 
			*durationStr is any string can be parsed by [time.ParseDuration()](https://golang.org/pkg/time/#ParseDuration)
	Return: status code with a list of [Owner:Resource] pairs, which can be unmarshal into map[string]string
	Example: /reset?type=gce-project&state=busy&dest=dirty&expire=20m

5.	URL: /metric
	Method: GET
	URL Params: 
		Required: type=[string] : type of requested resource

	Return: error code or JSON object with: 
			Count of projects in each state
			Count of projects with each owner (or without an owner)
			Sum of state moved to after /done (Todo)
			A sample object will look like:
```json
			{
				"type" : "project",
				"Current": 
				{
					"total"   : 35,
					"free"    : 20,
					"dirty"   : 10,
					"injured" : 5
				},
				"Owners":
				{
					"fejta" : 1,
					"Senlu" : 1,
					"sig-testing" : 20,
					"Janitor" : 10,
					"None" : 20
				}
			}

```

## Config update:
1. Edit resources.json, and send a PR.

1. After PR is LG'd, make sure your branch is synced up with master.

1. run `make update-config` to update the configmap.

1. Boskos updates its config every 10min. Newly added resources will be available after next update cycle.
Newly deleted resource will be removed in a future update cycle if the resource is not owned by any user. 

## Local test:
1. Start boskos with a fake resources.json, with `go run boskos.go -config=/path/to/resources.json`

1. Sent some local requests to boskos:
```
curl 'http://127.0.0.1:8080/acquire?type=project&state=free&dest=busy&owner=user'
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
[ root@curl-XXXXX:/ ]$ curl 'http://boskos/acquire?type=project&state=free&dest=busy&owner=user'
````
