#!/bin/bash
# Copyright 2016 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

# Based on https://github.com/travis-ci/travis-ci/issues/738#issuecomment-11179888
readonly GAE_ZIP=google_appengine_1.9.40.zip
readonly GAE_ROOT=${HOME}/google_appengine
wget -nv https://storage.googleapis.com/appengine-sdks/featured/${GAE_ZIP}
unzip -q ${GAE_ZIP} -d ${HOME}
pip install -r gubernator/test_requirements.txt
pip install -r jenkins/test-history/requirements.txt

./verify/verify-boilerplate.py
python -m unittest discover -s jenkins/test-history -p "*_test.py"
pylint jenkins/bootstrap.py
pylint queue-health/graph/graph.py
./jenkins/bootstrap_test.py
nosetests --with-doctest jenkins/bootstrap.py
