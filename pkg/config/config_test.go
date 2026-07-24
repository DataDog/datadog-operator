package config

import (
	"os"
	"reflect"
	"testing"

	"golang.org/x/exp/maps"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
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
				podObj:          {configured: true, namespaces: []string{"system"}},
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
			name: "Only Agent enabled; Monitor enabled without namespace config. Agent Pods are configured; other CRDs and Nodes are not",

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
				podObj:             {configured: true, namespaces: []string{"agentNs1", "agentNs2"}},
				nodeObj:            {configured: false},
				csiDriverObj:       {configured: false},
			},
		},
		{
			name: "DAP disabled, Introspection enabled; Node uses nil namespace; Agent Pods are configured, Profiles are not",

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
				podObj:             {configured: true, namespaces: []string{"agentNs1", "agentNs2"}},
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

func TestAgentPodCacheTransformPreservesPreparedRolloutStatus(t *testing.T) {
	t.Setenv(AgentWatchNamespaceEnvVar, "datadog-agent")
	options := CacheOptions(logf.Log.WithName(t.Name()), WatchOptions{DatadogAgentEnabled: true})
	podConfig, found := options.ByObject[podObj]
	require.True(t, found)
	require.NotNil(t, podConfig.Transform)

	started := true
	input := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "datadog-agent"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			InitContainerStatuses: []corev1.ContainerStatus{{
				Name:  "init-config",
				State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0}},
			}},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:    "agent",
				Started: &started,
				State:   corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			}},
		},
	}
	transformedObject, err := podConfig.Transform(input)
	require.NoError(t, err)
	transformed := transformedObject.(*corev1.Pod)
	assert.Equal(t, corev1.PodRunning, transformed.Status.Phase)
	require.Len(t, transformed.Status.InitContainerStatuses, 1)
	require.Len(t, transformed.Status.ContainerStatuses, 1)
	assert.True(t, *transformed.Status.ContainerStatuses[0].Started)
}
