#!/usr/bin/env python

# Copyright 2016 The Kubernetes Authors All rights reserved.
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

import webapp2

import view_base
import view_build
import view_logs
import view_pr

app = webapp2.WSGIApplication([
    (r'/', view_base.IndexHandler),
    (r'/jobs/(.*)$', view_build.JobListHandler),
    (r'/builds/(.*)/([^/]+)/?', view_build.BuildListHandler),
    (r'/build/(.*)/([^/]+)/(\d+)/?', view_build.BuildHandler),
    (r'/build/(.*)/([^/]+)/(\d+)/nodelog*', view_logs.NodeLogHandler),
    (r'/pr/(\d+)', view_pr.PRHandler),
    (r'/pr/?', view_pr.PRDashboard),
    (r'/pr/([-\w]+)', view_pr.PRDashboard),
    (r'/pr/(.*/build-log.txt)', view_pr.PRBuildLogHandler),
], debug=True)
