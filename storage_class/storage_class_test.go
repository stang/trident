// Copyright 2018 NetApp, Inc. All Rights Reserved.

package storageclass

import (
	"strings"
	"testing"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage/factory"
	"github.com/netapp/trident/storage/fake"
	sa "github.com/netapp/trident/storage_attribute"
	tu "github.com/netapp/trident/storage_class/test_utils"
	fake_driver "github.com/netapp/trident/storage_drivers/fake"
)

func TestAttributeMatches(t *testing.T) {
	mockPools := tu.GetFakePools()
	config, err := fake_driver.NewFakeStorageDriverConfigJSON("mock", config.File,
		mockPools)
	if err != nil {
		t.Fatalf("Unable to construct config JSON.")
	}
	backend, err := factory.NewStorageBackendForConfig(config)
	if err != nil {
		t.Fatalf("Unable to construct backend using mock driver.")
	}
	for _, test := range []struct {
		name          string
		sc            *StorageClass
		expectedPools []string
	}{
		{
			name: "Slow",
			sc: New(&Config{
				Name: "slow",
				Attributes: map[string]sa.Request{
					sa.IOPS:             sa.NewIntRequest(40),
					sa.Snapshots:        sa.NewBoolRequest(true),
					sa.ProvisioningType: sa.NewStringRequest("thin"),
				},
			}),
			expectedPools: []string{tu.SlowSnapshots},
		},
		{
			// Tests that a request for false will return backends offering
			// both true and false
			name: "Bool",
			sc: New(&Config{
				Name: "no-snapshots",
				Attributes: map[string]sa.Request{
					sa.IOPS:             sa.NewIntRequest(40),
					sa.Snapshots:        sa.NewBoolRequest(false),
					sa.ProvisioningType: sa.NewStringRequest("thin"),
				},
			}),
			expectedPools: []string{tu.SlowSnapshots, tu.SlowNoSnapshots},
		},
		{
			name: "Fast",
			sc: New(&Config{
				Name: "fast",
				Attributes: map[string]sa.Request{
					sa.IOPS:             sa.NewIntRequest(2000),
					sa.Snapshots:        sa.NewBoolRequest(true),
					sa.ProvisioningType: sa.NewStringRequest("thin"),
				},
			}),
			expectedPools: []string{tu.FastSmall, tu.FastThinOnly,
				tu.FastUniqueAttr},
		},
		{
			// This tests the correctness of matching string requests, using
			// ProvisioningType
			name: "String",
			sc: New(&Config{
				Name: "string",
				Attributes: map[string]sa.Request{
					sa.IOPS:             sa.NewIntRequest(2000),
					sa.Snapshots:        sa.NewBoolRequest(true),
					sa.ProvisioningType: sa.NewStringRequest("thick"),
				},
			}),
			expectedPools: []string{tu.FastSmall, tu.FastUniqueAttr},
		},
		{
			// This uses sa.UniqueOptions to test that StorageClass only
			// matches backends that have all required attributes.
			name: "Unique",
			sc: New(&Config{
				Name: "unique",
				Attributes: map[string]sa.Request{
					sa.IOPS:             sa.NewIntRequest(2000),
					sa.Snapshots:        sa.NewBoolRequest(true),
					sa.ProvisioningType: sa.NewStringRequest("thick"),
					sa.UniqueOptions:    sa.NewStringRequest("bar"),
				},
			}),
			expectedPools: []string{tu.FastUniqueAttr},
		},
		{
			// Tests overlapping int ranges
			name: "Overlap",
			sc: New(&Config{
				Name: "overlap",
				Attributes: map[string]sa.Request{
					sa.IOPS:             sa.NewIntRequest(1000),
					sa.Snapshots:        sa.NewBoolRequest(true),
					sa.ProvisioningType: sa.NewStringRequest("thin"),
				},
			}),
			expectedPools: []string{tu.FastSmall, tu.FastThinOnly,
				tu.FastUniqueAttr, tu.MediumOverlap},
		},
		// BEGIN Failure tests
		{
			// Tests non-existent bool attribute
			name: "Bool failure",
			sc: New(&Config{
				Name: "bool-failure",
				Attributes: map[string]sa.Request{
					sa.IOPS:             sa.NewIntRequest(1000),
					sa.Snapshots:        sa.NewBoolRequest(true),
					sa.ProvisioningType: sa.NewStringRequest("thick"),
					sa.NonexistentBool:  sa.NewBoolRequest(false),
				},
			}),
			expectedPools: []string{},
		},
		{
			name: "Invalid string value",
			sc: New(&Config{
				Name: "invalid-string",
				Attributes: map[string]sa.Request{
					sa.IOPS:             sa.NewIntRequest(1000),
					sa.Snapshots:        sa.NewBoolRequest(true),
					sa.ProvisioningType: sa.NewStringRequest("thicker"),
				},
			}),
			expectedPools: []string{},
		},
		{
			name: "Invalid int range",
			sc: New(&Config{
				Name: "invalid-int",
				Attributes: map[string]sa.Request{
					sa.IOPS:             sa.NewIntRequest(300),
					sa.Snapshots:        sa.NewBoolRequest(true),
					sa.ProvisioningType: sa.NewStringRequest("thick"),
				},
			}),
			expectedPools: []string{},
		},
	} {
		test.sc.CheckAndAddBackend(backend)
		expectedMap := make(map[string]bool, len(test.expectedPools))
		for _, pool := range test.expectedPools {
			expectedMap[pool] = true
		}
		unmatched := make([]string, 0)
		matched := make([]string, 0)
		for _, vc := range test.sc.pools {
			if _, ok := expectedMap[vc.Name]; !ok {
				unmatched = append(unmatched, vc.Name)
			} else {
				delete(expectedMap, vc.Name)
				matched = append(matched, vc.Name)
			}
		}
		if len(unmatched) > 0 {
			t.Errorf("%s:\n\tFailed to match pools: %s\n\tMatched:  %s",
				test.name,
				strings.Join(unmatched, ", "),
				strings.Join(matched, ", "),
			)
		}
		if len(expectedMap) > 0 {
			expectedMatches := make([]string, 0)
			for k := range expectedMap {
				expectedMatches = append(expectedMatches, k)
			}
			t.Errorf("%s:\n\tExpected additional matches:  %s\n\tMatched: %s",
				test.name,
				strings.Join(expectedMatches, ", "),
				strings.Join(matched, ", "),
			)
		}
	}
}

