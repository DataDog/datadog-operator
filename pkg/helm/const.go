package helm

import (
	"strings"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ResourcePolicyAnnotationKey is the annotation key used to set the resource policy for Helm resources
	ResourcePolicyAnnotationKey = "helm.sh/resource-policy"
	// ResourcePolicyKeep is the value for the resource policy annotation to prevent deletion
	ResourcePolicyKeep = "keep"
)

// IsHelmMigration returns true if the object is marked for Helm migration
func IsHelmMigration(obj metav1.Object) bool {
	val, ok := obj.GetAnnotations()[apicommon.HelmMigrationAnnotationKey]
	return ok && strings.EqualFold(val, "true")
}
