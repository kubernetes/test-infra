
# Kubernetes Continuous Delivery
Deployment scripts for continuous integration and/or continuous delivery of kubernetes projects. This project was tested and released using a private installs of CircleCI, Jenkins and SolanoCI. The core deployments scripts (`./deploy/`) are used for all three systems and as a result are fairly robust and compatible for other systems to use. Please contribute to add features and support for different CI/CD systems as needed.

The idea of these scripts was based off of the [docker-hello-google example on circleci repo](https://github.com/circleci/docker-hello-google). Thank you for giving us all a head start!

## Usage

In general, the documentation for scripts is handled inline with comments. You must have a [kubernetes config](http://kubernetes.io/v1.0/docs/user-guide/kubeconfig-file.html) file available and accessible to your build system from a URL. An S3 URL was used in testing. The files from this project should be added to your existing github project (minus the `Dockerfile`, `package.json` and `server.js` that are here just for testing). If you want to make sure your config file is cached an not downloaded with each run then `md5sum` the config file and update the `KUBECHECKSUM` variable in `circle.yml` or `jenkins.sh`. 

You CI build servers need to have docker installed and in the case of Jenkins and CircleCI the docker socket must be accessible from inside docker containers (`sudo chmod 777 /var/run/docker.sock`). This would be a security issue for a cloud provider but since we're working on our own private CI system here and we trust our own containers this is not a problem. This is critical for getting docker caching to work between builds until docker caching is available between docker daemon restarts. For CircleCI there are a few extra steps that should be a part of your bootstrap scipts.

```
echo 'DOCKER_OPTS="-g /data/docker"' | sudo tee -a /etc/default/docker
export CIRCLE_SHARED_DOCKER_ENGINE=true
```

SolanoCI has Docker caching built into their on-premise platform as a part of a beta release. Reach out to them for special instructions for applying that release.

You must have at least one running kubernetes cluster. If you intend to deploy to production install multiple kubernetes clusters and run the deploy command multiple times with the different context names from your kube config file.

Deployment scripts are in the `./deploy/` folder.

* `./deploy/ensure-kubectl.sh` - pulls down the kubectl binary if it doesn't exist and installs packages that are expected to be in place if missing.
* `./deploy/deploy-service.sh` - call kubectl commands to deploy services based on the yaml inside your project.

Your kubernetes spec yaml can be in any folder of your project as specified by the config file for Jenkins (`jenkins.sh`) or CircleCI (`circle.yaml`). All environment variables set by the build system can be leveraged in the kubernetes spec yaml and will be replaced before being passed onto the cluster. A kubectl delete, kubectl create pattern was chosen for fast qa/integration testing iterations. For production deployments a rolling update is used. This is controlled by passing a parameter to the deploy command and can be easily customized.


## Jenkins
If you already have a Jenkins instance running with a local docker daemon installed on the builder box, you should be able to get going by doing the following.

1. Create your jenkins job and link it to your github account using the git source code management plugin.
2. Create credentials for your docker registry in Jenkins.
3. Map the credentials to the `$dockeruser` and `$dockerpass` environment variables. NOTE: going this route so that the credentials are not stored in your github account.
4. Create an execute shell command as follows:
```
cd $WORKSPACE
chmod +x ./jenkins.sh && ./jenkins.sh
```
5. Update the environment variables in the `jenkins.sh`
6. push changes to github and check the Jenkins job console output for errors\success messages.

## Circle CI
1. Update the `circle.yml` environment variables to fit your environment.
2. Link your project to Circle CI
3. Manually set the docker `$dockeruser` and `$dockerpass` environment variables on your CircleCI project. NOTE: going this route so that the credentials are not stored in your github account.
4. Run a build.
5. Check the job output for any errors. The deploy script output prints the api proxy endpoint to hit your service for any manual testing and a link to kibana.

## Solano CI
1. Update the `solano.yml` environment variables to fit your environment.
2. Link your project to SolanoCI
3. Manually set the docker `$dockeruser` and `$dockerpass` environment variables on your solano project using the `solano config:add` command. NOTE: going this route so that the credentials are not stored in your github account.
4. Run a build.
5. Check the job output for any errors. The deploy script output prints the api proxy endpoint to hit your service for any manual testing and a link to kibana.

##### Author
Dan Wilson: emaildanwilson@gmail.com
