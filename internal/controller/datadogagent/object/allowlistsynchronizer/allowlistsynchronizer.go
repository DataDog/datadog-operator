package allowlistsynchronizer

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"k8s.io/apimachinery/pkg/runtime/schema"
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

type AllowlistSynchronizer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AllowlistSynchronizerSpec `json:"spec,omitempty"`
}

func (in *AllowlistSynchronizer) DeepCopyObject() runtime.Object {
	out := new(AllowlistSynchronizer)
	*out = *in
	return out
}

type AllowlistSynchronizerSpec struct {
	AllowlistPaths []string `json:"allowlistPaths,omitempty"`
}

func createAllowlistSynchronizerResource(k8sClient client.Client) error {
	obj := &AllowlistSynchronizer{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "allowlistsynchronizers.auto.gke.io",
			Kind:       "AllowlistSynchronizer",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "datadog-synchronizer",
			Annotations: map[string]string{
				"helm.sh/hook": "pre-install,pre-upgrade",
				"helm.sh/hook-weight": "-1",
			},
		},
		Spec: AllowlistSynchronizerSpec{
			AllowlistPaths: []string{
				"Datadog/datadog/datadog-datadog-daemonset-exemption-v1.0.1.yaml",
			},
		},
	}

	return k8sClient.Create(context.TODO(), obj)
}

func CreateAllowlistSynchronizer() {
	cfg, err := config.GetConfig()
	if err != nil {
		logger.Error(err, "failed to load kubeconfig")
		return
	}

	scheme := runtime.NewScheme()
	if err := SchemeBuilder.AddToScheme(scheme); err != nil {
		logger.Error(err, "failed to register AllowlistSynchronizer scheme")
		return
	}

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		logger.Error(err, "failed to create kubernetes client")
		return
	}

	existing := &AllowlistSynchronizer{}
	if err := k8sClient.Get(context.TODO(), client.ObjectKey{Name: "datadog-synchronizer"}, existing); err == nil {
		return
	} else if !apierrors.IsNotFound(err) {
		logger.Error(err, "failed to check existing AllowlistSynchronizer resource")
		return
	}

	if err := createAllowlistSynchronizerResource(k8sClient); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return
		}
		logger.Error(err, "failed to create AllowlistSynchronizer resource")
		return
	}

	logger.Info("Successfully created AllowlistSynchronizer")
}
