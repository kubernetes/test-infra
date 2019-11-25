/*
Copyright 2019 The Kubernetes Authors.

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

export function parseQuery(query: string): {[key: string]: string | undefined} {
  const ret: {[key: string]: string} = {};
  for (const [k, v] of query.split('&').map((x) => x.split('=').map(unescape))) {
    ret[k] = v;
  }
  return ret;
}

export function getParameterByName(name: string): string | undefined {
  return parseQuery(location.search.substr(1))[name];
}

export function relativeURL(params: { [key: string]: string } = {}): string {
  const url = new URL(location.href);
  for (const key of Object.keys(params)) {
    url.searchParams.append(key, params[key]);
  }
  return encodeURIComponent(url.pathname + url.search + url.hash);
}
