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

package v1_test

import (
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/test-infra/prow/apis/prowjobs/v1"
)

func TestValidate(t *testing.T) {
	for _, tc := range []struct {
		caches      v1.Caches
		shouldError bool
	}{
		{ // Success
			v1.Caches{{
				Path: "/path",
				Keys: []string{"key"},
			}},
			false,
		},
		{ // Name not unique
			v1.Caches{
				{
					Path: "/path",
					Keys: []string{"key"},
				},
				{
					Path: "/path",
					Keys: []string{"key"},
				},
			},
			true,
		},
		{ // Path not absolute
			v1.Caches{{
				Path: "path",
				Keys: []string{"key"},
			}},
			true,
		},
		{ // No Path
			v1.Caches{{
				Keys: []string{"key"},
			}},
			true,
		},
		{ // No Keys
			v1.Caches{{
				Path: "/path",
			}},
			true,
		},
	} {
		err := tc.caches.Validate()
		if err == nil && tc.shouldError {
			t.Errorf("should error but returned err is nil")
		} else if err != nil && !tc.shouldError {
			t.Errorf("test errored but should not: %v", err)
		}
	}
}

func TestVolumesAndMounts(t *testing.T) {
	for _, tc := range []struct {
		caches  v1.Caches
		volumes []corev1.Volume
		mounts  []corev1.VolumeMount
	}{
		{ // Success
			v1.Caches{{
				Path: "/path/to-some/dir/",
				Keys: []string{"key"},
			}},
			[]corev1.Volume{
				{
					Name: "path-to-some-dir",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
			[]corev1.VolumeMount{
				{
					Name:      "path-to-some-dir",
					MountPath: "/path/to-some/dir/",
				},
			},
		},
	} {
		volumes, mounts := tc.caches.VolumesAndMounts()
		if !reflect.DeepEqual(volumes, tc.volumes) {
			t.Errorf("volumes: got = %v, want %v", volumes, tc.volumes)
		}
		if !reflect.DeepEqual(mounts, tc.mounts) {
			t.Errorf("mounts: got = %v, want %v", mounts, tc.mounts)
		}
	}
}

func TestTarBallName(t *testing.T) {
	for _, tc := range []struct {
		prepare     func(*v1.Cache) func()
		cache       v1.Cache
		shouldError bool
		shouldEqual string
	}{
		{
			cache:       v1.Cache{Path: "/test"},
			shouldError: false,
			shouldEqual: "test.tar.gz",
		},
		{
			cache:       v1.Cache{Path: "/some/random/path"},
			shouldError: false,
			shouldEqual: "some-random-path.tar.gz",
		},
		{
			prepare: func(cache *v1.Cache) func() {
				f, err := ioutil.TempFile("", "some-file")
				if err != nil {
					t.Fail()
				}
				if _, err := f.WriteString("a bit of content"); err != nil {
					t.Fail()
				}

				cache.Keys = []string{f.Name(), f.Name(), f.Name()}

				return func() {
					os.Remove(f.Name())
				}
			},
			cache:       v1.Cache{Path: "/name"},
			shouldError: false,
			shouldEqual: "name-a5c19f01912ff4c5df7224ebde46ec4872f38b902c78f34c6752b5002006f59a-a5c19f01912ff4c5df7224ebde46ec4872f38b902c78f34c6752b5002006f59a-a5c19f01912ff4c5df7224ebde46ec4872f38b902c78f34c6752b5002006f59a.tar.gz",
		},
		{

			cache:       v1.Cache{Keys: []string{"not-existing"}},
			shouldError: true,
		},
	} {
		if tc.prepare != nil {
			cleanup := tc.prepare(&tc.cache)
			defer cleanup()
		}
		dir, err := tc.cache.TarBallName()
		if err == nil {
			if tc.shouldError {
				t.Errorf("should error but returned err is nil")
			}
			if dir != tc.shouldEqual {
				t.Errorf("returned dir %q is not equal expected %q", dir, tc.shouldEqual)
			}
		} else if !tc.shouldError {
			t.Errorf("test errored but should not: %v", err)
		}
	}
}
