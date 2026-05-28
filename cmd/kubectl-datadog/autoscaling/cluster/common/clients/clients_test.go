package clients

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestGetAccountIDFromKubeconfig(t *testing.T) {
	for _, tt := range []struct {
		name          string
		kubeconfig    string
		context       string
		wantAccountID string
	}{
		{
			name: "EKS ARN context extracts account ID",
			kubeconfig: `
apiVersion: v1
kind: Config
current-context: eks-context
contexts:
- name: eks-context
  context:
    cluster: arn:aws:eks:us-east-1:123456789012:cluster/my-cluster
    user: eks-user
clusters:
- name: arn:aws:eks:us-east-1:123456789012:cluster/my-cluster
  cluster:
    server: https://example.eks.amazonaws.com
users:
- name: eks-user
  user: {}
`,
			wantAccountID: "123456789012",
		},
		{
			name: "GovCloud ARN context extracts account ID",
			kubeconfig: `
apiVersion: v1
kind: Config
current-context: gov-context
contexts:
- name: gov-context
  context:
    cluster: arn:aws-us-gov:eks:us-gov-west-1:987654321098:cluster/gov-cluster
    user: gov-user
clusters:
- name: arn:aws-us-gov:eks:us-gov-west-1:987654321098:cluster/gov-cluster
  cluster:
    server: https://example.eks.amazonaws.com
users:
- name: gov-user
  user: {}
`,
			wantAccountID: "987654321098",
		},
		{
			name: "plain cluster name returns empty",
			kubeconfig: `
apiVersion: v1
kind: Config
current-context: plain-context
contexts:
- name: plain-context
  context:
    cluster: my-cluster
    user: my-user
clusters:
- name: my-cluster
  cluster:
    server: https://example.eks.amazonaws.com
users:
- name: my-user
  user: {}
`,
		},
		{
			name: "eksctl format returns empty",
			kubeconfig: `
apiVersion: v1
kind: Config
current-context: eksctl-context
contexts:
- name: eksctl-context
  context:
    cluster: my-cluster.us-east-1.eksctl.io
    user: eksctl-user
clusters:
- name: my-cluster.us-east-1.eksctl.io
  cluster:
    server: https://example.eks.amazonaws.com
users:
- name: eksctl-user
  user: {}
`,
		},
		{
			name: "explicit context override",
			kubeconfig: `
apiVersion: v1
kind: Config
current-context: default-context
contexts:
- name: default-context
  context:
    cluster: plain-cluster
    user: user1
- name: eks-context
  context:
    cluster: arn:aws:eks:eu-west-1:111222333444:cluster/prod
    user: user2
clusters:
- name: plain-cluster
  cluster:
    server: https://example1.eks.amazonaws.com
- name: arn:aws:eks:eu-west-1:111222333444:cluster/prod
  cluster:
    server: https://example2.eks.amazonaws.com
users:
- name: user1
  user: {}
- name: user2
  user: {}
`,
			context:       "eks-context",
			wantAccountID: "111222333444",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Write kubeconfig to a temp file
			dir := t.TempDir()
			kubeconfigPath := filepath.Join(dir, "kubeconfig")
			require.NoError(t, os.WriteFile(kubeconfigPath, []byte(tt.kubeconfig), 0600))

			flags := genericclioptions.NewConfigFlags(false)
			flags.KubeConfig = &kubeconfigPath
			if tt.context != "" {
				flags.Context = &tt.context
			}

			got, err := getAccountIDFromKubeconfig(flags)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantAccountID, got)
		})
	}
}

