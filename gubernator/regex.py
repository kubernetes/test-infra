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

import re

''' Matches against: kubelet.log
Purpose: Match UID from the object reference 
Example:
	line: Event(api.ObjectReference{Kind:"Pod",
	  Namespace:"e2e-tests-configmap-oi12h",
	  Name:"pod-configmaps-b5b876cb-3e1e-11e6-8956-42010af0001d",
	  UID:"b5b8a59e-3e1e-11e6-b358-42010af0001d", APIVersion:"v1",
	  ResourceVersion:"331", FieldPath:""}): type: 'Warning' reason:
	  'MissingClusterDNS' kubelet does not have ClusterDNS IP configured and
	  cannot create Pod using "ClusterFirst" policy. Falling back to DNSDefault
	  policy.
	matches: b5b8a59e-3e1e-11e6-b358-42010af0001d
'''
uidobj_re = re.compile(r'Event\(api\.ObjectReference\{[^}].*UID:&#34;(.*?)&#34;(, [^}]*)?\}')

'''
Purpose: Match a specific word
Example: word(abcdef)
	line: 'Pod abcdef failed'
	matches: abcdef
	line: 'Podname(abcdef)'
	matches: abcdef
	line: '/abcdef/'
	matches: abcdef
'''
def wordRE(word):
	return re.compile(r'\b(%s)\b' % word, re.IGNORECASE)
