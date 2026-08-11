package config

import (
	"os"
	"reflect"
	"testing"

	"golang.org/x/exp/maps"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

type objectConfig struct {
	configured bool
	namespaces []string
	// noPodLabel, when true, asserts Pod ByObject has no label selector (widened informer).
	noPodLabel bool
}

func Test_CacheConfig(t *testing.T) {

	tests := []struct {
		name string

		watchOptions WatchOptions
		envConfig    map[string]string

		wantDefaultNamepsace objectConfig
		wantObjectConfig     map[client.Object]objectConfig
	}{
		{
			name: "All envs non empty, all CRDs enabled",
			watchOptions: WatchOptions{
				DatadogAgentEnabled:           true,
				DatadogMonitorEnabled:         true,
				DatadogSLOEnabled:             true,
				DatadogAgentProfileEnabled:    true,
				DatadogDashboardEnabled:       true,
				DatadogGenericResourceEnabled: true,
				DatadogCSIDriverEnabled:       true,
			},

			envConfig: map[string]string{
				WatchNamespaceEnvVar:                "datadog",
				AgentWatchNamespaceEnvVar:           "agentNs",
				monitorWatchNamespaceEnvVar:         "monitorNs, monitorNs2",
				sloWatchNamespaceEnvVar:             "  nsWithSpace ",
				profileWatchNamespaceEnvVar:         "profileNs",
				dashboardWatchNamespaceEnvVar:       "dashboardNs",
				genericResourceWatchNamespaceEnvVar: "genericNs",
				csiDriverWatchNamespaceEnvVar:       "csiDriverNs",
			},

			wantDefaultNamepsace: objectConfig{configured: true, namespaces: []string{"agentNs"}},

			wantObjectConfig: map[client.Object]objectConfig{
				agentObj:           {configured: true, namespaces: []string{"agentNs"}},
				dashboardObj:       {configured: true, namespaces: []string{"dashboardNs"}},
				genericResourceObj: {configured: true, namespaces: []string{"genericNs"}},
				monitorObj:         {configured: true, namespaces: []string{"monitorNs", "monitorNs2"}},
				sloObj:             {configured: true, namespaces: []string{"nsWithSpace"}},
				profileObj:         {configured: true, namespaces: []string{"profileNs"}},
				podObj:             {configured: true, namespaces: []string{"agentNs"}},
				nodeObj:            {configured: true, namespaces: nil},
				csiDriverObj:       {configured: true, namespaces: []string{"csiDriverNs"}},
				csiDaemonSetObj:    {configured: true, namespaces: []string{"csiDriverNs", "agentNs"}},
			},
		},
		{
			name: "CSIDriver enabled; falls back to WATCH_NAMESPACE when DD_CSIDRIVER_WATCH_NAMESPACE not set",
			watchOptions: WatchOptions{
				DatadogCSIDriverEnabled: true,
			},

			envConfig: map[string]string{
				WatchNamespaceEnvVar: "commonNs",
			},

			wantDefaultNamepsace: objectConfig{configured: true, namespaces: []string{"commonNs"}},

			wantObjectConfig: map[client.Object]objectConfig{
				csiDriverObj:    {configured: true, namespaces: []string{"commonNs"}},
				csiDaemonSetObj: {configured: true, namespaces: []string{"commonNs"}},
			},
		},
		{
			name: "CSIDriver enabled; uses DD_CSIDRIVER_WATCH_NAMESPACE when set",
			watchOptions: WatchOptions{
				DatadogCSIDriverEnabled: true,
			},

			envConfig: map[string]string{
				WatchNamespaceEnvVar:          "commonNs",
				csiDriverWatchNamespaceEnvVar: "csiNs1,csiNs2",
			},

			wantDefaultNamepsace: objectConfig{configured: true, namespaces: []string{"commonNs"}},

			wantObjectConfig: map[client.Object]objectConfig{
				csiDriverObj:    {configured: true, namespaces: []string{"csiNs1", "csiNs2"}},
				csiDaemonSetObj: {configured: true, namespaces: []string{"csiNs1", "csiNs2", "commonNs"}},
			},
		},
		{
			name: "CSIDriver in different namespace than Agent; DaemonSet cached in both",
			watchOptions: WatchOptions{
				DatadogAgentEnabled:     true,
				DatadogCSIDriverEnabled: true,
			},

			envConfig: map[string]string{
				AgentWatchNamespaceEnvVar:     "system",
				csiDriverWatchNamespaceEnvVar: "default",
			},

			wantDefaultNamepsace: objectConfig{configured: true, namespaces: []string{"system"}},

			wantObjectConfig: map[client.Object]objectConfig{
				agentObj:        {configured: true, namespaces: []string{"system"}},
				csiDriverObj:    {configured: true, namespaces: []string{"default"}},
				csiDaemonSetObj: {configured: true, namespaces: []string{"system", "default"}},
			},
		},
		{
			name: "Agent, DAP enabled; Agent, Pod use default config; DAP uses Profile namespace; Node uses nil namespace",

			watchOptions: WatchOptions{
				DatadogAgentEnabled:        true,
				DatadogAgentProfileEnabled: true,
			},

			envConfig: map[string]string{
				WatchNamespaceEnvVar:        "datadog",
				profileWatchNamespaceEnvVar: "profileNs",
			},

			wantDefaultNamepsace: objectConfig{configured: true, namespaces: []string{"datadog"}},
			wantObjectConfig: map[client.Object]objectConfig{
				agentObj:           {configured: true, namespaces: []string{"datadog"}},
				dashboardObj:       {configured: false},
				genericResourceObj: {configured: false},
				monitorObj:         {configured: false},
				sloObj:             {configured: false},
				profileObj:         {configured: true, namespaces: []string{"profileNs"}},
				podObj:             {configured: true, namespaces: []string{"datadog"}},
				nodeObj:            {configured: true, namespaces: nil},
				csiDriverObj:       {configured: false},
			},
		},

		{
			name: "Agent, DAP enabled; Agent, Pod use Agent namespace; DAP uses Profile namespace; Node uses nil namespace",

			watchOptions: WatchOptions{
				DatadogAgentEnabled:        true,
				DatadogAgentProfileEnabled: true,
			},

			envConfig: map[string]string{
				WatchNamespaceEnvVar:        "datadog",
				AgentWatchNamespaceEnvVar:   "agentNs1,agentNs2",
				profileWatchNamespaceEnvVar: "profileNs",
			},

			// Expected
			wantDefaultNamepsace: objectConfig{configured: true, namespaces: []string{"agentNs1", "agentNs2"}},
			wantObjectConfig: map[client.Object]objectConfig{
				agentObj:           {configured: true, namespaces: []string{"agentNs1", "agentNs2"}},
				dashboardObj:       {configured: false},
				genericResourceObj: {configured: false},
				monitorObj:         {configured: false},
				sloObj:             {configured: false},
				profileObj:         {configured: true, namespaces: []string{"profileNs"}},
				podObj:             {configured: true, namespaces: []string{"agentNs1", "agentNs2"}},
				nodeObj:            {configured: true, namespaces: nil},
				csiDriverObj:       {configured: false},
			},
		},
		{
			name: "Only Agent enabled; Monitor enabled without namespace config. Other CRDs, Pods, Nodes not configured",

			watchOptions: WatchOptions{
				DatadogAgentEnabled:   true,
				DatadogMonitorEnabled: true,
			},

			envConfig: map[string]string{
				WatchNamespaceEnvVar:        "datadog",
				AgentWatchNamespaceEnvVar:   "agentNs1,agentNs2",
				profileWatchNamespaceEnvVar: "profileNs",
			},

			// Expected
			wantDefaultNamepsace: objectConfig{configured: true, namespaces: []string{"agentNs1", "agentNs2"}},
			wantObjectConfig: map[client.Object]objectConfig{
				agentObj:           {configured: true, namespaces: []string{"agentNs1", "agentNs2"}},
				dashboardObj:       {configured: false},
				genericResourceObj: {configured: false},
				monitorObj:         {configured: true, namespaces: []string{"datadog"}},
				sloObj:             {configured: false},
				profileObj:         {configured: false},
				podObj:             {configured: false},
				nodeObj:            {configured: false},
				csiDriverObj:       {configured: false},
			},
		},
		{
			name: "DAP disabled, Introspection enabled; Node uses nil namespace; Pods, Profiles are not configured",

			watchOptions: WatchOptions{
				DatadogAgentEnabled:        true,
				DatadogAgentProfileEnabled: false,
				IntrospectionEnabled:       true,
			},

			envConfig: map[string]string{
				WatchNamespaceEnvVar:        "datadog",
				AgentWatchNamespaceEnvVar:   "agentNs1,agentNs2",
				profileWatchNamespaceEnvVar: "profileNs",
			},

			// Expected
			wantDefaultNamepsace: objectConfig{configured: true, namespaces: []string{"agentNs1", "agentNs2"}},
			wantObjectConfig: map[client.Object]objectConfig{
				agentObj:           {configured: true, namespaces: []string{"agentNs1", "agentNs2"}},
				dashboardObj:       {configured: false},
				genericResourceObj: {configured: false},
				monitorObj:         {configured: false},
				sloObj:             {configured: false},
				profileObj:         {configured: false},
				podObj:             {configured: false},
				nodeObj:            {configured: true, namespaces: nil},
				csiDriverObj:       {configured: false},
			},
		},
		{
			name: "Untaint wait-for-CSI; Pod cache merges agent and CSI namespaces and omits label selector",

			watchOptions: WatchOptions{
				UntaintControllerEnabled:          true,
				UntaintControllerWaitForCSIDriver: true,
			},

			envConfig: map[string]string{
				WatchNamespaceEnvVar:          "commonNs",
				AgentWatchNamespaceEnvVar:     "agentNs",
				csiDriverWatchNamespaceEnvVar: "csiNs1,csiNs2",
			},

			wantDefaultNamepsace: objectConfig{configured: true, namespaces: []string{"agentNs"}},
			wantObjectConfig: map[client.Object]objectConfig{
				podObj:       {configured: true, namespaces: []string{"agentNs", "csiNs1", "csiNs2"}, noPodLabel: true},
				nodeObj:      {configured: true, namespaces: nil},
				csiDriverObj: {configured: false},
			},
		},
		{
			name: "Managed Agent installation namespace is included in Agent resource caches",

			watchOptions: WatchOptions{
				DatadogAgentEnabled:               true,
				DatadogAgentProfileEnabled:        true,
				DatadogCSIDriverEnabled:           true,
				ManagedAgentInstallationEnabled:   true,
				ManagedAgentInstallationNamespace: "addonNs",
			},

			envConfig: map[string]string{
				AgentWatchNamespaceEnvVar:     "agentNs",
				profileWatchNamespaceEnvVar:   "profileNs",
				csiDriverWatchNamespaceEnvVar: "csiNs",
			},

			wantDefaultNamepsace: objectConfig{configured: true, namespaces: []string{"agentNs", "addonNs"}},
			wantObjectConfig: map[client.Object]objectConfig{
				agentObj:         {configured: true, namespaces: []string{"agentNs", "addonNs"}},
				agentInternalObj: {configured: true, namespaces: []string{"agentNs", "addonNs"}},
				profileObj:       {configured: true, namespaces: []string{"profileNs", "addonNs"}},
				podObj:           {configured: true, namespaces: []string{"agentNs", "addonNs"}},
				csiDriverObj:     {configured: true, namespaces: []string{"csiNs"}},
				csiDaemonSetObj:  {configured: true, namespaces: []string{"agentNs", "addonNs", "csiNs"}},
			},
		},
	}

	logger := logf.Log.WithName(t.Name())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			for envVar, envVal := range tt.envConfig {
				os.Setenv(envVar, envVal)
			}

			cacheOptions := CacheOptions(logger, tt.watchOptions)

			assert.ElementsMatch(t, tt.wantDefaultNamepsace.namespaces, maps.Keys(cacheOptions.DefaultNamespaces))
			for objKey, wantConfig := range tt.wantObjectConfig {
				verifyResourceNamespace(t, objKey, wantConfig, cacheOptions)
			}
		})
	}
}