func TestSpecificBackends(t *testing.T) {
	backends := make(map[string]*storage.Backend)
	mockPools := tu.GetFakePools()
	for _, c := range []struct {
		name      string
		poolNames []string
	}{
		{name: "fast-a",
			poolNames: []string{tu.FastSmall, tu.FastThinOnly}},
		{name: "fast-b",
			poolNames: []string{tu.FastThinOnly, tu.FastUniqueAttr}},
		{name: "slow",
			poolNames: []string{tu.SlowSnapshots, tu.SlowNoSnapshots, tu.MediumOverlap}},
	} {
		pools := make(map[string]*fake.StoragePool, len(c.poolNames))
		for _, poolName := range c.poolNames {
			pools[poolName] = mockPools[poolName]
		}
		config, err := fake_driver.NewFakeStorageDriverConfigJSON(c.name, config.File,
			pools)
		if err != nil {
			t.Fatalf("Unable to generate config JSON for %s:  %v", c.name, err)
		}
		backend, err := factory.NewStorageBackendForConfig(config)
		if err != nil {
			t.Fatalf("Unable to construct backend using mock driver.")
		}
		backends[c.name] = backend
	}
	for _, test := range []struct {
		name     string
		sc       *StorageClass
		expected []*tu.PoolMatch
	}{
		{
			name: "Specific backends",
			sc: New(&Config{
				Name: "specific",
				AdditionalPools: map[string][]string{
					"fast-a": {tu.FastThinOnly},
					"slow":   {tu.SlowNoSnapshots, tu.MediumOverlap},
				},
			}),
			expected: []*tu.PoolMatch{
				{Backend: "fast-a", Pool: tu.FastThinOnly},
				{Backend: "slow", Pool: tu.SlowNoSnapshots},
				{Backend: "slow", Pool: tu.MediumOverlap},
			},
		},
		{
			name: "Mixed attributes, backends",
			sc: New(&Config{
				Name: "mixed",
				AdditionalPools: map[string][]string{
					"slow":   {tu.SlowNoSnapshots},
					"fast-b": {tu.FastThinOnly, tu.FastUniqueAttr},
				},
				Attributes: map[string]sa.Request{
					sa.IOPS:             sa.NewIntRequest(2000),
					sa.Snapshots:        sa.NewBoolRequest(true),
					sa.ProvisioningType: sa.NewStringRequest("thin"),
				},
			}),
			expected: []*tu.PoolMatch{
				{Backend: "fast-a", Pool: tu.FastSmall},
				{Backend: "fast-a", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastUniqueAttr},
				{Backend: "slow", Pool: tu.SlowNoSnapshots},
			},
		},
	} {
		for _, backend := range backends {
			test.sc.CheckAndAddBackend(backend)
		}
		for _, protocol := range []config.Protocol{config.File, config.Block} {
			for _, vc := range test.sc.GetStoragePoolsForProtocol(protocol) {
				nameFound := false
				for _, scName := range vc.StorageClasses {
					if scName == test.sc.GetName() {
						nameFound = true
						break
					}
				}
				if !nameFound {
					t.Errorf("%s: Storage class name not found in storage "+
						"pool %s", test.name, vc.Name)
				}
				matchIndex := -1
				for i, e := range test.expected {
					if e.Matches(vc) {
						matchIndex = i
						break
					}
				}
				if matchIndex >= 0 {
					// If we match, remove the match from the potential matches.
					test.expected[matchIndex] = test.expected[len(
						test.expected)-1]
					test.expected[len(test.expected)-1] = nil
					test.expected = test.expected[:len(test.expected)-1]
				} else {
					t.Errorf("%s:  Found unexpected match for storage class:  "+
						"%s:%s", test.name, vc.Backend.Name, vc.Name)
				}
			}
		}
		if len(test.expected) > 0 {
			expectedNames := make([]string, len(test.expected))
			for i, e := range test.expected {
				expectedNames[i] = e.String()
			}
			t.Errorf("%s:  Storage class failed to match storage pools %s",
				test.name, strings.Join(expectedNames, ", "))
		}
	}
}

