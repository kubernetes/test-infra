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

package main

import (
	"fmt"
	"sync"

	kube_api "k8s.io/kubernetes/pkg/api"
)

func podId(pod *kube_api.Pod) string {
	return fmt.Sprintf("%s_%s", pod.Namespace, pod.Name)
}

// Thread safe implementation of set of Pods.
type podSet struct {
	set   map[string]struct{}
	mutex sync.Mutex
}

// NewPodSet creates new instance of podSet.
func NewPodSet() *podSet {
	return &podSet{
		set:   make(map[string]struct{}),
		mutex: sync.Mutex{},
	}
}

// Add the pod to the set.
func (s *podSet) Add(pod *kube_api.Pod) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.set[podId(pod)] = struct{}{}
}

// Remove the pod from set.
func (s *podSet) Remove(pod *kube_api.Pod) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.set, podId(pod))
}

// Has checks whether the pod is in the set.
func (s *podSet) Has(pod *kube_api.Pod) bool {
	return s.HasId(podId(pod))
}

// HasId checks whether the pod is in the set.
func (s *podSet) HasId(pod string) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	_, found := s.set[pod]
	return found
}
