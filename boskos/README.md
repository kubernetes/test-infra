# Boskos relocated

Boskos has been moved into its own repository under kubernetes-sigs:

https://github.com/kubernetes-sigs/boskos

# What's still here

The only thing matters here is `./cmd/janitor/gcp_janitor.py` script, which is
still used by
[/scenarios/kubernetes_janitor.py#L66](https://github.com/kubernetes/test-infra/blob/d641897cea52d493ef883a4dfa6728ffdfa02dfa/scenarios/kubernetes_janitor.py#L66),
and `/scenarios` had been announced deprecated since 2018, which will be done
with https://github.com/kubernetes/test-infra/issues/20760.