func TestRegex(t *testing.T) {
	backends := make(map[string]*storage.Backend)
	mockPools := tu.GetFakePools()
	for _, c := range []struct {
		backendName string
		poolNames   []string
	}{
		{backendName: "fast-a",
			poolNames: []string{tu.FastSmall, tu.FastThinOnly}},
		{backendName: "fast-b",
			poolNames: []string{tu.FastThinOnly, tu.FastUniqueAttr}},
		{backendName: "slow",
			poolNames: []string{tu.SlowSnapshots, tu.SlowNoSnapshots, tu.MediumOverlap}},
	} {
		pools := make(map[string]*fake.StoragePool, len(c.poolNames))
		for _, poolName := range c.poolNames {
			pools[poolName] = mockPools[poolName]
		}
		config, err := fake_driver.NewFakeStorageDriverConfigJSON(c.backendName, config.File, pools)
		if err != nil {
			t.Fatalf("Unable to generate config JSON for %s:  %v", c.backendName, err)
		}
		backend, err := factory.NewStorageBackendForConfig(config)
		if err != nil {
			t.Fatalf("Unable to construct backend using mock driver.")
		}
		backends[c.backendName] = backend
	}
	for _, test := range []struct {
		description string
		sc          *StorageClass
		expected    []*tu.PoolMatch
	}{
		{
			description: "All pools for the 'slow' backend via regex '.*''",
			sc: New(&Config{
				Name: "regex1",
				Pools: map[string][]string{
					"slow": {".*"},
				},
			}),
			expected: []*tu.PoolMatch{
				{Backend: "slow", Pool: tu.SlowSnapshots},
				{Backend: "slow", Pool: tu.SlowNoSnapshots},
				{Backend: "slow", Pool: tu.MediumOverlap},
			},
		},
		{
			description: "TBD",
			sc: New(&Config{
				Name: "regex1b",
				Pools: map[string][]string{
					"slow": {"slow.*", tu.MediumOverlap},
				},
			}),
			expected: []*tu.PoolMatch{
				{Backend: "slow", Pool: tu.SlowSnapshots},
				{Backend: "slow", Pool: tu.SlowNoSnapshots},
				{Backend: "slow", Pool: tu.MediumOverlap},
			},
		},
		{
			description: "Implicitly exclude the medium-overlap pool via regex by only matching those starting with 'slow-'",
			sc: New(&Config{
				Name: "regex2",
				Pools: map[string][]string{
					"slow": {"slow-.*"},
				},
			}),
			expected: []*tu.PoolMatch{
				{Backend: "slow", Pool: tu.SlowSnapshots},
				{Backend: "slow", Pool: tu.SlowNoSnapshots},
			},
		},
		{
			description: "Backends whose names start with 'fast-' and all of their respective pools",
			sc: New(&Config{
				Name: "regex3",
				Pools: map[string][]string{
					"fast-.*": {".*"}, // All fast backends:  all their pools
				},
			}),
			expected: []*tu.PoolMatch{
				{Backend: "fast-a", Pool: tu.FastSmall},
				{Backend: "fast-a", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastUniqueAttr},
			},
		},
		{
			description: "Backends {fast-a, slow} and all of their respective pools",
			sc: New(&Config{
				Name: "regex4",
				Pools: map[string][]string{
					"fast-a|slow": {".*"}, // fast-a or slow:  all their pools
				},
			}),
			expected: []*tu.PoolMatch{
				{Backend: "fast-a", Pool: tu.FastSmall},
				{Backend: "fast-a", Pool: tu.FastThinOnly},
				{Backend: "slow", Pool: tu.SlowSnapshots},
				{Backend: "slow", Pool: tu.SlowNoSnapshots},
				{Backend: "slow", Pool: tu.MediumOverlap},
			},
		},
		{
			description: "Backends {fast-a, slow} and all of their respective pools",
			sc: New(&Config{
				Name: "regex5",
				Pools: map[string][]string{
					"(fast-a|slow|NOT_ACTUALLY_THERE)": {".*"}, // fast-a or slow or 1 that doesn't exist:  all their pools
				},
			}),
			expected: []*tu.PoolMatch{
				{Backend: "fast-a", Pool: tu.FastSmall},
				{Backend: "fast-a", Pool: tu.FastThinOnly},
				{Backend: "slow", Pool: tu.SlowSnapshots},
				{Backend: "slow", Pool: tu.SlowNoSnapshots},
				{Backend: "slow", Pool: tu.MediumOverlap},
			},
		},
		{
			description: "Every backend and every pool",
			sc: New(&Config{
				Name: "regex6",
				Pools: map[string][]string{
					".*": {".*"}, // All backends:  all pools
				},
			}),
			expected: []*tu.PoolMatch{
				{Backend: "slow", Pool: tu.SlowSnapshots},
				{Backend: "slow", Pool: tu.SlowNoSnapshots},
				{Backend: "slow", Pool: tu.MediumOverlap},
				{Backend: "fast-a", Pool: tu.FastSmall},
				{Backend: "fast-a", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastUniqueAttr},
			},
		},
		{
			description: "Exclusion test",
			sc: New(&Config{
				Name: "regex7",
				Pools: map[string][]string{
					".*": {".*"}, // Start off allowing all backends:  all pools
				},
				ExcludePools: map[string][]string{
					"slow": {tu.MediumOverlap}, // Now, specifically exclude backend 'slow':  tu.MediumOverlap pool
				},
			}),
			expected: []*tu.PoolMatch{
				{Backend: "slow", Pool: tu.SlowSnapshots},
				{Backend: "slow", Pool: tu.SlowNoSnapshots},
				{Backend: "fast-a", Pool: tu.FastSmall},
				{Backend: "fast-a", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastUniqueAttr},
			},
		},
		{
			description: "Exclude the FastThinOnly pools from the fast backends",
			sc: New(&Config{
				Name: "regex8",
				Pools: map[string][]string{
					"fast-.*": {".*"}, // All fast backends:  all their pools
				},
				ExcludePools: map[string][]string{
					".*": {tu.FastThinOnly}, // Now, specifically exclude backend 'slow':  tu.MediumOverlap pool
				},
			}),
			expected: []*tu.PoolMatch{
				{Backend: "fast-a", Pool: tu.FastSmall},
				{Backend: "fast-b", Pool: tu.FastUniqueAttr},
			},
		},
	} {
		// add the backends to the test StorageClass
		for _, backend := range backends {
			test.sc.CheckAndAddBackend(backend)
		}
		// validate the results for the test StorageClass
		for _, protocol := range []config.Protocol{config.File, config.Block} {
			for _, pool := range test.sc.GetStoragePoolsForProtocol(protocol) {
				nameFound := false
				for _, scName := range pool.StorageClasses {
					if scName == test.sc.GetName() {
						nameFound = true
						break
					}
				}
				if !nameFound {
					t.Errorf("%s: Storage class name not found in storage pool: %s", test.description, pool.Name)
				}
				matchIndex := -1
				for i, e := range test.expected {
					if e.Matches(pool) {
						matchIndex = i
						break
					}
				}
				if matchIndex >= 0 {
					// If we match, remove the match from the potential matches.
					test.expected[matchIndex] = test.expected[len(test.expected)-1]
					test.expected[len(test.expected)-1] = nil
					test.expected = test.expected[:len(test.expected)-1]
				} else {
					t.Errorf("%s:  Found unexpected match for storage class: %s:%s", test.description, pool.Backend.Name, pool.Name)
				}
			}
		}
		if len(test.expected) > 0 {
			expectedNames := make([]string, len(test.expected))
			for i, e := range test.expected {
				expectedNames[i] = e.String()
			}
			t.Errorf("%s:  Storage class failed to match storage pools %s", test.description, strings.Join(expectedNames, ", "))
		}
	}
}
