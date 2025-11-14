package guess

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// IsAwsAuthConfigMapPresent checks if the aws-auth ConfigMap exists in the kube-system namespace
func IsAwsAuthConfigMapPresent(ctx context.Context, clientset *kubernetes.Clientset) (bool, error) {
	if _, err := clientset.CoreV1().ConfigMaps("kube-system").Get(ctx, "aws-auth", metav1.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get aws-auth ConfigMap: %w", err)
	}

	return true, nil
}
