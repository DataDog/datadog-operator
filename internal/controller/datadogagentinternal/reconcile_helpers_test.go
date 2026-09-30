package datadogagentinternal

import (
	"context"
	"fmt"
	"testing"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/openshift"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	agenttestutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/testutils"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dummyFeature is a simple implementation of the Feature interface for testing purposes.
type dummyFeature struct {
	IDValue                         string
	ConfigureReturn                 feature.RequiredComponents
	ManageDependenciesError         error
	ManageClusterAgentError         error
	ManageNodeAgentError            error
	ManageSingleContainerAgentError error
	ManageClusterChecksRunnerError  error
	ManageOtelAgentGatewayError     error
}

// ID returns the dummy feature's ID.
func (df *dummyFeature) ID() feature.IDType {
	return feature.IDType(df.IDValue)
}

// Configure returns a predefined RequiredComponents value.
func (df *dummyFeature) Configure(ddai metav1.Object, ddaiSpec *v2alpha1.DatadogAgentSpec, ddaiRCStatus *v2alpha1.RemoteConfigConfiguration) feature.RequiredComponents {
	return df.ConfigureReturn
}

// ManageDependencies returns a predefined error (or nil for success).
func (df *dummyFeature) ManageDependencies(managers feature.ResourceManagers) error {
	return df.ManageDependenciesError
}

// ManageClusterAgent returns a predefined error (or nil for success).
func (df *dummyFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return df.ManageClusterAgentError
}

// ManageNodeAgent returns a predefined error (or nil for success).
func (df *dummyFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	return df.ManageNodeAgentError
}

// ManageSingleContainerNodeAgent returns a predefined error (or nil for success).
func (df *dummyFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers) error {
	return df.ManageSingleContainerAgentError
}

// ManageClusterChecksRunner returns a predefined error (or nil for success).
func (df *dummyFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return df.ManageClusterChecksRunnerError
}

// ManageOtelAgentGateway returns a predefined error (or nil for success).
func (df *dummyFeature) ManageOtelAgentGateway(managers feature.PodTemplateManagers) error {
	return df.ManageOtelAgentGatewayError
}

// Test_setupDependencies verifies that store and resource managers are initialized.
func Test_setupDependencies(t *testing.T) {
	// Create a dummy DatadogAgent instance.
	dummyAgent := &datadoghqv1alpha1.DatadogAgentInternal{}
	dummyPlatformInfo := kubernetes.PlatformInfo{}
	dummyOpts := &ReconcilerOptions{
		SupportCilium: false,
	}
	scheme := runtime.NewScheme()
	r := &Reconciler{
		options:      *dummyOpts,
		platformInfo: dummyPlatformInfo,
		scheme:       scheme,
	}
	storeObj, resMgrs := r.setupDependencies(context.Background(), dummyAgent)
	require.NotNil(t, storeObj)
	require.NotNil(t, resMgrs)
}

// Test_manageFeatureDependencies checks that feature dependency management aggregates errors correctly.
func Test_manageFeatureDependencies(t *testing.T) {
	dummyResMgrs := feature.NewResourceManagers(nil) // passing nil for simplicity

	// One feature succeeds and one fails.
	f1 := &dummyFeature{
		IDValue: "f1",
	}
	f2 := &dummyFeature{
		IDValue:                 "f2",
		ManageDependenciesError: fmt.Errorf("fail dependency"),
	}

	r := &Reconciler{}
	// Test when all features succeed.
	err := r.manageFeatureDependencies([]feature.Feature{f1}, dummyResMgrs)
	require.NoError(t, err)

	// Test with one failing feature.
	err = r.manageFeatureDependencies([]feature.Feature{f1, f2}, dummyResMgrs)
	require.Error(t, err)
	require.Contains(t, err.Error(), "fail dependency")
}

// TestApplyAndCleanupDependencies_DisownGatedByProvider covers the provider gate on the
// bundle ServiceAccount disown. Off OpenShift the same name may be an account the
// operator owns, so its markers must survive.
func TestApplyAndCleanupDependencies_DisownGatedByProvider(t *testing.T) {
	adoptedSA := func() *corev1.ServiceAccount {
		return &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      openshift.SCCServiceAccountName,
				Namespace: "ns",
				Labels:    map[string]string{store.OperatorStoreLabelKey: "true"},
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "DatadogAgentInternal", Name: "dd", UID: "ddai-uid"},
				},
			},
		}
	}

	tests := []struct {
		name         string
		provider     string
		wantDisowned bool
	}{
		{"detected openshift", "openshift-rhcos", true},
		{"bare openshift", kubernetes.OpenshiftProvider, true},
		{"non-openshift provider", kubernetes.GKECosProvider, false},
		{"no provider", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ddai := &datadoghqv1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "dd",
					Namespace:   "ns",
					UID:         "ddai-uid",
					Annotations: map[string]string{kubernetes.ProviderAnnotationKey: tt.provider},
				},
			}
			scheme := agenttestutils.TestScheme()
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(adoptedSA()).Build()
			r := &Reconciler{client: c, scheme: scheme}

			depsStore := store.NewStore(ddai, &store.StoreOptions{
				PlatformInfo: kubernetes.NewPlatformInfoFromVersionMaps(&version.Info{GitVersion: "1.32.0"}, nil, nil),
				Scheme:       scheme,
			})
			require.NoError(t, r.applyAndCleanupDependencies(context.Background(), ddai, depsStore))

			got := &corev1.ServiceAccount{}
			require.NoError(t, c.Get(context.Background(),
				types.NamespacedName{Namespace: "ns", Name: openshift.SCCServiceAccountName}, got))

			if tt.wantDisowned {
				assert.NotContains(t, got.Labels, store.OperatorStoreLabelKey)
				assert.Empty(t, got.OwnerReferences)
			} else {
				assert.Contains(t, got.Labels, store.OperatorStoreLabelKey)
				assert.Len(t, got.OwnerReferences, 1)
			}
		})
	}
}
