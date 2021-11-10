// Generate by running:
//      cue export --out yaml --outfile cloudbuild.yaml
package cloudbuild

import "list"

timeout: '3600s'
options: {
	// Cloud Build only offers machines with low memory per
	// https://cloud.google.com/build/pricing, use N1_HIGHCPU_32
	// to avoid OOM.
	machineType: 'N1_HIGHCPU_32'
	env: [
		'GOPATH=/go',
		'GO111MODULE=on',
		'GOPROXY=https://proxy.golang.org,direct',
		'CGO_ENABLED=0',
		'GOOS=linux',
	]
	volumes: [
		{
			name: 'go-modules'
			path: '/go/pkg'
		},
	]
}

#ImageSpecs: {
	name:     string
	path:     string | *""
	platform: string | *"amd64"
}

let _IMAGES = {
	// List of prow images from https://github.com/kubernetes/test-infra/blob/2839f81b32e34e5398f7f22a4f6960c4503218f4/prow/BUILD.bazel#L15,
	// minus:
	// - deck: due to typescript
	// - grandmatriach: due to bash

	amd64: [
		for podutilsImage in [
			"admission",
			"branchprotector",
			"checkconfig",
			"config-bootstrapper",
			"exporter",
			"gerrit",
			"crier",
			"generic-autobumper",
			"gcsupload",
			"hook",
			"hmac",
			"horologium",
			"invitations-accepter",
			"jenkins-operator",
			"mkpj",
			"mkpod",
			"peribolos",
			"sinker",
			"status-reconciler",
			"sub",
			"tide",
			"tot",
			"pipeline",
			"prow-controller-manager",
			// pod utilities images separated below
			"clonerefs",
			"entrypoint",
			"initupload",
			"sidecar",
		] {
			#ImageSpecs & {name: podutilsImage}
		},

		#ImageSpecs & {name: "needs-rebase", path:   "prow/external-plugins/needs-rebase"},
		#ImageSpecs & {name: "cherrypicker", path:   "prow/external-plugins/cherrypicker"},
		#ImageSpecs & {name: "refresh", path:        "prow/external-plugins/refresh"},
		#ImageSpecs & {name: "ghproxy", path:        "ghproxy"},
		#ImageSpecs & {name: "label_sync", path:     "label_sync"},
		#ImageSpecs & {name: "commenter", path:      "robots/commenter"},
		#ImageSpecs & {name: "pr-creator", path:     "robots/pr-creator"},
		#ImageSpecs & {name: "issue-creator", path:  "robots/issue-creator"},
		#ImageSpecs & {name: "configurator", path:   "testgrid/cmd/configurator"},
		#ImageSpecs & {name: "transfigure", path:    "testgrid/cmd/transfigure"},
		#ImageSpecs & {name: "gcsweb", path:         "gcsweb/cmd/gcsweb"},
		#ImageSpecs & {name: "bumpmonitoring", path: "experiment/bumpmonitoring"},
	]
	arm64: [
		for podutilsImage in [
			"clonerefs",
			"entrypoint",
			"initupload",
			"sidecar",
		] {
			#ImageSpecs & {name: podutilsImage}
		},
	]
	s390x: [
		for podutilsImage in [
			"clonerefs",
			"entrypoint",
			"initupload",
			"sidecar",
		] {
			#ImageSpecs & {name: podutilsImage}
		},
	]
	ppc64le: [
		for podutilsImage in [
			"clonerefs",
			"entrypoint",
			"initupload",
			"sidecar",
		] {
			#ImageSpecs & {name: podutilsImage}
		},
	]
}

substitutions: {
	'_REPO': 'prow-exp'
	'_TAG':  ''
}

