package apply

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/karpenter"
	"github.com/DataDog/datadog-operator/pkg/plugin/common"
)

// assertKarpenterCheckConfig parses the JSON body of a Datadog Autodiscovery
// annotation and asserts it declares the `karpenter` OpenMetrics check on
// port 8080. Used to keep the assertion robust to whitespace/formatting
// changes in the embedded JSON literal.
func assertKarpenterCheckConfig(t *testing.T, raw string) {
	t.Helper()
	var parsed map[string]struct {
		InitConfig map[string]any   `json:"init_config"`
		Instances  []map[string]any `json:"instances"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &parsed), "annotation body must be valid JSON")
	check, ok := parsed["karpenter"]
	require.True(t, ok, "annotation must declare a `karpenter` check")
	require.Len(t, check.Instances, 1, "the `karpenter` check must have exactly one instance")
	assert.Equal(t, "http://%%host%%:8080/metrics", check.Instances[0]["openmetrics_endpoint"])
}

func TestKarpenterHelmValues(t *testing.T) {
	t.Run("existing-nodes mode carries Datadog ownership labels and no IRSA annotation", func(t *testing.T) {
		values := karpenterHelmValues("my-cluster", InstallModeExistingNodes, "")

		labels, ok := values["additionalLabels"].(map[string]any)
		require.True(t, ok, "additionalLabels must be a map")
		assert.Equal(t, karpenter.InstalledByValue, labels[karpenter.InstalledByLabel],
			"installed-by sentinel must match what karpenter.FindInstallation looks for")
		assert.Contains(t, labels, karpenter.InstallerVersionLabel)

		settings, ok := values["settings"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "my-cluster", settings["clusterName"])
		assert.Equal(t, "my-cluster", settings["interruptionQueue"])

		assert.NotContains(t, values, "serviceAccount",
			"existing-nodes mode must not annotate the ServiceAccount with an IRSA role")
	})

	t.Run("existing-nodes mode wires the Karpenter check as a pod-level annotation", func(t *testing.T) {
		values := karpenterHelmValues("my-cluster", InstallModeExistingNodes, "")

		podAnnotations, ok := values["podAnnotations"].(map[string]any)
		require.True(t, ok, "existing-nodes mode must set podAnnotations for the colocated node agent")
		raw, ok := podAnnotations[common.ADPrefix+"controller.checks"].(string)
		require.True(t, ok, "pod-level Autodiscovery annotation must be present")
		assertKarpenterCheckConfig(t, raw)

		assert.NotContains(t, values, "service",
			"existing-nodes mode must not configure an endpoint check on the Service")
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

	t.Run("fargate mode wires the Karpenter check as a Service endpoint check", func(t *testing.T) {
		values := karpenterHelmValues("my-cluster", InstallModeFargate, "arn:aws:iam::123456789012:role/dd-karpenter")

		service, ok := values["service"].(map[string]any)
		require.True(t, ok, "fargate mode must set service.annotations for the cluster check runner")
		annotations, ok := service["annotations"].(map[string]any)
		require.True(t, ok)
		raw, ok := annotations[common.ADPrefix+"endpoints.checks"].(string)
		require.True(t, ok, "Service-level endpoint-check annotation must be present")
		assertKarpenterCheckConfig(t, raw)

		assert.Equal(t, "ip", annotations[common.ADPrefix+"endpoints.resolve"],
			"`resolve: ip` is required to force the cluster agent to dispatch the check to the cluster check runner instead of the (non-existent) node agent on the Fargate node")

		assert.NotContains(t, values, "podAnnotations",
			"fargate mode must not set the pod-level annotation (no node agent on Fargate)")
	})
}

func TestDisplayForeignKarpenterMessage(t *testing.T) {
	// browser.OpenURL spawns xdg-open with non-*os.File writers, which makes
	// `cmd.Wait` hang on the pipe-copy goroutine until xdg-open's descendants
	// all close the write side. Empty PATH makes the LookPath probe fail and
	// browser.OpenURL returns ErrNotFound without spawning anything.
	t.Setenv("PATH", "")

	out := &bytes.Buffer{}
	streams := genericclioptions.IOStreams{Out: out, ErrOut: &bytes.Buffer{}}

	foreign := &karpenter.Installation{Namespace: "karpenter", Name: "karpenter"}
	err := displayForeignKarpenterMessage(streams, "my-cluster", foreign)
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
