package eksautomode

import (
	"fmt"
	"slices"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
)

// IsEnabled checks if EKS auto-mode is active on the cluster by looking for
// the nodeclasses resource in the eks.amazonaws.com/v1 API group.
func IsEnabled(discoveryClient discovery.DiscoveryInterface) (bool, error) {
	resources, err := discoveryClient.ServerResourcesForGroupVersion("eks.amazonaws.com/v1")
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to query eks.amazonaws.com/v1 API group: %w", err)
	}

	return slices.ContainsFunc(resources.APIResources, func(r metav1.APIResource) bool {
		return r.Name == "nodeclasses"
	}), nil
}
