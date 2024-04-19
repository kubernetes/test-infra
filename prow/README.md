![Prow Logo](./docs/logos/logo-horizontal.svg)

# Prow source code has moved

The Prow source code that previously lived here was moved along with ghProxy to the [kubernetes-sigs/prow](https://github.com/kubernetes-sigs/prow) repository on April 9, 2024.

## How this impacts you and what to do:

- If you just use Prow or maintain a Prow instance there is nothing you need to do. Container images will still be available at [gcr.io/k8s-prow/](http://gcr.io/k8s-prow/).
- If you don't develop Prow itself, but you do rely on its go packages (via importing the k8s.io/test-infra go module), after April 9th you'll need to update import statements to be prefixed with 'sigs.k8s.io/prow' rather than 'k8s.io/test-infra'. Since the repo relative paths will remain the same, you can use a sed command like this one to fix any references: 
  `sed -i 's,k8s.io/test-infra/prow,sigs.k8s.io/prow/prow,g;s,k8s.io/test-infra/ghproxy,sigs.k8s.io/prow/ghproxy,g'`
  Don't forget to run `go mod tidy` afterwards.
- If you do develop Prow you'll need to start targeting PRs against the [kubernetes-sigs/prow](https://github.com/kubernetes-sigs/prow) repo instead of this repo. You can transfer any open PRs to the new repo by using the `git format-patch` and `git am` commands. You'll also need to update package references as described in the previous bullet point.
