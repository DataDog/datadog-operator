package config

import (
	"os"
	"testing"

	"golang.org/x/exp/maps"
	corev1 "k8s.io/api/core/v1"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_CacheConfig(t *testing.T) {

	type config struct {
		namespaceEnv string
		crdEnabled   bool
	}

	tests := []struct {
		name string

		defaultNamespaceEnv string
		agentConfig         config
		monitorConfig       config
		sloConfig           config
		profileConfig       config

		defaultNamespaces []string
		agentNamespaces   []string
		monitorNamespaces []string
		sloNamespaces     []string
		profileNamespaces []string
		podNamespaces     []string
		nodeNamespaces    []string
	}{
		{
			name:                "all envs non empty, all enabled",
			defaultNamespaceEnv: "datadog",
			agentConfig: config{
				namespaceEnv: "agentNs",
				crdEnabled:   true,
			},
			monitorConfig: config{
				namespaceEnv: "monitorNs, monitorNs2",
				crdEnabled:   true,
			},
			sloConfig: config{
				namespaceEnv: "  nsWithSpace ",
				crdEnabled:   true,
			},
			profileConfig: config{
				namespaceEnv: "profileNs1",
				crdEnabled:   true,
			},
			// Expected
			defaultNamespaces: []string{"datadog"},
			agentNamespaces:   []string{"agentNs"},
			monitorNamespaces: []string{"monitorNs", "monitorNs2"},
			sloNamespaces:     []string{"nsWithSpace"},
			profileNamespaces: []string{"profileNs"},
			podNamespaces:     []string{"agentNs"},
			nodeNamespaces:    nil,
		},

		{
			name:                "Agent uses default",
			defaultNamespaceEnv: "datadog",
			agentConfig: config{
				crdEnabled: true,
			},
			profileConfig: config{
				namespaceEnv: "profileNs",
				crdEnabled:   true,
			},
			// Expected
			defaultNamespaces: []string{"datadog"},
			agentNamespaces:   []string{"datadog"},
			monitorNamespaces: nil,
			sloNamespaces:     nil,
			profileNamespaces: []string{"profileNs"},
			podNamespaces:     []string{"datadog"},
			nodeNamespaces:    nil,
		},

		{
			name:                "Profile enabled, Pod uses Agent namespace",
			defaultNamespaceEnv: "datadog",
			agentConfig: config{
				namespaceEnv: "agentNs1,agentNs2",
				crdEnabled:   true,
			},
			profileConfig: config{
				namespaceEnv: "profileNs",
				crdEnabled:   true,
			},
			// Expected
			defaultNamespaces: []string{"datadog"},
			agentNamespaces:   []string{"agentNs1", "agentNs2"},
			monitorNamespaces: nil,
			sloNamespaces:     nil,
			profileNamespaces: []string{"profileNs"},
			podNamespaces:     []string{"agentNs1", "agentNs2"},
			nodeNamespaces:    nil,
		},
	}

	logger := logf.Log.WithName(t.Name())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv(watchNamespaceEnvVar, tt.defaultNamespaceEnv)
			os.Setenv(agentWatchNamespaceEnvVar, tt.agentConfig.namespaceEnv)
			os.Setenv(monitorWatchNamespaceEnvVar, tt.monitorConfig.namespaceEnv)
			os.Setenv(sloWatchNamespaceEnvVar, tt.sloConfig.namespaceEnv)
			os.Setenv(profileWatchNamespaceEnvVar, tt.profileConfig.namespaceEnv)

			cacheOptions := CacheOptions(logger, tt.agentConfig.crdEnabled, tt.sloConfig.crdEnabled, tt.profileConfig.crdEnabled, tt.monitorConfig.crdEnabled)

			assert.Equal(t, tt.defaultNamespaces, maps.Keys(cacheOptions.DefaultNamespaces))

			verifyResourceNamespace(t, tt.agentNamespaces, &datadoghqv2alpha1.DatadogAgent{}, cacheOptions)
			verifyResourceNamespace(t, tt.monitorNamespaces, &datadoghqv1alpha1.DatadogMonitor{}, cacheOptions)
			verifyResourceNamespace(t, tt.sloNamespaces, &datadoghqv1alpha1.DatadogSLO{}, cacheOptions)
			verifyResourceNamespace(t, tt.profileNamespaces, &datadoghqv1alpha1.DatadogSLO{}, cacheOptions)
			verifyResourceNamespace(t, tt.podNamespaces, &corev1.Pod{}, cacheOptions)
			verifyResourceNamespace(t, tt.nodeNamespaces, &corev1.Node{}, cacheOptions)
		})
	}
}

func verifyResourceNamespace(t *testing.T, expectedNamespaces []string, resource client.Object, cacheOptions cache.Options) {
	for k, v := range cacheOptions.ByObject {
		if k.GetObjectKind() == resource.GetObjectKind() {
			assert.Equal(t, expectedNamespaces, maps.Keys(v.Namespaces))
		}
	}
}