func TestGetClusterARNFromKubeconfig(t *testing.T) {
	for _, tt := range []struct {
		name       string
		kubeconfig string
		context    string
		wantOK     bool
		wantRegion string
	}{
		{
			name: "EKS ARN context extracts region",
			kubeconfig: `
apiVersion: v1
kind: Config
current-context: eks-context
contexts:
- name: eks-context
  context:
    cluster: arn:aws:eks:us-east-2:123456789012:cluster/my-cluster
    user: eks-user
clusters:
- name: arn:aws:eks:us-east-2:123456789012:cluster/my-cluster
  cluster:
    server: https://example.eks.amazonaws.com
users:
- name: eks-user
  user: {}
`,
			wantOK:     true,
			wantRegion: "us-east-2",
		},
		{
			name: "GovCloud ARN context extracts region",
			kubeconfig: `
apiVersion: v1
kind: Config
current-context: gov-context
contexts:
- name: gov-context
  context:
    cluster: arn:aws-us-gov:eks:us-gov-west-1:987654321098:cluster/gov-cluster
    user: gov-user
clusters:
- name: arn:aws-us-gov:eks:us-gov-west-1:987654321098:cluster/gov-cluster
  cluster:
    server: https://example.eks.amazonaws.com
users:
- name: gov-user
  user: {}
`,
			wantOK:     true,
			wantRegion: "us-gov-west-1",
		},
		{
			name: "plain cluster name is not an ARN",
			kubeconfig: `
apiVersion: v1
kind: Config
current-context: plain-context
contexts:
- name: plain-context
  context:
    cluster: my-cluster
    user: my-user
clusters:
- name: my-cluster
  cluster:
    server: https://example.eks.amazonaws.com
users:
- name: my-user
  user: {}
`,
			wantOK: false,
		},
		{
			name: "eksctl FQDN is not an ARN",
			kubeconfig: `
apiVersion: v1
kind: Config
current-context: eksctl-context
contexts:
- name: eksctl-context
  context:
    cluster: my-cluster.us-east-1.eksctl.io
    user: eksctl-user
clusters:
- name: my-cluster.us-east-1.eksctl.io
  cluster:
    server: https://example.eks.amazonaws.com
users:
- name: eksctl-user
  user: {}
`,
			wantOK: false,
		},
		{
			name: "non-EKS ARN is rejected",
			kubeconfig: `
apiVersion: v1
kind: Config
current-context: iam-context
contexts:
- name: iam-context
  context:
    cluster: arn:aws:iam::123456789012:role/some-role
    user: iam-user
clusters:
- name: arn:aws:iam::123456789012:role/some-role
  cluster:
    server: https://example.eks.amazonaws.com
users:
- name: iam-user
  user: {}
`,
			wantOK: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			kubeconfigPath := filepath.Join(dir, "kubeconfig")
			require.NoError(t, os.WriteFile(kubeconfigPath, []byte(tt.kubeconfig), 0600))

			flags := genericclioptions.NewConfigFlags(false)
			flags.KubeConfig = &kubeconfigPath
			if tt.context != "" {
				flags.Context = &tt.context
			}

			parsed, ok, err := getClusterARNFromKubeconfig(flags)
			require.NoError(t, err)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantRegion, parsed.Region)
		})
	}
}

func TestResolveRegion(t *testing.T) {
	for _, tt := range []struct {
		name         string
		configRegion string
		kubeRegion   string
		clusterName  string
		wantRegion   string
		wantMismatch bool
		wantErr      bool
	}{
		{
			name:         "both empty is an error",
			configRegion: "",
			kubeRegion:   "",
			wantErr:      true,
		},
		{
			name:         "derives from kubeconfig when config is empty",
			configRegion: "",
			kubeRegion:   "us-east-2",
			wantRegion:   "us-east-2",
		},
		{
			name:         "keeps configured region when kubeconfig is unknown",
			configRegion: "eu-west-3",
			kubeRegion:   "",
			wantRegion:   "eu-west-3",
		},
		{
			name:         "keeps configured region when they match",
			configRegion: "us-east-2",
			kubeRegion:   "us-east-2",
			wantRegion:   "us-east-2",
		},
		{
			name:         "mismatch is an error",
			configRegion: "us-west-2",
			kubeRegion:   "us-east-2",
			clusterName:  "my-cluster",
			wantMismatch: true,
			wantErr:      true,
		},
		{
			name:         "GovCloud mismatch is an error",
			configRegion: "us-gov-east-1",
			kubeRegion:   "us-gov-west-1",
			clusterName:  "gov-cluster",
			wantMismatch: true,
			wantErr:      true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveRegion(tt.configRegion, tt.kubeRegion, tt.clusterName)
			if !tt.wantErr {
				require.NoError(t, err)
				assert.Equal(t, tt.wantRegion, got)
				return
			}

			require.Error(t, err)
			var mismatch *RegionMismatchError
			if tt.wantMismatch {
				require.ErrorAs(t, err, &mismatch)
				assert.Equal(t, tt.configRegion, mismatch.ConfigRegion)
				assert.Equal(t, tt.kubeRegion, mismatch.ClusterRegion)
				assert.Equal(t, tt.clusterName, mismatch.ClusterName)
			} else {
				assert.False(t, errors.As(err, &mismatch))
			}
		})
	}
}
