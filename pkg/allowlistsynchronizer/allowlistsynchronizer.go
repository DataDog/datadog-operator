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

// DefaultCSIWorkloadAllowlistVersion is the default version of the Datadog CSI
// driver daemonset WorkloadAllowlist.
const DefaultCSIWorkloadAllowlistVersion = "v1.1.0"

const allowlistSynchronizerFieldOwner = "datadog-operator-allowlist-synchronizer"

const (
	agentAllowlistSynchronizerName = "datadog-synchronizer"
	agentAllowlistAppNameLabel     = "datadog-allowlist-synchronizer"

	csiAllowlistSynchronizerName = "datadog-csi-synchronizer"
	csiAllowlistAppNameLabel     = "datadog-csi-allowlist-synchronizer"
)

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
	return resolveWorkloadAllowlistVersionWithDefault(version, DefaultWorkloadAllowlistVersion)
}

func resolveCSIWorkloadAllowlistVersion(version string) string {
	return resolveWorkloadAllowlistVersionWithDefault(version, DefaultCSIWorkloadAllowlistVersion)
}

func resolveWorkloadAllowlistVersionWithDefault(version, defaultVersion string) string {
	if version == "" {
		return defaultVersion
	}
	if !workloadAllowlistVersionRegexp.MatchString(version) {
		logger.Info("Ignoring malformed WorkloadAllowlist version override, falling back to default",
			"requested", version, "default", defaultVersion)
		return defaultVersion
	}
	return version
}

func applyAllowlistSynchronizerResource(k8sClient client.Client, version, partOfLabel string) error {
	return applyAllowlistSynchronizerResourceForPath(
		k8sClient,
		agentAllowlistSynchronizerName,
		agentAllowlistAppNameLabel,
		fmt.Sprintf("Datadog/datadog/datadog-datadog-daemonset-exemption-%s.yaml", version),
		partOfLabel,
	)
}

func applyCSIAllowlistSynchronizerResource(k8sClient client.Client, version, partOfLabel string) error {
	return applyAllowlistSynchronizerResourceForPath(
		k8sClient,
		csiAllowlistSynchronizerName,
		csiAllowlistAppNameLabel,
		fmt.Sprintf("Datadog/datadog-csi-driver/datadog-datadog-csi-driver-daemonset-exemption-%s.yaml", version),
		partOfLabel,
	)
}

func applyAllowlistSynchronizerResourceForPath(k8sClient client.Client, name, appNameLabel, allowlistPath, partOfLabel string) error {
	obj := &AllowlistSynchronizer{
		TypeMeta: metav1.TypeMeta{
			APIVersion: SchemeGroupVersion.String(),
			Kind:       "AllowlistSynchronizer",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"app.kubernetes.io/created-by":           "datadog-operator",
				kubernetes.AppKubernetesManageByLabelKey: "datadog-operator",
				kubernetes.AppKubernetesNameLabelKey:     appNameLabel,
				kubernetes.AppKubernetesPartOfLabelKey:   partOfLabel,
			},
		},
		Spec: AllowlistSynchronizerSpec{
			AllowlistPaths: []string{
				allowlistPath,
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

	createAllowlistSynchronizer(resolvedVersion, partOfLabel, applyAllowlistSynchronizerResource)
}

// CreateCSIAllowlistSynchronizer creates a GKE AllowlistSynchronizer Custom Resource (auto.gke.io/v1)
// for the Datadog CSI driver WorkloadAllowlist if it doesn't exist.
//
// version selects the Datadog CSI driver WorkloadAllowlist YAML to point at.
// Pass an empty string to use DefaultCSIWorkloadAllowlistVersion. Malformed
// versions also fall back to the default.
func CreateCSIAllowlistSynchronizer(version, partOfLabel string) {
	resolvedVersion := resolveCSIWorkloadAllowlistVersion(version)

	createAllowlistSynchronizer(resolvedVersion, partOfLabel, applyCSIAllowlistSynchronizerResource)
}

func createAllowlistSynchronizer(version, partOfLabel string, applyFunc func(client.Client, string, string) error) {
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

	if err := applyFunc(k8sClient, version, partOfLabel); err != nil {
		logger.Error(err, "failed to apply AllowlistSynchronizer resource")
		return
	}

	logger.V(1).Info("Successfully applied AllowlistSynchronizer", "version", version)
}
