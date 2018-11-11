# mason


## Introduction

Mason creates and update virtual resources from existing physical resources. As an example, in order to create
a GKE cluster you need a GCP Project. Mason will construct virtual resources based on the configuration of a
given resource type. Each configuration defines its physical resources requirement. Mason will acquire those
resources from Boskos and pass them to the specific implementation that will turn them into a virtual resource.
Mason only handles lease and release of physical resources.

## Configuration

Each configuration is represented by a name. The name of configuration maps to the virtual resource type in Boskos.

Here is an example configuration:

```yaml

configs:
- name: type2
  needs:
    type1: 1
  config:
    type: GCPResourceConfig
    content: |
      projectconfigs:
      - type: type1
        clusters:
        - machinetype: n1-standard-2
          numnodes: 4
          version: 1.7
          zone: us-central-1f
        vms:
        - machinetype: n1-standard-4
          sourceimage: projects/debian-cloud/global/images/debian-9-stretch-v20180105
          zone: us-central-1f
          tags:
          - http-server
          - https-server
          scopes:
          - https://www.googleapis.com/auth/cloud-platform

```

As you can see in this example. In order to create a virtual resource of type2 we need one resource
of type1. Once we have acquired the right resources, we need Masonable interface of type GCPResourceConfig
that will know how to parse this configuration.

## Operation

Mason is only operating on a given set of resource types.
In order to use Mason, one needs to register a Masonable interface using the RegisterConfigConverter function.

A simple binary would like that:

```golang

func main() {
  flag.Parse()
  logrus.SetFormatter(&logrus.JSONFormatter{})

  if *configPath == "" {
    logrus.Panic("--config must be set")
  }

  mason := mason.NewMasonFromFlags()

  // Registering Masonable Converters
  if err := mason.RegisterConfigConverter(gcp.ResourceConfigType, gcp.ConfigConverter); err != nil {
    logrus.WithError(err).Panicf("unable tp register config converter")
  }
  if err := mason.UpdateConfigs(*configPath); err != nil {
    logrus.WithError(err).Panicf("failed to update mason config")
  }
  go func() {
    for range time.NewTicker(defaultUpdatePeriod).C {
      if err := mason.UpdateConfigs(*configPath); err != nil {
        logrus.WithError(err).Warning("failed to update mason config")
      }
    }
  }()

  mason.Start()
  defer mason.Stop()
  stop := make(chan os.Signal, 1)
  signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
  <-stop
```

### Recycling Thread

The recycling thread is acquiring dirty virtual resources and releasing the associated physical resources as
dirty such Janitor can clean them up as an example. It then put the virtual resource to the Fulfilling queue.

### Fulfilling Thread

The fulfilling thread will look at the config resource needs, and will acquire the necessary resources.
Once a resource is acquired it will update the virtual resource LEASED_RESOURCES user data. Once all resources
are acquired, it will be put on the cleaning queue.

### Cleaning Thread

The cleaning thread will construct the virtual resources from the leased physical resources and update the user
data with the user data provided by the Masonable implementation

### Freeing Thread

The freeing thread will release all resources. It will release the leased physical resources with a state that is
equal to the name of virtual resource and release the virtual resource as free.

### Mason Client
Mason comes with its own client to ease usage. The mason client takes care of
acquiring and release all the right resources from the User Data information.

## References

[Istio Deployment](https://github.com/istio/test-infra/tree/master/boskos)

[Design Doc](https://goo.gl/vHNfww)

