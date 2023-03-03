/*
Copyright 2020 The Kubernetes Authors.

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

package version

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

var (
	prowVersion = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "prow_version",
		Help: "Prow version.",
	})
)

func init() {
	prometheus.MustRegister(prowVersion)
	// For components that import current package, if `gatherProwVersion` if not
	// explicitly called, there will be no metrics for `prow_version`, and when
	// querying prometheus for `prow_version` the value will be zero, which
	// is inaccurate. Since the version would not change for the running binary,
	//  doing this once when binary starts should be fine.
	gatherProwVersion()
}

// gatherProwVersion reports prow version
func gatherProwVersion() {
	// record prow version
	version, err := VersionTimestamp()
	if err != nil {
		// Not worth panicking
		logrus.WithError(err).Debug("Failed to get version timestamp")
		prowVersion.Set(-1)
	} else {
		prowVersion.Set(float64(version))
	}
}