steps: [
	// Downloading packages from go.mod
	{
		id:   'download-go-mod'
		name: 'golang:1.16.9'
		args: [
			'go',
			'mod',
			'download',
			'-x',
		]
		waitFor: [
			'-',
		]
	},

	// Docker build

	// Cuelang by design doesn't allow nested for loop (https://github.com/cue-lang/cue/issues/1375),
	// So defining for arm64, s390x and ppc64le individually.
	for img in _IMAGES["amd64"] {
		id:   'dockerize-\(img.name)-amd64'
		name: "docker:20"
		args: [
			"build",
			"--file=images/prow-base/Dockerfile_amd64",
			"--tag=gcr.io/${_REPO}/\(img.name):latest",
			"--tag=gcr.io/${_REPO}/\(img.name):${_TAG}",
			"--tag=gcr.io/${_REPO}/\(img.name):amd64",
			// e.g.: latest-arm64
			"--tag=gcr.io/${_REPO}/\(img.name):latest-amd64",
			// e.g.: 20211111-abcdeft-arm64
			"--tag=gcr.io/${_REPO}/\(img.name):${_TAG}-amd64",
			"--build-arg=IMAGE_NAME=\(img.name)",
			if img.path != "" {
				"--build-arg=SOURCE_PATH=\(img.path)"
			},
			"--build-arg=TARGETARCH=amd64",
			".",
		]
		waitFor: [
			'download-go-mod',
		]
	},

	for img in _IMAGES["arm64"] {
		id:   'dockerize-\(img.name)-arm64'
		name: "docker:20"
		args: [
			"build",
			"--file=images/prow-base/Dockerfile_arm64",
			"--tag=gcr.io/${_REPO}/\(img.name):arm64",
			// e.g.: latest-arm64
			"--tag=gcr.io/${_REPO}/\(img.name):latest-arm64",
			// e.g.: 20211111-abcdeft-arm64
			"--tag=gcr.io/${_REPO}/\(img.name):${_TAG}-arm64",
			"--build-arg=IMAGE_NAME=\(img.name)",
			if img.path != "" {
				"--build-arg=SOURCE_PATH=\(img.path)"
			},
			"--build-arg=TARGETARCH=arm64",
			".",
		]
		waitFor: [
			'-',
		]
	},

	for img in _IMAGES["s390x"] {
		id:   'dockerize-\(img.name)-s390x'
		name: "docker:20"
		args: [
			"build",
			"--file=images/prow-base/Dockerfile_s390x",
			"--tag=gcr.io/${_REPO}/\(img.name):s390x",
			// e.g.: latest-arm64
			"--tag=gcr.io/${_REPO}/\(img.name):latest-s390x",
			// e.g.: 20211111-abcdeft-arm64
			"--tag=gcr.io/${_REPO}/\(img.name):${_TAG}-s390x",
			"--build-arg=IMAGE_NAME=\(img.name)",
			if img.path != "" {
				"--build-arg=SOURCE_PATH=\(img.path)"
			},
			"--build-arg=TARGETARCH=s390x",
			".",
		]
		waitFor: [
			'-',
		]
	},

	for img in _IMAGES["ppc64le"] {
		id:   'dockerize-\(img.name)-ppc64le'
		name: "docker:20"
		args: [
			"build",
			"--file=images/prow-base/Dockerfile_ppc64le",
			"--tag=gcr.io/${_REPO}/\(img.name):ppc64le",
			// e.g.: latest-arm64
			"--tag=gcr.io/${_REPO}/\(img.name):latest-ppc64le",
			// e.g.: 20211111-abcdeft-arm64
			"--tag=gcr.io/${_REPO}/\(img.name):${_TAG}-ppc64le",
			"--build-arg=IMAGE_NAME=\(img.name)",
			if img.path != "" {
				"--build-arg=SOURCE_PATH=\(img.path)"
			},
			"--build-arg=TARGETARCH=ppc64le",
			".",
		]
		waitFor: [
			'-',
		]
	},

	// Docker push.
	//
	// A prow image can have up to 11 tags, like:
	// - 'latest', '${_TAG}', 'arm64', 's390x', 'ppc64le', plus permutations.
	// Docker push can only take a single tag, so if we want to explicitly push
	// each tag then there will be 11 steps for an image, pushing all prow images
	// will exceed the GCB limit of 100 steps per build (https://cloud.google.com/build/quotas#resource_limits).
	// So '--all-tags' seems to be a better solution here. Since this is GCB, pushing
	// all tags should only push tags from this build.
	for img in _IMAGES["amd64"] {
		id:   'push-\(img.name)-latest'
		name: 'docker:20'
		args: [
			"image",
			"push",
			"gcr.io/${_REPO}/\(img.name)",
			"--all-tags",
		]
		waitFor: [
			'dockerize-\(img.name)-amd64',
			// Ensure wait until images are built for all platforms
			if list.Contains(_IMAGES["arm64"], img) {
				'dockerize-\(img.name)-arm64'
			},
			if list.Contains(_IMAGES["s390x"], img) {
				'dockerize-\(img.name)-s390x'
			},
			if list.Contains(_IMAGES["ppc64le"], img) {
				'dockerize-\(img.name)-ppc64le'
			},
		]
	},
]
