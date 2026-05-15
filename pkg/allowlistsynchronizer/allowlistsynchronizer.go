// Package allowlistsynchronizer contains helpers to manage the
// AllowlistSynchronizer CRD required by GKE Autopilot clusters.
package allowlistsynchronizer

import (
	"context"
	"slices"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	SchemeGroupVersion = schema.GroupVersion{
		Group:   "auto.gke.io",
		Version: "v1",
	}

	SchemeBuilder = runtime.NewSchemeBuilder(func(scheme *runtime.Scheme) error {
		scheme.AddKnownTypes(SchemeGroupVersion, &AllowlistSynchronizer{})
		metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
		return nil
	})
)

var logger = logf.Log.WithName("AllowlistSynchronizer")

// Allowlist paths kept in sync with the Datadog partner exemption YAMLs published in
// the datadog-gke-workload-allowlist repo.
const (
	// allowlistPathV101 exempts the base agent / process-agent / trace-agent containers.
	allowlistPathV101 = "Datadog/datadog/datadog-datadog-daemonset-exemption-v1.0.1.yaml"
	// allowlistPathV105 exempts the otel-agent (ddot-collector) container when OTel
	// feature gates are configured. See OTAGENT-980.
	allowlistPathV105 = "Datadog/datadog/datadog-datadog-daemonset-exemption-v1.0.5.yaml"
)

type AllowlistSynchronizer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec AllowlistSynchronizerSpec `json:"spec"`
}

func (in *AllowlistSynchronizer) DeepCopyObject() runtime.Object {
	out := new(AllowlistSynchronizer)
	*out = *in
	return out
}

type AllowlistSynchronizerSpec struct {
	AllowlistPaths []string `json:"allowlistPaths,omitempty"`
}

// ComputeAllowlistPaths returns the set of Datadog WorkloadAllowlist exemption paths
// that the AllowlistSynchronizer should reference for the given DatadogAgent.
//
// v1.0.5 is included when the OTel collector is enabled so that the otel-agent
// (ddot-collector image) container is permitted by GKE Autopilot Warden. See OTAGENT-980.
func ComputeAllowlistPaths(otelCollectorEnabled bool) []string {
	paths := []string{allowlistPathV101}
	if otelCollectorEnabled {
		paths = append(paths, allowlistPathV105)
	}
	return paths
}

func newAllowlistSynchronizer(paths []string) *AllowlistSynchronizer {
	return &AllowlistSynchronizer{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "allowlistsynchronizers.auto.gke.io",
			Kind:       "AllowlistSynchronizer",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "datadog-synchronizer",
			Annotations: map[string]string{
				"helm.sh/hook":        "pre-install,pre-upgrade",
				"helm.sh/hook-weight": "-1",
			},
		},
		Spec: AllowlistSynchronizerSpec{
			AllowlistPaths: paths,
		},
	}
}

func createAllowlistSynchronizerResource(k8sClient client.Client, paths []string) error {
	return k8sClient.Create(context.TODO(), newAllowlistSynchronizer(paths))
}

// CreateAllowlistSynchronizer creates a GKE AllowlistSynchronizer Custom Resource (auto.gke.io/v1) for the Datadog WorkloadAllowlist if it doesn't exist.
// The AllowlistSynchronizer is needed so that GKE Autopilot can sync the Datadog WorkloadAllowlist to the cluster. See the CRD reference:
// https://cloud.google.com/kubernetes-engine/docs/reference/crds/allowlistsynchronizer
//
// otelCollectorEnabled controls whether the v1.0.5 exemption path (which permits the
// otel-agent / ddot-collector container) is included.
func CreateAllowlistSynchronizer(otelCollectorEnabled bool) {
	cfg, configErr := config.GetConfig()
	if configErr != nil {
		logger.Error(configErr, "failed to load kubeconfig")
		return
	}

	scheme := runtime.NewScheme()
	if SchemeErr := SchemeBuilder.AddToScheme(scheme); SchemeErr != nil {
		logger.Error(SchemeErr, "failed to register AllowlistSynchronizer scheme")
		return
	}

	k8sClient, clietErr := client.New(cfg, client.Options{Scheme: scheme})
	if clietErr != nil {
		logger.Error(clietErr, "failed to create kubernetes client")
		return
	}

	reconcileAllowlistSynchronizer(k8sClient, otelCollectorEnabled)
}

// reconcileAllowlistSynchronizer creates the AllowlistSynchronizer resource if it does
// not already exist; if it exists with stale allowlistPaths (e.g. installed by an older
// operator version that didn't reference v1.0.5), the spec is updated in place so OTel
// collector workloads are admitted by Warden after enabling the feature. Extracted from
// CreateAllowlistSynchronizer so it can be exercised with a fake client.
func reconcileAllowlistSynchronizer(k8sClient client.Client, otelCollectorEnabled bool) {
	desired := ComputeAllowlistPaths(otelCollectorEnabled)

	existing := &AllowlistSynchronizer{}
	getErr := k8sClient.Get(context.TODO(), client.ObjectKey{Name: "datadog-synchronizer"}, existing)
	switch {
	case getErr == nil:
		if slices.Equal(existing.Spec.AllowlistPaths, desired) {
			return
		}
		existing.Spec.AllowlistPaths = desired
		if updateErr := k8sClient.Update(context.TODO(), existing); updateErr != nil {
			logger.Error(updateErr, "failed to update AllowlistSynchronizer resource")
			return
		}
		logger.Info("Successfully updated AllowlistSynchronizer allowlistPaths")
		return
	case !apierrors.IsNotFound(getErr):
		logger.Error(getErr, "failed to check existing AllowlistSynchronizer resource")
		return
	}

	if err := createAllowlistSynchronizerResource(k8sClient, desired); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return
		}
		logger.Error(err, "failed to create AllowlistSynchronizer resource")
		return
	}

	logger.Info("Successfully created AllowlistSynchronizer")
}
