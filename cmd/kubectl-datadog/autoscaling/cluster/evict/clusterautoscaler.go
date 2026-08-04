package evict

import (
	"context"
	"fmt"
	"log"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clusterinfo"
)

// Poll cadence for confirming the cluster-autoscaler Deployment has actually
// reached 0 replicas. Declared as vars (not consts) so tests can shrink them.
var (
	caScaleDownPollInterval = 2 * time.Second
	caScaleDownPollTimeout  = 2 * time.Minute
)

func scaleDownClusterAutoscaler(ctx context.Context, clientset kubernetes.Interface, ca clusterinfo.ClusterAutoscaler, dryRun bool) error {
	if !ca.Present {
		return nil
	}
	if dryRun {
		log.Printf("[dry-run] would scale Deployment %s/%s to 0 replicas", ca.Namespace, ca.Name)
		return nil
	}

	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		scale, err := clientset.AppsV1().Deployments(ca.Namespace).GetScale(ctx, ca.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get Deployment scale %s/%s: %w", ca.Namespace, ca.Name, err)
		}
		if scale.Spec.Replicas == 0 {
			return nil
		}
		scale.Spec.Replicas = 0
		if _, err = clientset.AppsV1().Deployments(ca.Namespace).UpdateScale(ctx, ca.Name, scale, metav1.UpdateOptions{}); err != nil {
			return err
		}
		log.Printf("Requested scale-down of Deployment %s/%s to 0 replicas.", ca.Namespace, ca.Name)
		return nil
	}); err != nil {
		return err
	}

	if err := wait.PollUntilContextTimeout(ctx, caScaleDownPollInterval, caScaleDownPollTimeout, true, func(ctx context.Context) (bool, error) {
		if scale, err := clientset.AppsV1().Deployments(ca.Namespace).GetScale(ctx, ca.Name, metav1.GetOptions{}); err != nil {
			return false, fmt.Errorf("failed to get Deployment scale %s/%s: %w", ca.Namespace, ca.Name, err)
		} else {
			return scale.Status.Replicas == 0, nil
		}
	}); err != nil {
		return fmt.Errorf("cluster-autoscaler Deployment %s/%s did not reach 0 replicas: %w", ca.Namespace, ca.Name, err)
	}
	log.Printf("Cluster-autoscaler Deployment %s/%s is now at 0 replicas.", ca.Namespace, ca.Name)
	return nil
}
