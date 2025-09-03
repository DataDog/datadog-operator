package guess

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetClusterNameFromKubeConfig(t *testing.T) {
	testcases := []struct {
		name        string
		kubeconfig  string
		clusterName string
	}{
		{
			name:        "EKS cluster with ARN",
			kubeconfig:  "testdata/kubeconfig-eks.yaml",
			clusterName: "lenaic-eks",
		},
		{
			name:        "eksctl",
			kubeconfig:  "testdata/kubeconfig-eksctl.yaml",
			clusterName: "lenaic-karpenter-test",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, os.Setenv("KUBECONFIG", tc.kubeconfig))
			assert.Equal(t, tc.clusterName, GetClusterNameFromKubeconfig(context.Background()))
		})
	}
}
