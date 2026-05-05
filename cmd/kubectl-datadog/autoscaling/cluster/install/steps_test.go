package install

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/install/guess"
)

func TestKarpenterHelmValues(t *testing.T) {
	t.Run("existing-nodes mode carries Datadog ownership labels and no IRSA annotation", func(t *testing.T) {
		values := karpenterHelmValues("my-cluster", InstallModeExistingNodes, "")

		labels, ok := values["additionalLabels"].(map[string]any)
		require.True(t, ok, "additionalLabels must be a map")
		assert.Equal(t, guess.InstalledByValue, labels[guess.InstalledByLabel],
			"installed-by sentinel must match what FindKarpenterInstallation looks for")
		assert.Contains(t, labels, guess.InstallerVersionLabel)

		settings, ok := values["settings"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "my-cluster", settings["clusterName"])
		assert.Equal(t, "my-cluster", settings["interruptionQueue"])

		assert.NotContains(t, values, "serviceAccount",
			"existing-nodes mode must not annotate the ServiceAccount with an IRSA role")
	})

	t.Run("fargate mode annotates the ServiceAccount with the IRSA role ARN", func(t *testing.T) {
		const arn = "arn:aws:iam::123456789012:role/dd-karpenter"
		values := karpenterHelmValues("my-cluster", InstallModeFargate, arn)

		serviceAccount, ok := values["serviceAccount"].(map[string]any)
		require.True(t, ok, "fargate mode must populate serviceAccount values")
		annotations, ok := serviceAccount["annotations"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, arn, annotations["eks.amazonaws.com/role-arn"])
	})
}

func TestDisplayForeignKarpenterMessage(t *testing.T) {
	// browser.OpenURL spawns xdg-open with non-*os.File writers, which makes
	// `cmd.Wait` hang on the pipe-copy goroutine until xdg-open's descendants
	// all close the write side. Empty PATH makes the LookPath probe fail and
	// browser.OpenURL returns ErrNotFound without spawning anything.
	t.Setenv("PATH", "")

	out := &bytes.Buffer{}
	cmd := &cobra.Command{}
	cmd.SetOut(out)
	cmd.SetErr(&bytes.Buffer{})

	foreign := &guess.KarpenterInstallation{Namespace: "karpenter", Name: "karpenter"}
	err := displayForeignKarpenterMessage(cmd, "my-cluster", foreign)
	require.NoError(t, err, "foreign Karpenter is a successful no-op, not an error")

	rendered := out.String()
	assert.Contains(t, rendered, "Karpenter is already installed on cluster my-cluster")
	assert.Contains(t, rendered, "Deployment karpenter/karpenter.",
		"the message must surface the foreign install's namespace/name so the user can locate it")
	assert.Contains(t, rendered, "kubectl-datadog has nothing to install.")
	assert.Contains(t, rendered, "Autoscaling settings page")
	assert.Contains(t, rendered, "kube_cluster_name%3Amy-cluster",
		"the linked URL must point at the cluster's autoscaling settings")
}
