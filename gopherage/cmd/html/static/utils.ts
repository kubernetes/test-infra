/*
Copyright 2018 The Kubernetes Authors.

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

// JavaScript for some reason can map over arrays but nothing else.
// Provide our own tools.

export function*
    map<T, U>(iterable: Iterable<T>, fn: (value: T) => U): Iterable<U> {
  for (const entry of iterable) {
    yield fn(entry);
  }
}

export function reduce<T, U>(
    iterable: Iterable<T>, fn: (acc: U, value: T) => U, initialValue: U): U {
  let accumulator = initialValue;
  for (const entry of iterable) {
    accumulator = fn(accumulator, entry);
  }
  return accumulator;
}

export function*
    filter<T>(iterable: Iterable<T>, fn: (value: T) => boolean): Iterable<T> {
  for (const entry of iterable) {
    if (fn(entry)) {
      yield entry;
    }
  }
}

export function* enumerate<T>(iterable: Iterable<T>): Iterable<[number, T]> {
  let i = 0;
  for (const entry of iterable) {
    yield [i++, entry];
  }
}
