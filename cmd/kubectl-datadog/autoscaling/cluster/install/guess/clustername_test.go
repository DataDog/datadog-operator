package guess

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
)

func TestGetClusterNameFromKubeConfig(t *testing.T) {
	testcases := []struct {
		name        string
		kubeConfig  string
		kubeContext string
		clusterName string
	}{
		{
			name:        "EKS cluster with ARN",
			kubeConfig:  "testdata/kubeconfig-eks.yaml",
			kubeContext: "",
			clusterName: "lenaic-eks",
		},
		{
			name:        "eksctl",
			kubeConfig:  "testdata/kubeconfig-eks.yaml",
			kubeContext: "lenaic.huard@datadoghq.com@lenaic-karpenter-test.us-east-1.eksctl.io",
			clusterName: "lenaic-karpenter-test",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
				&clientcmd.ClientConfigLoadingRules{ExplicitPath: tc.kubeConfig},
				&clientcmd.ConfigOverrides{CurrentContext: tc.kubeContext},
			)
			rawConfig, err := clientConfig.RawConfig()
			require.NoError(t, err)
			assert.Equal(t, tc.clusterName, GetClusterNameFromKubeconfig(t.Context(), rawConfig, tc.kubeContext))
		})
	}
}
