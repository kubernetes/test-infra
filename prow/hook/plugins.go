/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package hook

// All enabled plugins. We need to empty import them like this so that they
// will be linked into any hook binary.
import (
	_ "k8s.io/test-infra/prow/plugins/approve"
	_ "k8s.io/test-infra/prow/plugins/assign"
	_ "k8s.io/test-infra/prow/plugins/blockade"
	_ "k8s.io/test-infra/prow/plugins/blunderbuss"
	_ "k8s.io/test-infra/prow/plugins/buildifier"
	_ "k8s.io/test-infra/prow/plugins/cat"
	_ "k8s.io/test-infra/prow/plugins/cla"
	_ "k8s.io/test-infra/prow/plugins/docs-no-retest"
	_ "k8s.io/test-infra/prow/plugins/dog"
	_ "k8s.io/test-infra/prow/plugins/golint"
	_ "k8s.io/test-infra/prow/plugins/heart"
	_ "k8s.io/test-infra/prow/plugins/help"
	_ "k8s.io/test-infra/prow/plugins/hold"
	_ "k8s.io/test-infra/prow/plugins/label"
	_ "k8s.io/test-infra/prow/plugins/lgtm"
	_ "k8s.io/test-infra/prow/plugins/lifecycle"
	_ "k8s.io/test-infra/prow/plugins/milestone"
	_ "k8s.io/test-infra/prow/plugins/milestonestatus"
	_ "k8s.io/test-infra/prow/plugins/owners-label"
	_ "k8s.io/test-infra/prow/plugins/releasenote"
	_ "k8s.io/test-infra/prow/plugins/requiresig"
	_ "k8s.io/test-infra/prow/plugins/shrug"
	_ "k8s.io/test-infra/prow/plugins/sigmention"
	_ "k8s.io/test-infra/prow/plugins/size"
	_ "k8s.io/test-infra/prow/plugins/skip"
	_ "k8s.io/test-infra/prow/plugins/slackevents"
	_ "k8s.io/test-infra/prow/plugins/trigger"
	_ "k8s.io/test-infra/prow/plugins/updateconfig"
	_ "k8s.io/test-infra/prow/plugins/wip"
	_ "k8s.io/test-infra/prow/plugins/yuks"
)
