Simple utility to detect when AWS account has no c4.large instances available, due to InsufficientInstanceCapacity.

This queries EC2 for all regions, and tries to launch a c4.large in each.  (Note that this therefore costs money to run!).

After looping through all regions, it shuts down all instances tagged with its tag.

We currently update kubernetes_e2e.py with the results, e.g. [#8170](https://github.com/kubernetes/test-infra/pull/8170)