func TestIncludeWatchNamespacePreservesClusterWideWatch(t *testing.T) {
	namespaces := map[string]cache.Config{cache.AllNamespaces: {}}

	assert.Equal(t, namespaces, includeWatchNamespace(namespaces, "datadog-agent"))
}

func verifyResourceNamespace(t *testing.T, resource client.Object, wantConfig objectConfig, cacheOptions cache.Options) {
	byObjectOptions, ok := cacheOptions.ByObject[resource]
	assert.Equal(t, wantConfig.configured, ok)
	if wantConfig.configured {
		if wantConfig.namespaces == nil {
			assert.Nil(t, byObjectOptions.Namespaces, "Namespaces should be nil for", reflect.TypeOf(resource).Elem())
		} else {
			assert.ElementsMatch(t, wantConfig.namespaces, maps.Keys(byObjectOptions.Namespaces), "Namespaces don't match for", reflect.TypeOf(resource).Elem())
		}
		if wantConfig.noPodLabel {
			assert.Nil(t, byObjectOptions.Label)
		}
	}
}

func TestCacheConfigStripsManagedFields(t *testing.T) {
	cacheOptions := CacheOptions(logf.Log.WithName(t.Name()), WatchOptions{})
	requireTransform := cacheOptions.DefaultTransform
	if !assert.NotNil(t, requireTransform) {
		return
	}

	obj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "test-manager"}},
	}}
	transformed, err := requireTransform(obj)

	assert.NoError(t, err)
	assert.Same(t, obj, transformed)
	assert.Nil(t, obj.ManagedFields)
}

