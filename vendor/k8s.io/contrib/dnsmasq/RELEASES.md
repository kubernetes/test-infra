# Cutting a release

Until we have a proper setup for building this automatically with every binary
release, here are the steps for making a release. We make releases when they
are ready, not on every PR.

1. Build the container for testing: 

    ```console
    cd dnsmasq
    make container REGISTRY=<your-name> TAG=rc ARCH=<your architecture>
    ```

2. Manually deploy this to your own cluster by updating the replication
   controller and deleting the running pod(s).

3. Verify it works.

4. Update the TAG version in `Makefile` and update the `Changelog`.

5. Build and push the container for real for all architectures:

	```console
	# Build for linux/amd64 (default)
	$ make push ARCH=amd64
	# ---> gcr.io/google_containers/kube-dnsmasq-amd64:TAG

	$ make push ARCH=arm
	# ---> gcr.io/google_containers/kube-dnsmasq-arm:TAG

	$ make push ARCH=arm64
	# ---> gcr.io/google_containers/kube-dnsmasq-arm64:TAG

	$ make push ARCH=ppc64le
	# ---> gcr.io/google_containers/kube-dnsmasq-ppc64le:TAG
	```
    
6. Submit a PR for the kubernetes/contrib repository and let it run conformance tests.

7. Submit a PR for the kubernetes/kubernetes repository to switch to the new TAG version with a do-not-merge label.

8. Manually deploy this to your own cluster by updating the replication
   controller and deleting the running pod(s).

9. Verify it works.

10. Merge the kubernetes/contrib repository PR.

11. Allow the kubernetes/kubernetes PR to be merged by removing the do-not-merge label.


[![Analytics](https://kubernetes-site.appspot.com/UA-36037335-10/GitHub/contrib/dnsmasq/RELEASES.md?pixel)]()
