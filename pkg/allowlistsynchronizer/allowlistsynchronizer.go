// Package allowlistsynchronizer contains helpers to manage the
// AllowlistSynchronizer CRD required by GKE Autopilot clusters.
package allowlistsynchronizer

import (
	"context"
	"fmt"
	"regexp"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// DefaultWorkloadAllowlistVersion is the default version of the Datadog
// daemonset WorkloadAllowlist. v1.0.3 includes the system-probe / NPM
// exemptions required by the NPM feature on GKE Autopilot.
const DefaultWorkloadAllowlistVersion = "v1.0.3"

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

func createAllowlistSynchronizerResource(k8sClient client.Client, version string) error {
	obj := &AllowlistSynchronizer{
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
			AllowlistPaths: []string{
				fmt.Sprintf("Datadog/datadog/datadog-datadog-daemonset-exemption-%s.yaml", version),
			},
		},
	}

	return k8sClient.Create(context.TODO(), obj)
}

// CreateAllowlistSynchronizer creates a GKE AllowlistSynchronizer Custom Resource (auto.gke.io/v1) for the Datadog WorkloadAllowlist if it doesn't exist.
// The AllowlistSynchronizer is needed so that GKE Autopilot can sync the Datadog WorkloadAllowlist to the cluster. See the CRD reference:
// https://cloud.google.com/kubernetes-engine/docs/reference/crds/allowlistsynchronizer
//
// version selects the WorkloadAllowlist YAML to point at. Pass an empty string
// to use DefaultWorkloadAllowlistVersion. Malformed versions also fall back to
// the default.
func CreateAllowlistSynchronizer(version string) {
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

	existing := &AllowlistSynchronizer{}
	if existingErr := k8sClient.Get(context.TODO(), client.ObjectKey{Name: "datadog-synchronizer"}, existing); existingErr == nil {
		return
	} else if !apierrors.IsNotFound(existingErr) {
		logger.Error(existingErr, "failed to check existing AllowlistSynchronizer resource")
		return
	}

	if err := createAllowlistSynchronizerResource(k8sClient, resolvedVersion); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return
		}
		logger.Error(err, "failed to create AllowlistSynchronizer resource")
		return
	}

	logger.Info("Successfully created AllowlistSynchronizer", "version", resolvedVersion)
}
