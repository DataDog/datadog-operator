package helm

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
)

// IsHelmMigration returns true if the object is marked for Helm migration
func IsHelmMigration(obj metav1.Object) bool {
	val, ok := obj.GetAnnotations()[apicommon.HelmMigrationAnnotationKey]
	return ok && strings.EqualFold(val, "true")
}
