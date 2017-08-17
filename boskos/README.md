# boskos


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

###	`POST /acquire`

Use `/acquire` when you want to get hold of some resource.

#### Required Parameters

| Name    | Type     | Description                                 |
| ------- | -------- | ------------------------------------------- |
| `type`  | `string` | type of requested resource                  |
| `state` | `string` | current state of the requested resource     |
| `dest`  | `string` | destination state of the requested resource |
| `owner` | `string` | requester of the resource                   |

Example: `/acquire?type=gce-project&state=free&dest=busy&owner=user`.

On a successful request, `/acquire` will return HTTP 200 and a valid Resource JSON object.

###	`POST /release`

Use `/release` when you finish use some resource. Owner need to match current owner.

#### Required Parameters

| Name    | Type     | Description                                |
| ------- | -------- | ------------------------------------------ |
| `name`  | `string` | name of finished resource                  |
| `owner` | `string` | owner of the resource                      |
| `dest`  | `string` | destination state of the released resource |

Example: `/release?name=k8s-jkns-foo&dest=dirty&owner=user`

###	`POST /update`

Use `/update` to update resource last-update timestamp. Owner need to match current owner.

#### Required Parameters

| Name    | Type     | Description                    |
| ------- | -------- | ------------------------------ |
| `name`  | `string` | name of target resource        |
| `owner` | `string` | owner of the resource          |
| `state` | `string` | current state of the resource  |

Example: `/update?name=k8s-jkns-foo&state=free&owner=user`

###	`POST /reset`

Use `/reset` to reset a group of expired resource to certain state.

#### Required Parameters

| Name     | Type          | Description                                         |
| -------- | ------------- | --------------------------------------------------- |
| `type`   | `string`      | type of resource in interest                        |
| `state`  | `string`      | current state of the expired resource               |
| `dest`   | `string`      | destination state of the expired resource           |
| `expire` | `durationStr` | resource has not been updated since before `expire` |

Note: `durationStr` is any string can be parsed by [`time.ParseDuration()`](https://golang.org/pkg/time/#ParseDuration)

On a successful request, `/reset` will return HTTP 200 and a list of [Owner:Resource] pairs, which can be unmarshalled into `map[string]string{}`

Example: `/reset?type=gce-project&state=busy&dest=dirty&expire=20m`

###	`GET /metric`

Use `/metric` to retrieve a metric.

#### Required Parameters

| Name   | Type     | Description                |
| ------ | -------- | -------------------------- |
| `type` | `string` | type of requested resource |

On a successful request, `/metric` will return HTTP 200 and a JSON object containing the count of projects in each state, the count of projects with each owner (or without an owner), and the sum of state moved to after `/done` (Todo). A sample object will look like:

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

## Other Components:

[`Reaper`] will be look for inactive resource in free state but occupied (have not been updated for a long time) 
actively, and reset the stale resources to dirty state for the [`Janitor`] component to pick up.

[`Janitor`] looks for dirty resources from boskos, and will kick off sub-janitor process to clean up the 
resource, finally return them back to boskos in a clean state.

[`Metrics`] is a separate service, which can display json metric results, and has HTTP endpoint 
opened for prometheus monitoring.

For the boskos server handles k8s e2e jobs, the status is available from the [`Velodrome dashboard`]


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

[`Reaper`]: ./reaper
[`Janitor`]: ./janitor
[`Metrics`]: ./metrics
[`Velodrome dashboard`]: http://velodrome.k8s.io/dashboard/db/boskos-dashboard