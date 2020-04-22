# boskos


## Background
[βοσκός](https://en.wiktionary.org/wiki/%CE%B2%CE%BF%CF%83%CE%BA%CF%8C%CF%82) - shepherd in greek!

boskos is a resource manager service, that handles and manages different kind of resources and transition between
different states.

## Introduction

Boskos is inited with a config of resources, a list of resources by names. It's passed in by `-config`, usually as a
config map.

Boskos supports 2 types of resources, static and dynamic resources. Static resources are the one that depends on actual
physical resources, meaning someone needs to physically create it, and add it to the list. Dynamic resources
may depend on static resources. In the example bellow , aws-account is static resource, and aws-cluster is a dynamic
resource that depends on having an aws-account. Once a cluster is created, AWS are in used, so admin might want to
always have a minimum cluster available for testing, and might allow for more cluster to be created for spike usage.

```yaml
---
resources:
  # Static
  - type: "aws-account"
    state: free
    names:
    - "account1"
    - "account2"
  # Dynamic
  - type: "aws-cluster"
    state: dirty
    min-count: 1
    max-count: 2
    lifespan: 48h
    needs:
      aws-account: 1
    config:
      type: AWSClusterCreator
      content: "..."

```

Type can be GCPProject, cluster, or even a dota2 server, anything that you
want to be a group of resources. Name is a unique identifier of the resource.
State is a string that tells the current status of the resource.

User Data is here for customization. In Mason as an example, we create new
resources from existing ones (creating a cluster inside a GCP project), but
in order to acquire the right resources, we need to store some information in the
final resource UserData. It is up to the implementation to parse the string into the
right struct. UserData can be updated using the update API call. All resource
user data is returned as part of acquisition (calling acquire or acquirebystate)


## Dynamic Resources

As explain in the introduction, dynamic resources were introduced to reduce cost.

If all resources are currently being used, and the count of resources is bellow Max,
boskos will create new resources on Acquire. In order to take advantage of this,
users need to specify a request ID in Acquire and keep using the same requestID
until the resource is available.

Boskos will take care of naming and creating resources (if the current
count is below min-count) and deleting the resources if they are expired (lifetime
option) or over max-count.

All resource being deleted (due to config update or expiration) will be marked
as `ToBeDeleted`. The cleaner component will mark them as `Tombstone` such that
they can be safely deleted by Boskos. The cleaner will ensure that dynamic
resources release other leased resources associated with it to prevent leaks.

## API

###   `POST /acquire`

Use `/acquire` when you want to get hold of some resource.

#### Required Parameters

| Name    | Type     | Description                                 |
| ------- | -------- | ------------------------------------------- |
| `type`  | `string` | type of requested resource                  |
| `state` | `string` | current state of the requested resource     |
| `dest`  | `string` | destination state of the requested resource |
| `owner` | `string` | requester of the resource                   |

#### Optional Parameters

| Name         | Type     | Description                                   |
| ------------ | -------- | --------------------------------------------- |
| `request_id` | `string` | request id to use to keep your priority rank  |


Example: `/acquire?type=gce-project&state=free&dest=busy&owner=user`.

On a successful request, `/acquire` will return HTTP 200 and a valid Resource JSON object.

###   `POST /acquirebystate`

Use `/acquirebystate` when you want to get hold of a set of resources in a given
state.

#### Required Parameters

| Name    | Type     | Description                                 |
| ------- | -------- | ------------------------------------------- |
| `state` | `string` | current state of the requested resource     |
| `dest`  | `string` | destination state of the requested resource |
| `owner` | `string` | requester of the resource                   |
| `names` | `string` | comma separated list of resource names      |

Example: `/acquirebystate?state=free&dest=busy&owner=user&names=res1,res2`.

On a successful request, `/acquirebystate` will return HTTP 200 and a valid list of Resources JSON object.

###   `POST /release`

Use `/release` when you finish use some resource. Owner need to match current owner.

#### Required Parameters

| Name    | Type     | Description                                |
| ------- | -------- | ------------------------------------------ |
| `name`  | `string` | name of finished resource                  |
| `owner` | `string` | owner of the resource                      |
| `dest`  | `string` | destination state of the released resource |

Example: `/release?name=k8s-jkns-foo&dest=dirty&owner=user`

###   `POST /update`

Use `/update` to update resource last-update timestamp. Owner need to match current owner.

#### Required Parameters

| Name    | Type     | Description                    |
| ------- | -------- | ------------------------------ |
| `name`  | `string` | name of target resource        |
| `owner` | `string` | owner of the resource          |
| `state` | `string` | current state of the resource  |


#### Optional Parameters
In order to update user data, just marshall the user data into the request body.

Example: `/update?name=k8s-jkns-foo&state=free&owner=user`

###   `POST /reset`

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

###   `GET /metric`

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
1. Edit resources.yaml, and send a PR.

1. After PR is LG'd, make sure your branch is synced up with master.

1. run `make update-config` to update the configmap.

1. Boskos updates its config every 10min. Newly added resources will be available after next update cycle.
Newly deleted resource will be removed in a future update cycle if the resource is not owned by any user.

## Other Components:

[`Reaper`] looks for resources that owned by someone, but have not been updated for a period of time,
and reset the stale resources to dirty state for the [`Janitor`] component to pick up. It will prevent
state leaks if a client process is killed unexpectedly.

[`Janitor`] looks for dirty resources from boskos, and will kick off sub-janitor process to clean up the
resource, finally return them back to boskos in a free state.

[`Metrics`] is a separate service, which can display json metric results, and has HTTP endpoint
opened for prometheus monitoring.

[`Mason`] updates virtual resources with existing resources. An example would be
a cluster. In order to create a GKE cluster you need a GCP Project. Mason will look for specific
resources and release leased resources as dirty (such that Janitor can pick it up) and ask for
brand new resources in order to convert them in the final resource states. Mason
comes with its own client to ease usage. The mason client takes care of
acquiring and release all the right resources from the User Data information.

[`cleaner`] Mark resource with status `ToBeDeleted` as `Tombstone` such they can be
safely deleted by Boskos. This is important for dynamic resources such that all
associated resources can be released before deletion to prevent leak.

[`Storage`] There could be multiple implementation on how resources and mason
config are stored. Since we have multiple components with storage needs, we have
now shared storage implementation. In memory and in Cluster via k8s custom
resource definition.

[`crds`] General client library to store data on k8s custom resource definition.
In theory those could be use outside of Boskos.

For the boskos server that handles k8s e2e jobs, the status is available from the [`Velodrome dashboard`]

## Adding UserData to a resource

1. Check it out: 
    ```shell
    curl -X POST "http://localhost:8080/acquire?type=my-resource&state=free&dest=busy&owner=$(whoami)"
    {"type":"my-resource","name":"resource1","state":"busy","owner":"user","lastupdate":"2019-02-07T22:33:38.01350902Z","userdata":null}
    ```

1. Add the data:
    ```shell
    curl -X POST -d '{"access-key-id":"17","secret-access-key":"18"}' "http://localhost:8080/update?name=resource1&state=busy&owner=$(whoami)"
    ```

1. Check it back in:
    ```shell
    curl -X POST 'http://localhost:8080/release?name=liz2&dest=free&owner=user'
    ```

## Local test:
1. Start boskos with a fake config.yaml, with `go run boskos.go -in_memory -config=/path/to/config.yaml`

1. Sent some local requests to boskos:
```
curl 'http://127.0.0.1:8080/acquire?type=project&state=free&dest=busy&owner=user'
```

## K8s test:
1. Create and navigate to your own cluster

1. `make server-deployment`

1. `make service`

1. `kubectl create configmap -n test-pods resources --from-file=config=cfg.yaml`
  See [`boskos-resources.yaml`](../config/prow/cluster/boskos-resources.yaml) for an example of how the config file should look

1. `kubectl describe svc -n test-pods boskos` to make sure boskos is running

1. Test from another pod within the cluster
```
kubectl run curl --image=radial/busyboxplus:curl -i --tty
Waiting for pod default/curl-XXXXX to be running, status is Pending, pod ready: false
If you don't see a command prompt, try pressing enter.
[ root@curl-XXXXX:/ ]$ curl -X POST 'http://boskos.test-pods.svc.cluster.local/acquire?type=project&state=free&dest=busy&owner=user'
````

[`Reaper`]: ./reaper
[`Janitor`]: ./janitor
[`Metrics`]: ./metrics
[`Mason`]: ./mason
[`Storage`]: ./storage
[`crds`]: ./crds
[`Velodrome dashboard`]: http://velodrome.k8s.io/dashboard/db/boskos-dashboard