func TestCacheConfigNodeTransform(t *testing.T) {
	node := func() *corev1.Node {
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1", Labels: map[string]string{"kubernetes.io/os": "linux"}},
			Status:     corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{OSImage: "Talos (v1.13.7)"}},
		}
	}

	transformNode := func(t *testing.T, opts WatchOptions) *corev1.Node {
		t.Helper()
		byObject, ok := CacheOptions(logf.Log.WithName(t.Name()), opts).ByObject[nodeObj]
		if !assert.True(t, ok) || !assert.NotNil(t, byObject.Transform) {
			return nil
		}
		transformed, err := byObject.Transform(node())
		assert.NoError(t, err)
		return transformed.(*corev1.Node)
	}

	// Provider detection identifies Talos from osImage alone, and its node-list
	// fallback is wired whenever this cache exists — for any of the three options
	// below, not just introspection. So every one of them must keep osImage.
	for name, opts := range map[string]WatchOptions{
		"introspection":      {IntrospectionEnabled: true},
		"profiles":           {DatadogAgentProfileEnabled: true},
		"untaint controller": {UntaintControllerEnabled: true},
	} {
		t.Run(name+" keeps Status.NodeInfo.OSImage", func(t *testing.T) {
			got := transformNode(t, opts)
			if got == nil {
				return
			}
			assert.Equal(t, "Talos (v1.13.7)", got.Status.NodeInfo.OSImage)
			assert.Equal(t, kubernetes.TalosProvider, kubernetes.ClusterProviderFromNode(got))
		})
	}
}

