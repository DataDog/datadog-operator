package evict

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clusterinfo"
)

func TestParseTargetSpec(t *testing.T) {
	for _, tc := range []struct {
		name      string
		input     string
		want      Target
		wantError string
	}{
		{name: "asg", input: "asg/legacy-asg", want: Target{Manager: clusterinfo.NodeManagerASG, Entity: "legacy-asg"}},
		{name: "eks-mng", input: "eksManagedNodeGroup/standard-2", want: Target{Manager: clusterinfo.NodeManagerEKSManagedNodeGroup, Entity: "standard-2"}},
		{name: "karpenter", input: "karpenter/user-pool", want: Target{Manager: clusterinfo.NodeManagerKarpenter, Entity: "user-pool"}},
		{name: "standalone-no-slash", input: "standalone", want: Target{Manager: clusterinfo.NodeManagerStandalone, Entity: ""}},
		{name: "standalone-empty-after-slash", input: "standalone/", want: Target{Manager: clusterinfo.NodeManagerStandalone, Entity: ""}},
		{name: "leading-trailing-spaces", input: "  asg/foo  ", want: Target{Manager: clusterinfo.NodeManagerASG, Entity: "foo"}},

		{name: "empty", input: "", wantError: "cannot be empty"},
		{name: "asg-no-name", input: "asg", wantError: "requires a name"},
		{name: "asg-empty-name", input: "asg/", wantError: "requires a name"},
		{name: "standalone-with-name", input: "standalone/foo", wantError: "does not take a name"},
		{name: "fargate-rejected", input: "fargate/profile", wantError: "not supported"},
		{name: "unknown-rejected", input: "unknown/x", wantError: "not supported"},
		{name: "garbage", input: "blob/x", wantError: "unknown manager"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseTargetSpec(tc.input)
			if tc.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantError)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestBuildPlan_All(t *testing.T) {
	info := &clusterinfo.ClusterInfo{
		NodeManagement: map[clusterinfo.NodeManager]map[string]clusterinfo.NodeManagerEntry{
			clusterinfo.NodeManagerASG: {
				"legacy-asg-a": {Nodes: []string{"ip-1", "ip-2"}, ManagedByDatadog: false},
				"legacy-asg-b": {Nodes: []string{"ip-3"}, ManagedByDatadog: false},
			},
			clusterinfo.NodeManagerEKSManagedNodeGroup: {
				"mng-1":         {Nodes: []string{"ip-4"}, ManagedByDatadog: false},
				"datadog-owned": {Nodes: []string{"ip-5"}, ManagedByDatadog: true},
			},
			clusterinfo.NodeManagerKarpenter: {
				"user-np": {Nodes: []string{"ip-6"}, ManagedByDatadog: false},
				"dd-np":   {Nodes: []string{"ip-7"}, ManagedByDatadog: true},
			},
			clusterinfo.NodeManagerStandalone: {
				"": {Nodes: []string{"ip-8"}, ManagedByDatadog: false},
			},
			clusterinfo.NodeManagerFargate: {
				"profile-1": {Nodes: []string{"fargate-ip-9"}, ManagedByDatadog: false},
			},
			clusterinfo.NodeManagerUnknown: {
				"": {Nodes: []string{"weird-1"}, ManagedByDatadog: false},
			},
		},
	}
	got, err := BuildPlan(info, true, nil)
	require.NoError(t, err)

	// Datadog-managed entries dropped; fargate + unknown skipped.
	// Each non-DD entity present, with its nodes copied.
	entities := make([]string, 0, len(got))
	for _, t := range got {
		entities = append(entities, string(t.Manager)+"/"+t.Entity)
	}
	assert.ElementsMatch(t, []string{
		"karpenter/user-np",
		"eksManagedNodeGroup/mng-1",
		"asg/legacy-asg-a",
		"asg/legacy-asg-b",
		"standalone/",
	}, entities)
}

func TestBuildPlan_TargetedHappyPath(t *testing.T) {
	info := &clusterinfo.ClusterInfo{
		NodeManagement: map[clusterinfo.NodeManager]map[string]clusterinfo.NodeManagerEntry{
			clusterinfo.NodeManagerASG: {
				"my-asg": {Nodes: []string{"ip-1", "ip-2"}, ManagedByDatadog: false},
			},
		},
	}
	got, err := BuildPlan(info, false, []Target{
		{Manager: clusterinfo.NodeManagerASG, Entity: "my-asg"},
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "my-asg", got[0].Entity)
	assert.Equal(t, []string{"ip-1", "ip-2"}, got[0].Nodes)
}

func TestBuildPlan_TargetErrors(t *testing.T) {
	info := &clusterinfo.ClusterInfo{
		NodeManagement: map[clusterinfo.NodeManager]map[string]clusterinfo.NodeManagerEntry{
			clusterinfo.NodeManagerASG: {
				"dd-asg": {Nodes: []string{"ip-1"}, ManagedByDatadog: true},
			},
		},
	}

	t.Run("missing entity", func(t *testing.T) {
		_, err := BuildPlan(info, false, []Target{{Manager: clusterinfo.NodeManagerASG, Entity: "ghost"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("datadog-managed entity", func(t *testing.T) {
		_, err := BuildPlan(info, false, []Target{{Manager: clusterinfo.NodeManagerASG, Entity: "dd-asg"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "managed by Datadog")
	})

	t.Run("missing manager bucket", func(t *testing.T) {
		_, err := BuildPlan(info, false, []Target{{Manager: clusterinfo.NodeManagerKarpenter, Entity: "x"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no karpenter entities")
	})

	t.Run("no targets at all", func(t *testing.T) {
		_, err := BuildPlan(info, false, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one --target")
	})
}

func TestGroupByManager(t *testing.T) {
	targets := []Target{
		{Manager: clusterinfo.NodeManagerASG, Entity: "a"},
		{Manager: clusterinfo.NodeManagerASG, Entity: "b"},
		{Manager: clusterinfo.NodeManagerKarpenter, Entity: "k"},
	}
	got := groupByManager(targets)
	require.Len(t, got, 2)
	assert.Len(t, got[clusterinfo.NodeManagerASG], 2)
	assert.Len(t, got[clusterinfo.NodeManagerKarpenter], 1)
}

func TestHasDatadogManagedNodePool(t *testing.T) {
	t.Run("nil info", func(t *testing.T) {
		assert.False(t, hasDatadogManagedNodePool(nil))
	})
	t.Run("no karpenter bucket", func(t *testing.T) {
		info := &clusterinfo.ClusterInfo{NodeManagement: map[clusterinfo.NodeManager]map[string]clusterinfo.NodeManagerEntry{}}
		assert.False(t, hasDatadogManagedNodePool(info))
	})
	t.Run("only user NodePools", func(t *testing.T) {
		info := &clusterinfo.ClusterInfo{NodeManagement: map[clusterinfo.NodeManager]map[string]clusterinfo.NodeManagerEntry{
			clusterinfo.NodeManagerKarpenter: {
				"user-np": {ManagedByDatadog: false},
			},
		}}
		assert.False(t, hasDatadogManagedNodePool(info))
	})
	t.Run("at least one Datadog NodePool", func(t *testing.T) {
		info := &clusterinfo.ClusterInfo{NodeManagement: map[clusterinfo.NodeManager]map[string]clusterinfo.NodeManagerEntry{
			clusterinfo.NodeManagerKarpenter: {
				"user-np": {ManagedByDatadog: false},
				"dd-np":   {ManagedByDatadog: true},
			},
		}}
		assert.True(t, hasDatadogManagedNodePool(info))
	})
}
