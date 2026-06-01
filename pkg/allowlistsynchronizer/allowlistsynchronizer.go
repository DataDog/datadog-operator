// Package allowlistsynchronizer contains helpers to manage the
// AllowlistSynchronizer CRD required by GKE Autopilot clusters.
package allowlistsynchronizer

import (
	"context"
	"fmt"
	"regexp"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// DefaultWorkloadAllowlistVersion is the default version of the Datadog
// daemonset WorkloadAllowlist. v1.0.5 includes the system-probe / NPM
// exemptions required by the NPM feature on GKE Autopilot.
const DefaultWorkloadAllowlistVersion = "v1.0.5"

// DefaultCSIWorkloadAllowlistVersion is the default version of the Datadog
// CSI driver WorkloadAllowlist. v1.1.0 includes the storage-dir hostPath and
// DD_APM_ENABLED env var exemptions required for SSI on GKE Autopilot >= 1.32.1-gke.1729000.
const DefaultCSIWorkloadAllowlistVersion = "v1.1.0"

// CSIMatchingAllowlistLabel is the label value for cloud.google.com/matching-allowlist
// that must be set on the CSI driver DaemonSet to match the v1.1.0 WorkloadAllowlist.
const CSIMatchingAllowlistLabel = "datadog-datadog-csi-driver-daemonset-exemption-v1.1.0"

const allowlistSynchronizerFieldOwner = "datadog-operator-allowlist-synchronizer"

var workloadAllowlistVersionRegexp = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

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

// resolveWorkloadAllowlistVersion returns the requested allowlist version if it
// is non-empty and well-formed, otherwise it falls back to
// DefaultWorkloadAllowlistVersion (logging the malformed input).
func resolveWorkloadAllowlistVersion(version string) string {
	if version == "" {
		return DefaultWorkloadAllowlistVersion
	}
	if !workloadAllowlistVersionRegexp.MatchString(version) {
		logger.Info("Ignoring malformed WorkloadAllowlist version override, falling back to default",
			"requested", version, "default", DefaultWorkloadAllowlistVersion)
		return DefaultWorkloadAllowlistVersion
	}
	return version
}

func applyAllowlistSynchronizerResource(k8sClient client.Client, version, partOfLabel string) error {
	obj := &AllowlistSynchronizer{
		TypeMeta: metav1.TypeMeta{
			APIVersion: SchemeGroupVersion.String(),
			Kind:       "AllowlistSynchronizer",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "datadog-synchronizer",
			Labels: map[string]string{
				"app.kubernetes.io/created-by":           "datadog-operator",
				kubernetes.AppKubernetesManageByLabelKey: "datadog-operator",
				kubernetes.AppKubernetesNameLabelKey:     "datadog-allowlist-synchronizer",
				kubernetes.AppKubernetesPartOfLabelKey:   partOfLabel,
			},
		},
		Spec: AllowlistSynchronizerSpec{
			AllowlistPaths: []string{
				fmt.Sprintf("Datadog/datadog/datadog-datadog-daemonset-exemption-%s.yaml", version),
			},
		},
	}

	return k8sClient.Patch(
		context.TODO(),
		obj,
		client.Apply,
		client.FieldOwner(allowlistSynchronizerFieldOwner),
		client.ForceOwnership,
	)
}

// CreateAllowlistSynchronizer creates a GKE AllowlistSynchronizer Custom Resource (auto.gke.io/v1) for the Datadog WorkloadAllowlist if it doesn't exist.
// The AllowlistSynchronizer is needed so that GKE Autopilot can sync the Datadog WorkloadAllowlist to the cluster. See the CRD reference:
// https://cloud.google.com/kubernetes-engine/docs/reference/crds/allowlistsynchronizer
//
// version selects the WorkloadAllowlist YAML to point at. Pass an empty string
// to use DefaultWorkloadAllowlistVersion. Malformed versions also fall back to
// the default.
func CreateAllowlistSynchronizer(version, partOfLabel string) {
	resolvedVersion := resolveWorkloadAllowlistVersion(version)

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

	if err := applyAllowlistSynchronizerResource(k8sClient, resolvedVersion, partOfLabel); err != nil {
		logger.Error(err, "failed to apply AllowlistSynchronizer resource")
		return
	}

	logger.V(1).Info("Successfully applied AllowlistSynchronizer", "version", resolvedVersion)
}

// resolveCSIWorkloadAllowlistVersion returns the requested CSI allowlist version if it
// is non-empty and well-formed, otherwise it falls back to
// DefaultCSIWorkloadAllowlistVersion (logging the malformed input).
func resolveCSIWorkloadAllowlistVersion(version string) string {
	if version == "" {
		return DefaultCSIWorkloadAllowlistVersion
	}
	if !workloadAllowlistVersionRegexp.MatchString(version) {
		logger.Info("Ignoring malformed CSI WorkloadAllowlist version override, falling back to default",
			"requested", version, "default", DefaultCSIWorkloadAllowlistVersion)
		return DefaultCSIWorkloadAllowlistVersion
	}
	return version
}

func applyCSIAllowlistSynchronizerResource(k8sClient client.Client, version, partOfLabel string) error {
	obj := &AllowlistSynchronizer{
		TypeMeta: metav1.TypeMeta{
			APIVersion: SchemeGroupVersion.String(),
			Kind:       "AllowlistSynchronizer",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "datadog-csi-synchronizer",
			Labels: map[string]string{
				"app.kubernetes.io/created-by":           "datadog-operator",
				kubernetes.AppKubernetesManageByLabelKey: "datadog-operator",
				kubernetes.AppKubernetesNameLabelKey:     "datadog-csi-allowlist-synchronizer",
				kubernetes.AppKubernetesPartOfLabelKey:   partOfLabel,
			},
		},
		Spec: AllowlistSynchronizerSpec{
			AllowlistPaths: []string{
				fmt.Sprintf("Datadog/datadog-csi-driver/datadog-datadog-csi-driver-daemonset-exemption-%s.yaml", version),
			},
		},
	}

	return k8sClient.Patch(
		context.TODO(),
		obj,
		client.Apply,
		client.FieldOwner(allowlistSynchronizerFieldOwner),
		client.ForceOwnership,
	)
}

// CreateCSIAllowlistSynchronizer creates a GKE AllowlistSynchronizer Custom Resource (auto.gke.io/v1)
// for the Datadog CSI driver WorkloadAllowlist if it doesn't exist.
// The AllowlistSynchronizer is needed so that GKE Autopilot can sync the Datadog CSI driver
// WorkloadAllowlist to the cluster. See the CRD reference:
// https://cloud.google.com/kubernetes-engine/docs/reference/crds/allowlistsynchronizer
//
// version selects the WorkloadAllowlist YAML to point at. Pass an empty string
// to use DefaultCSIWorkloadAllowlistVersion. Malformed versions also fall back to
// the default.
func CreateCSIAllowlistSynchronizer(version, partOfLabel string) {
	resolvedVersion := resolveCSIWorkloadAllowlistVersion(version)

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

	k8sClient, clientErr := client.New(cfg, client.Options{Scheme: scheme})
	if clientErr != nil {
		logger.Error(clientErr, "failed to create kubernetes client")
		return
	}

	if err := applyCSIAllowlistSynchronizerResource(k8sClient, resolvedVersion, partOfLabel); err != nil {
		logger.Error(err, "failed to apply CSI AllowlistSynchronizer resource")
		return
	}

	logger.V(1).Info("Successfully applied CSI AllowlistSynchronizer", "version", resolvedVersion)
}

// GetCSIMatchingAllowlistLabelValue returns the value for the cloud.google.com/matching-allowlist
// label that should be set on the CSI driver DaemonSet. If a custom version is provided and valid,
// it computes the label value; otherwise it returns CSIMatchingAllowlistLabel (the default for v1.1.0).
func GetCSIMatchingAllowlistLabelValue(version string) string {
	resolvedVersion := resolveCSIWorkloadAllowlistVersion(version)
	if resolvedVersion == DefaultCSIWorkloadAllowlistVersion {
		return CSIMatchingAllowlistLabel
	}
	return fmt.Sprintf("datadog-datadog-csi-driver-daemonset-exemption-%s", resolvedVersion)
}
