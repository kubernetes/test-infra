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

package v2

import (
	"context"
	"errors"
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/test-infra/boskos/common"
	"k8s.io/test-infra/boskos/crds"
)

const (
	testNamespace    = "my-ns"
	testResourceName = "my-resource"
)

func TestReconcile(t *testing.T) {
	verifyResourceObjectwasIgnored := func(c ctrlruntimeclient.Client) error {
		rO := &crds.ResourceObject{}
		name := types.NamespacedName{Namespace: testNamespace, Name: testResourceName}
		if err := c.Get(context.Background(), name, rO); err != nil {
			return fmt.Errorf("failed to get object: %v", err)
		}
		if rO.ResourceVersion != "1" {
			return errors.New("object got updated")
		}
		return nil
	}

	testCases := []struct {
		name    string
		objects []runtime.Object
		verify  func(ctrlruntimeclient.Client) error
	}{
		{
			name: "IsNotFound errors are ignored",
		},
		{
			name: "Resources with owners are ignored",
			objects: createTestObjects(func(rO *crds.ResourceObject, _ *crds.DRLCObject) {
				rO.Status.Owner = "hans"
			}),
			verify: verifyResourceObjectwasIgnored,
		},
		{
			name: "Non-dynamic resources are ignored",
			objects: createTestObjects(func(_ *crds.ResourceObject, drlc *crds.DRLCObject) {
				drlc.Name = "openstack-slice"
			}),
			verify: verifyResourceObjectwasIgnored,
		},
		{
			name: "Resources not in ToBeDeleted status are ignored",
			objects: createTestObjects(func(rO *crds.ResourceObject, _ *crds.DRLCObject) {
				rO.Status.State = "to-be-used"
			}),
			verify: verifyResourceObjectwasIgnored,
		},
		{
			name:    "State is updated to tombstone",
			objects: createTestObjects(),
			verify: func(c ctrlruntimeclient.Client) error {
				rO := &crds.ResourceObject{}
				name := types.NamespacedName{Namespace: testNamespace, Name: testResourceName}
				if err := c.Get(context.Background(), name, rO); err != nil {
					return fmt.Errorf("failed to get object: %v", err)
				}
				if rO.Status.State != common.Tombstone {
					return fmt.Errorf("state was not %q but %q", common.Tombstone, rO.Status.State)
				}
				return nil
			},
		},
	}

	for idx := range testCases {
		tc := testCases[idx]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client := fakectrlruntimeclient.NewFakeClient(tc.objects...)
			r := &reconciler{
				client:    client,
				namespace: testNamespace,
			}

			request := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: testResourceName}}

			if _, err := r.Reconcile(request); err != nil {
				t.Fatalf("reconciliation failed: %v", err)
			}
			if tc.verify == nil {
				return
			}
			if err := tc.verify(client); err != nil {
				t.Errorf("verification failed: %v", err)
			}
		})

	}
}

type testObjectModifier func(*crds.ResourceObject, *crds.DRLCObject)

func createTestObjects(modifiers ...testObjectModifier) []runtime.Object {

	drlcObject := &crds.DRLCObject{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      "aws-slice",
		},
	}

	resourceObject := &crds.ResourceObject{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       testNamespace,
			Name:            testResourceName,
			ResourceVersion: "1",
		},
		Spec: crds.ResourceSpec{
			Type: drlcObject.Name,
		},
		Status: crds.ResourceStatus{
			State: common.ToBeDeleted,
		},
	}

	for _, modify := range modifiers {
		modify(resourceObject, drlcObject)
	}

	return []runtime.Object{drlcObject, resourceObject}
}
