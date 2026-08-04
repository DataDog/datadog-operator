package clusterinfo

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func sampleInfo() *ClusterInfo {
	return &ClusterInfo{
		APIVersion:  APIVersion,
		ClusterName: "test-cluster",
		ClusterARN:  "arn:aws:eks:eu-west-3:013364996899:cluster/test-cluster",
		Region:      "eu-west-3",
		GeneratedAt: time.Date(2026, 4, 27, 14, 33, 0, 0, time.UTC),
		NodeManagement: map[NodeManager]map[string]NodeManagerEntry{
			NodeManagerFargate: {
				"dd-karpenter-test": {
					Nodes:            []string{"fargate-ip-10-0-0-1.eu-west-3.compute.internal"},
					ManagedByDatadog: true,
				},
			},
			NodeManagerStandalone: {
				"": {Nodes: []string{"ip-10-0-0-9.eu-west-3.compute.internal"}},
			},
		},
		Autoscaling: Autoscaling{
			ClusterAutoscaler: ClusterAutoscaler{Present: true, Namespace: "kube-system", Name: "cluster-autoscaler"},
			Karpenter: Karpenter{
				Present:          true,
				Namespace:        "dd-karpenter",
				Name:             "karpenter",
				Version:          "v1.9.0",
				ManagedByDatadog: true,
				InstallerVersion: "v0.7.0",
			},
			EKSAutoMode: EKSAutoMode{Enabled: false},
		},
	}
}

func TestPersist_CreatesConfigMap(t *testing.T) {
	cli := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	info := sampleInfo()

	err := Persist(t.Context(), cli, "dd-karpenter", info)
	require.NoError(t, err)

	got := &corev1.ConfigMap{}
	require.NoError(t, cli.Get(t.Context(), types.NamespacedName{Namespace: "dd-karpenter", Name: ConfigMapName}, got))
	assert.Equal(t, "kubectl-datadog", got.Labels["app.kubernetes.io/managed-by"])

	var roundTrip ClusterInfo
	require.NoError(t, yaml.Unmarshal([]byte(got.Data[ConfigMapDataKey]), &roundTrip))
	assert.Equal(t, info.APIVersion, roundTrip.APIVersion)
	assert.Equal(t, info.ClusterName, roundTrip.ClusterName)
	assert.True(t, info.GeneratedAt.Equal(roundTrip.GeneratedAt))
	assert.Equal(t, info.NodeManagement, roundTrip.NodeManagement)
	assert.Equal(t, info.Autoscaling, roundTrip.Autoscaling)
}

// TestPersist_YAMLShape locks the wire shape against the gopkg.in/yaml.v3
// lower-case-by-default footgun: if a struct field loses its `yaml:"..."`
// tag, the marshaled key would silently become `managedbydatadog` instead
// of `managedByDatadog`, breaking future readers of the ConfigMap.
func TestPersist_YAMLShape(t *testing.T) {
	cli := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	require.NoError(t, Persist(t.Context(), cli, "dd-karpenter", sampleInfo()))

	got := &corev1.ConfigMap{}
	require.NoError(t, cli.Get(t.Context(), types.NamespacedName{Namespace: "dd-karpenter", Name: ConfigMapName}, got))
	rendered := got.Data[ConfigMapDataKey]

	// Hierarchy + the lowerCamelCase keys we care about most.
	for _, want := range []string{
		"nodeManagement:",
		"autoscaling:",
		"clusterAutoscaler:",
		"karpenter:",
		"eksAutoMode:",
		"managedByDatadog: true",
		"installerVersion: v0.7.0",
		"enabled: false",
		"clusterArn: arn:aws:eks:eu-west-3:013364996899:cluster/test-cluster",
		"region: eu-west-3",
	} {
		assert.Contains(t, rendered, want, "wire format must keep the documented keys")
	}
}

func TestPersist_UpdatesExistingConfigMap(t *testing.T) {
	cli := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()

	first := sampleInfo()
	require.NoError(t, Persist(t.Context(), cli, "dd-karpenter", first))

	second := sampleInfo()
	second.ClusterName = "renamed-cluster"
	second.NodeManagement = map[NodeManager]map[string]NodeManagerEntry{
		NodeManagerASG: {"asg-1": {Nodes: []string{"node-x"}}},
	}
	require.NoError(t, Persist(t.Context(), cli, "dd-karpenter", second))

	got := &corev1.ConfigMap{}
	require.NoError(t, cli.Get(t.Context(), types.NamespacedName{Namespace: "dd-karpenter", Name: ConfigMapName}, got))

	var roundTrip ClusterInfo
	require.NoError(t, yaml.Unmarshal([]byte(got.Data[ConfigMapDataKey]), &roundTrip))
	assert.Equal(t, "renamed-cluster", roundTrip.ClusterName)
	assert.Equal(t, []string{"node-x"}, roundTrip.NodeManagement[NodeManagerASG]["asg-1"].Nodes)
	assert.NotContains(t, roundTrip.NodeManagement, NodeManagerFargate, "previous buckets should be gone")
}
