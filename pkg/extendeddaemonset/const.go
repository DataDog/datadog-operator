package extendeddaemonset

import "k8s.io/apimachinery/pkg/runtime/schema"

// GroupVersion is group version used to register these objects
var GroupVersion = schema.GroupVersion{Group: "datadoghq.com", Version: "v1alpha1"}
