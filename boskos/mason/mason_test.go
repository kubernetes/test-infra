/*
Copyright 2017 The Kubernetes Authors.

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

package mason

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"context"
	"sync"

	"k8s.io/test-infra/boskos/common"
	"k8s.io/test-infra/boskos/ranch"
	"k8s.io/test-infra/boskos/storage"
)

const (
	fakeConfigType = "fakeConfig"
	emptyContent   = "empty content"
	owner          = "mason"
)

type fakeBoskos struct {
	ranch *ranch.Ranch
}

type testConfig map[string]struct {
	resourceNeeds *common.ResourceNeeds
	count         int
}

type fakeConfig struct {
}

func fakeConfigConverter(in string) (Masonable, error) {
	return &fakeConfig{}, nil
}

func (fc *fakeConfig) Construct(res *common.Resource, typeToRes common.TypeToResources) (common.UserData, error) {
	return common.UserData{"fakeConfig": "unused"}, nil
}

// Create a fake client
func createFakeBoskos(tc testConfig) (*ranch.Storage, *Client, []common.ResourcesConfig) {
	configNames := map[string]bool{}
	var configs []common.ResourcesConfig
	s, _ := ranch.NewStorage(storage.NewMemoryStorage(), "")
	r, _ := ranch.NewRanch("", s)

	for rtype, c := range tc {
		for i := 0; i < c.count; i++ {
			res := common.Resource{
				Type:     rtype,
				Name:     fmt.Sprintf("%s_%d", rtype, i),
				State:    common.Free,
				UserData: common.UserData{},
			}
			if c.resourceNeeds != nil {
				res.State = common.Dirty
				if _, ok := configNames[rtype]; !ok {
					configNames[rtype] = true
					configs = append(configs, common.ResourcesConfig{
						Config: common.ConfigType{
							Type:    fakeConfigType,
							Content: emptyContent,
						},
						Name:  rtype,
						Needs: *c.resourceNeeds,
					})
				}
			}
			s.AddResource(res)
		}
	}
	return s, NewClient(&fakeBoskos{r}), configs
}

func (fb *fakeBoskos) Acquire(rtype, state, dest string) (*common.Resource, error) {
	return fb.ranch.Acquire(rtype, state, dest, owner)
}

func (fb *fakeBoskos) AcquireByState(state, dest string, names []string) ([]common.Resource, error) {
	return fb.ranch.AcquireByState(state, dest, owner, names)
}

func (fb *fakeBoskos) ReleaseOne(name, dest string) error {
	return fb.ranch.Release(name, dest, owner)
}

func (fb *fakeBoskos) UpdateOne(name, state string, userData common.UserData) error {
	return fb.ranch.Update(name, owner, state, userData)
}

func (fb *fakeBoskos) UpdateAll(state string) error {
	// not used in this test
	return nil
}

func TestRecycleLeasedResources(t *testing.T) {
	tc := testConfig{
		"type1": {
			count: 1,
		},
		"type2": {
			resourceNeeds: &common.ResourceNeeds{
				"type1": 1,
			},
			count: 1,
		},
	}

	rStorage, mClient, configs := createFakeBoskos(tc)
	res1, _ := rStorage.GetResource("type1_0")
	res1.State = "type2_0"
	rStorage.UpdateResource(res1)
	res2, _ := rStorage.GetResource("type2_0")
	res2.UserData.Set(LeasedResources, &[]string{"type1_0"})
	rStorage.UpdateResource(res2)
	m := NewMason(1, 1, mClient.basic, 50*time.Millisecond)
	m.storage.SyncConfigs(configs)
	m.RegisterConfigConverter(fakeConfigType, fakeConfigConverter)
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.start(ctx, m.recycleAll)
	select {
	case <-m.pending:
		break
	case <-time.After(1 * time.Second):
		t.Errorf("Timeout")
	}
	m.Stop()
	res1, _ = rStorage.GetResource("type1_0")
	res2, _ = rStorage.GetResource("type2_0")
	if res2.State != common.Cleaning {
		t.Errorf("Resource state should be cleaning, found %s", res2.State)
	}
	if res1.State != common.Dirty {
		t.Errorf("Resource state should be dirty, found %s", res1.State)
	}
}

func TestRecycleNoLeasedResources(t *testing.T) {
	tc := testConfig{
		"type1": {
			count: 1,
		},
		"type2": {
			resourceNeeds: &common.ResourceNeeds{
				"type1": 1,
			},
			count: 1,
		},
	}

	rStorage, mClient, configs := createFakeBoskos(tc)
	m := NewMason(1, 1, mClient.basic, 50*time.Millisecond)
	m.storage.SyncConfigs(configs)
	m.RegisterConfigConverter(fakeConfigType, fakeConfigConverter)
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.start(ctx, m.recycleAll)
	select {
	case <-m.pending:
		break
	case <-time.After(1 * time.Second):
		t.Errorf("Timeout")
	}
	m.Stop()
	res1, _ := rStorage.GetResource("type1_0")
	res2, _ := rStorage.GetResource("type2_0")
	if res2.State != common.Cleaning {
		t.Errorf("Resource state should be cleaning")
	}
	if res1.State != common.Free {
		t.Errorf("Resource state should be untouched, current %s", mClient.resources["type1_0"].State)
	}
}

func TestFulfillOne(t *testing.T) {
	tc := testConfig{
		"type1": {
			count: 1,
		},
		"type2": {
			resourceNeeds: &common.ResourceNeeds{
				"type1": 1,
			},
			count: 1,
		},
	}

	rStorage, mClient, configs := createFakeBoskos(tc)
	m := NewMason(1, 1, mClient.basic, 50*time.Millisecond)
	m.storage.SyncConfigs(configs)
	res, _ := mClient.basic.Acquire("type2", common.Dirty, common.Cleaning)
	conf, err := m.storage.GetConfig("type2")
	if err != nil {
		t.Error("failed to get config")
	}
	req := requirements{
		resource:    *res,
		needs:       conf.Needs,
		fulfillment: common.TypeToResources{},
	}
	if err = m.fulfillOne(context.Background(), &req); err != nil {
		t.Errorf("could not satisty requirements ")
	}
	if len(req.fulfillment) != 1 {
		t.Errorf("there should be only one type")
	}
	if len(req.fulfillment["type1"]) != 1 {
		t.Errorf("there should be only one resources")
	}
	userRes := req.fulfillment["type1"][0]
	leasedResource, _ := rStorage.GetResource(userRes.Name)
	if leasedResource.State != common.Leased {
		t.Errorf("State should be Leased")
	}
	*res, _ = rStorage.GetResource(res.Name)
	var leasedResources common.LeasedResources
	if res.UserData.Extract(LeasedResources, &leasedResources); err != nil {
		t.Errorf("unable to extract %s", LeasedResources)
	}
	if res.UserData[LeasedResources] != req.resource.UserData[LeasedResources] {
		t.Errorf(
			"resource user data from requirement %v should be the same as the one received %v",
			req.resource.UserData[LeasedResources], res.UserData[LeasedResources])
	}
	if len(leasedResources) != 1 {
		t.Errorf("there should be one leased resource, found %d", len(leasedResources))
	}
	if leasedResources[0] != leasedResource.Name {
		t.Errorf("Leased resource don t match")
	}
}

func TestMason(t *testing.T) {
	tc := testConfig{
		"type1": {
			count: 10,
		},
		"type2": {
			resourceNeeds: &common.ResourceNeeds{
				"type1": 1,
			},
			count: 10,
		},
	}
	rStorage, mClient, configs := createFakeBoskos(tc)
	m := NewMason(5, 5, mClient.basic, 50*time.Millisecond)
	m.storage.SyncConfigs(configs)
	m.RegisterConfigConverter(fakeConfigType, fakeConfigConverter)
	m.Start()
	<-time.After(time.Second)
	resources, _ := rStorage.GetResources()
	for _, r := range resources {
		switch r.Type {
		case "type1":
			if !strings.Contains(r.State, "type2_") {
				t.Errorf("state should be starting with type2, found %s", r.State)
			}
		case "type2":
			if r.State != common.Free {
				t.Errorf("state should be %s, found %s", common.Free, r.State)
			}
		}
	}

	leasedResourceFromRes := func(r common.Resource) (l []common.Resource) {
		var leasedResources []string
		r.UserData.Extract(LeasedResources, &leasedResources)
		for _, name := range leasedResources {
			r, _ := rStorage.GetResource(name)
			l = append(l, r)
		}
		return
	}

	var resourcesToRelease []common.Resource

	for i := 0; i < 10; i++ {
		res, err := mClient.Acquire("type2", common.Free, "Used")
		if err != nil {
			t.Errorf("Count %d: There should be free resources", i)
		}
		leasedResources := leasedResourceFromRes(*res)
		if len(leasedResources) != 1 {
			t.Error("there should be 1 resource of type1")
		}
		for _, r := range leasedResources {
			if r.Type != "type1" {
				t.Error("resource should be of type type1")
			}
		}
		resourcesToRelease = append(resourcesToRelease, *res)
	}
	if _, err := mClient.Acquire("type2", common.Free, "Used"); err == nil {
		t.Error("there should not be any resource left")
	}
	m.Stop()
	for _, res := range resourcesToRelease {
		if err := mClient.ReleaseOne(res.Name, common.Dirty); err != nil {
			t.Error("unable to release leased resources")

		}
	}
	resources, _ = rStorage.GetResources()
	for _, r := range resources {
		if r.State != common.Dirty {
			t.Errorf("state should be %s, found %s", common.Dirty, r.State)
		}
	}
}

func TestMasonStartStop(t *testing.T) {
	tc := testConfig{
		"type1": {
			count: 10,
		},
		"type2": {
			resourceNeeds: &common.ResourceNeeds{
				"type1": 1,
			},
			count: 10,
		},
	}
	_, mClient, configs := createFakeBoskos(tc)
	m := NewMason(5, 5, mClient.basic, 50*time.Millisecond)
	m.storage.SyncConfigs(configs)
	m.RegisterConfigConverter(fakeConfigType, fakeConfigConverter)
	m.Start()
	var done bool
	var lock sync.RWMutex
	go func() {
		m.Stop()
		lock.Lock()
		defer lock.Unlock()
		done = true
	}()
	<-time.After(1 * time.Second)
	lock.Lock()
	defer lock.Unlock()
	if !done {
		t.Errorf("unable to stop mason")
	}
}

func TestConfig(t *testing.T) {
	resources, err := ranch.ParseConfig("test-resources.yaml")
	if err != nil {
		t.Error(err)
	}
	configs, err := ParseConfig("test-configs.yaml")
	if err != nil {
		t.Error(err)
	}
	if err := ValidateConfig(configs, resources); err == nil {
		t.Error(err)
	}
}

func makeFakeConfig(name, cType, content string, needs int) common.ResourcesConfig {
	c := common.ResourcesConfig{
		Name:  name,
		Needs: common.ResourceNeeds{},
		Config: common.ConfigType{
			Type:    cType,
			Content: content,
		},
	}
	for i := 0; i < needs; i++ {
		c.Needs[fmt.Sprintf("type_%d", i)] = i
	}
	return c
}

func TestSyncConfig(t *testing.T) {
	var testcases = []struct {
		name      string
		oldConfig []common.ResourcesConfig
		newConfig []common.ResourcesConfig
		expect    []common.ResourcesConfig
	}{
		{
			name: "empty",
		},
		{
			name: "deleteAll",
			oldConfig: []common.ResourcesConfig{
				makeFakeConfig("config1", "fakeType", "", 2),
				makeFakeConfig("config2", "fakeType", "", 3),
				makeFakeConfig("config3", "fakeType", "", 4),
			},
		},
		{
			name: "new",
			newConfig: []common.ResourcesConfig{
				makeFakeConfig("config1", "fakeType", "", 2),
				makeFakeConfig("config2", "fakeType", "", 3),
				makeFakeConfig("config3", "fakeType", "", 4),
			},
			expect: []common.ResourcesConfig{
				makeFakeConfig("config1", "fakeType", "", 2),
				makeFakeConfig("config2", "fakeType", "", 3),
				makeFakeConfig("config3", "fakeType", "", 4),
			},
		},
		{
			name: "noChange",
			oldConfig: []common.ResourcesConfig{
				makeFakeConfig("config1", "fakeType", "", 2),
				makeFakeConfig("config2", "fakeType", "", 3),
				makeFakeConfig("config3", "fakeType", "", 4),
			},
			newConfig: []common.ResourcesConfig{
				makeFakeConfig("config1", "fakeType", "", 2),
				makeFakeConfig("config2", "fakeType", "", 3),
				makeFakeConfig("config3", "fakeType", "", 4),
			},
			expect: []common.ResourcesConfig{
				makeFakeConfig("config1", "fakeType", "", 2),
				makeFakeConfig("config2", "fakeType", "", 3),
				makeFakeConfig("config3", "fakeType", "", 4),
			},
		},
		{
			name: "update",
			oldConfig: []common.ResourcesConfig{
				makeFakeConfig("config1", "fakeType", "", 2),
				makeFakeConfig("config2", "fakeType", "", 3),
				makeFakeConfig("config3", "fakeType", "", 4),
			},
			newConfig: []common.ResourcesConfig{
				makeFakeConfig("config1", "fakeType2", "", 2),
				makeFakeConfig("config2", "fakeType", "something", 3),
				makeFakeConfig("config3", "fakeType", "", 5),
			},
			expect: []common.ResourcesConfig{
				makeFakeConfig("config1", "fakeType2", "", 2),
				makeFakeConfig("config2", "fakeType", "something", 3),
				makeFakeConfig("config3", "fakeType", "", 5),
			},
		},
		{
			name: "delete",
			oldConfig: []common.ResourcesConfig{
				makeFakeConfig("config1", "fakeType", "", 2),
				makeFakeConfig("config2", "fakeType", "", 3),
				makeFakeConfig("config3", "fakeType", "", 4),
			},
			newConfig: []common.ResourcesConfig{
				makeFakeConfig("config1", "fakeType2", "", 2),
				makeFakeConfig("config3", "fakeType", "", 5),
			},
			expect: []common.ResourcesConfig{
				makeFakeConfig("config1", "fakeType2", "", 2),
				makeFakeConfig("config3", "fakeType", "", 5),
			},
		},
	}

	for _, tc := range testcases {
		s := newStorage(storage.NewMemoryStorage())
		s.SyncConfigs(tc.newConfig)
		configs, err := s.GetConfigs()
		if err != nil {
			t.Errorf("failed to get resources")
			continue
		}
		sort.Stable(common.ResourcesConfigByName(configs))
		sort.Stable(common.ResourcesConfigByName(tc.expect))
		if !reflect.DeepEqual(configs, tc.expect) {
			t.Errorf("Test %v: got %v, expect %v", tc.name, configs, tc.expect)
		}
	}
}

func TestGetConfig(t *testing.T) {
	var testcases = []struct {
		name, configName string
		exists           bool
		configs          []common.ResourcesConfig
	}{
		{
			name:       "exists",
			exists:     true,
			configName: "test",
			configs: []common.ResourcesConfig{
				{
					Needs: common.ResourceNeeds{"type1": 1, "type2": 2},
					Config: common.ConfigType{
						Type:    "type3",
						Content: "content",
					},
					Name: "test",
				},
			},
		},
		{
			name:       "noConfig",
			exists:     false,
			configName: "test",
		},
		{
			name:       "existsMultipleConfigs",
			exists:     true,
			configName: "test1",
			configs: []common.ResourcesConfig{
				{
					Needs: common.ResourceNeeds{"type1": 1, "type2": 2},
					Config: common.ConfigType{
						Type:    "type3",
						Content: "content",
					},
					Name: "test",
				},
				{
					Needs: common.ResourceNeeds{"type1": 1, "type2": 2},
					Config: common.ConfigType{
						Type:    "type3",
						Content: "content",
					},
					Name: "test1",
				},
			},
		},
		{
			name:       "noExistMultipleConfigs",
			exists:     false,
			configName: "test2",
			configs: []common.ResourcesConfig{
				{
					Needs: common.ResourceNeeds{"type1": 1, "type2": 2},
					Config: common.ConfigType{
						Type:    "type3",
						Content: "content",
					},
					Name: "test",
				},
				{
					Needs: common.ResourceNeeds{"type1": 1, "type2": 2},
					Config: common.ConfigType{
						Type:    "type3",
						Content: "content",
					},
					Name: "test1",
				},
			},
		},
	}
	for _, tc := range testcases {
		s := newStorage(storage.NewMemoryStorage())
		for _, config := range tc.configs {
			s.AddConfig(config)
		}
		config, err := s.GetConfig(tc.configName)
		if !tc.exists {
			if err == nil {
				t.Error("client should return an error")
			}
		} else {
			if config.Name != tc.configName {
				t.Error("config name should match")
			}
		}
	}
}
