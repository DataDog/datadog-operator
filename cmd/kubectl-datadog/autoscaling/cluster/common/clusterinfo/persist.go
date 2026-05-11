package clusterinfo

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonk8s "github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/k8s"
)

// Persist marshals info as YAML and writes it to a ConfigMap in namespace.
// The ConfigMap is created or updated idempotently.
func Persist(ctx context.Context, cli client.Client, namespace string, info *ClusterInfo) error {
	payload, err := yaml.Marshal(info)
	if err != nil {
		return fmt.Errorf("failed to marshal cluster info: %w", err)
	}

	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "kubectl-datadog",
			},
		},
		Data: map[string]string{
			ConfigMapDataKey: string(payload),
		},
	}

	return commonk8s.CreateOrUpdate(ctx, cli, cm)
}
