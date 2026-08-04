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
				assert.ErrorContains(t, err, tc.wantError)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

func TestBuildPlan(t *testing.T) {
	mixedInfo := &clusterinfo.ClusterInfo{
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
	asgOnlyInfo := &clusterinfo.ClusterInfo{
		NodeManagement: map[clusterinfo.NodeManager]map[string]clusterinfo.NodeManagerEntry{
			clusterinfo.NodeManagerASG: {
				"my-asg": {Nodes: []string{"ip-1", "ip-2"}, ManagedByDatadog: false},
			},
		},
	}
	ddASGInfo := &clusterinfo.ClusterInfo{
		NodeManagement: map[clusterinfo.NodeManager]map[string]clusterinfo.NodeManagerEntry{
			clusterinfo.NodeManagerASG: {
				"dd-asg": {Nodes: []string{"ip-1"}, ManagedByDatadog: true},
			},
		},
	}

	for _, tc := range []struct {
		name    string
		info    *clusterinfo.ClusterInfo
		all     bool
		targets []Target

		wantErr         bool
		wantErrContains string
		// wantEntities, when non-nil, is the expected set of `<manager>/<entity>`
		// strings in the returned plan. ElementsMatch is used (order
		// irrelevant). nil disables the check.
		wantEntities []string
		// wantNodes maps entity name to its expected Nodes slice in the
		// returned plan (only used in the targeted happy-path case).
		wantNodes map[string][]string
	}{
		{
			// --all: Datadog-managed entries dropped; fargate + unknown
			// skipped entirely.
			name: "--all returns every non-Datadog entity",
			info: mixedInfo,
			all:  true,
			wantEntities: []string{
				"karpenter/user-np",
				"eksManagedNodeGroup/mng-1",
				"asg/legacy-asg-a",
				"asg/legacy-asg-b",
				"standalone/",
			},
		},
		{
			name:         "targeted happy path",
			info:         asgOnlyInfo,
			targets:      []Target{{Manager: clusterinfo.NodeManagerASG, Entity: "my-asg"}},
			wantEntities: []string{"asg/my-asg"},
			wantNodes:    map[string][]string{"my-asg": {"ip-1", "ip-2"}},
		},
		{
			name:            "missing entity errors",
			info:            ddASGInfo,
			targets:         []Target{{Manager: clusterinfo.NodeManagerASG, Entity: "ghost"}},
			wantErr:         true,
			wantErrContains: "not found",
		},
		{
			name:            "datadog-managed entity is refused",
			info:            ddASGInfo,
			targets:         []Target{{Manager: clusterinfo.NodeManagerASG, Entity: "dd-asg"}},
			wantErr:         true,
			wantErrContains: "managed by Datadog",
		},
		{
			name:            "missing manager bucket errors",
			info:            ddASGInfo,
			targets:         []Target{{Manager: clusterinfo.NodeManagerKarpenter, Entity: "x"}},
			wantErr:         true,
			wantErrContains: "no karpenter entities",
		},
		{
			name:            "no targets and not --all errors",
			info:            ddASGInfo,
			wantErr:         true,
			wantErrContains: "at least one --target",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BuildPlan(tc.info, tc.all, tc.targets)
			if tc.wantErr {
				require.Error(t, err)
				if tc.wantErrContains != "" {
					assert.Contains(t, err.Error(), tc.wantErrContains)
				}
				return
			}
			require.NoError(t, err)
			if tc.wantEntities != nil {
				entities := make([]string, 0, len(got))
				for _, g := range got {
					entities = append(entities, string(g.Manager)+"/"+g.Entity)
				}
				assert.ElementsMatch(t, tc.wantEntities, entities)
			}
			for entity, wantNodes := range tc.wantNodes {
				found := false
				for _, g := range got {
					if g.Entity == entity {
						assert.Equal(t, wantNodes, g.Nodes)
						found = true
						break
					}
				}
				assert.True(t, found, "expected entity %q in plan", entity)
			}
		})
	}
}

func TestHasDatadogManagedNodePool(t *testing.T) {
	for _, tc := range []struct {
		name string
		info *clusterinfo.ClusterInfo
		want bool
	}{
		{
			name: "no karpenter bucket",
			info: &clusterinfo.ClusterInfo{NodeManagement: map[clusterinfo.NodeManager]map[string]clusterinfo.NodeManagerEntry{}},
			want: false,
		},
		{
			name: "only user NodePools",
			info: &clusterinfo.ClusterInfo{NodeManagement: map[clusterinfo.NodeManager]map[string]clusterinfo.NodeManagerEntry{
				clusterinfo.NodeManagerKarpenter: {"user-np": {ManagedByDatadog: false}},
			}},
			want: false,
		},
		{
			name: "at least one Datadog NodePool",
			info: &clusterinfo.ClusterInfo{NodeManagement: map[clusterinfo.NodeManager]map[string]clusterinfo.NodeManagerEntry{
				clusterinfo.NodeManagerKarpenter: {
					"user-np": {ManagedByDatadog: false},
					"dd-np":   {ManagedByDatadog: true},
				},
			}},
			want: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, hasDatadogManagedNodePool(tc.info))
		})
	}
}