// TestCacheConfigPodTransformComponentHealthGating verifies that when the
// wait-for-CSI branch widens the Pod cache to a merged namespace set and drops
// the informer-level label selector, the Transform still only retains the
// memory-heavy status fields (container statuses, phase, etc.) for the managed
// component pods (DCA/CLC), not for every Pod that now passes through it (e.g.
// CSI driver node-server pods).
func TestCacheConfigPodTransformComponentHealthGating(t *testing.T) {
	opts := WatchOptions{
		UntaintControllerEnabled:          true,
		UntaintControllerWaitForCSIDriver: true,
		ComponentHealthEnabled:            true,
	}
	os.Clearenv()
	os.Setenv(AgentWatchNamespaceEnvVar, "agentNs")
	os.Setenv(csiDriverWatchNamespaceEnvVar, "csiNs")

	byObject, ok := CacheOptions(logf.Log.WithName(t.Name()), opts).ByObject[podObj]
	if !assert.True(t, ok) || !assert.NotNil(t, byObject.Transform) {
		return
	}

	podWithStatus := func(component string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{common.AgentDeploymentComponentLabelKey: component},
			},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Name: "container"}},
			},
		}
	}

	dca, err := byObject.Transform(podWithStatus(constants.DefaultClusterAgentResourceSuffix))
	assert.NoError(t, err)
	assert.NotEmpty(t, dca.(*corev1.Pod).Status.ContainerStatuses, "DCA pod should keep container statuses")

	csi, err := byObject.Transform(podWithStatus("datadog-csi-driver-node-server"))
	assert.NoError(t, err)
	assert.Empty(t, csi.(*corev1.Pod).Status.ContainerStatuses, "non-managed-component pod should not keep container statuses")
	assert.Empty(t, csi.(*corev1.Pod).Status.Phase, "non-managed-component pod should not keep phase")
}
