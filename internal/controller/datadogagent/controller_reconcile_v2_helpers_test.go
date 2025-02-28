package datadogagent

import (
	"context"
	"fmt"
	"testing"

	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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
}

// ID returns the dummy feature's ID.
func (df *dummyFeature) ID() feature.IDType {
	return feature.IDType(df.IDValue)
}

// Configure returns a predefined RequiredComponents value.
func (df *dummyFeature) Configure(dda *datadoghqv2alpha1.DatadogAgent) feature.RequiredComponents {
	return df.ConfigureReturn
}

// ManageDependencies returns a predefined error (or nil for success).
func (df *dummyFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	return df.ManageDependenciesError
}

// ManageClusterAgent returns a predefined error (or nil for success).
func (df *dummyFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return df.ManageClusterAgentError
}

// ManageNodeAgent returns a predefined error (or nil for success).
func (df *dummyFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return df.ManageNodeAgentError
}

// ManageSingleContainerNodeAgent returns a predefined error (or nil for success).
func (df *dummyFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return df.ManageSingleContainerAgentError
}

// ManageClusterChecksRunner returns a predefined error (or nil for success).
func (df *dummyFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return df.ManageClusterChecksRunnerError
}

// Test_fetchAndValidateDatadogAgent tests the retrieval and basic validation of a DatadogAgent.
func Test_fetchAndValidateDatadogAgent(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, datadoghqv2alpha1.AddToScheme(scheme))

	// Create a valid DatadogAgent with credentials configured.
	validAgent := &datadoghqv2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "valid",
			Namespace: "default",
		},
		Spec: datadoghqv2alpha1.DatadogAgentSpec{
			Global: &datadoghqv2alpha1.GlobalConfig{
				Credentials: &datadoghqv2alpha1.DatadogCredentials{
					APIKey: apiutils.NewStringPointer("dummy"),
				},
			},
		},
	}
	// Create an invalid DatadogAgent (missing credentials).
	invalidAgent := &datadoghqv2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid",
			Namespace: "default",
		},
		Spec: datadoghqv2alpha1.DatadogAgentSpec{
			Global: nil,
		},
	}

	// --- Valid agent test ---
	cli := fakeclient.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(validAgent).Build()
	r := &Reconciler{
		client: cli,
	}
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "valid", Namespace: "default"}}
	inst, res, err := r.fetchAndValidateDatadogAgent(context.TODO(), req)
	require.NoError(t, err)
	require.NotNil(t, inst)
	require.Equal(t, reconcile.Result{}, res)

	// --- Not found test ---
	reqNF := reconcile.Request{NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: "default"}}
	inst, res, err = r.fetchAndValidateDatadogAgent(context.TODO(), reqNF)
	require.NoError(t, err)
	require.Nil(t, inst)

	// --- Invalid (missing credentials) test ---
	cli2 := fakeclient.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(invalidAgent).Build()
	r2 := &Reconciler{
		client: cli2,
	}
	reqInv := reconcile.Request{NamespacedName: types.NamespacedName{Name: "invalid", Namespace: "default"}}
	inst, res, err = r2.fetchAndValidateDatadogAgent(context.TODO(), reqInv)
	require.Error(t, err)
	require.Contains(t, err.Error(), "credentials not configured")
}

// Test_setupDependencies verifies that store and resource managers are initialized.
func Test_setupDependencies(t *testing.T) {
	// Create a dummy DatadogAgent instance.
	dummyAgent := &datadoghqv2alpha1.DatadogAgent{}
	dummyPlatformInfo := kubernetes.PlatformInfo{}
	dummyLogger := logr.Discard()
	dummyOpts := &ReconcilerOptions{
		SupportCilium: false,
	}
	scheme := runtime.NewScheme()
	r := &Reconciler{
		options:      *dummyOpts,
		platformInfo: dummyPlatformInfo,
		scheme:       scheme,
	}
	storeObj, resMgrs := r.setupDependencies(dummyAgent, dummyLogger)
	require.NotNil(t, storeObj)
	require.NotNil(t, resMgrs)
}

// Test_manageFeatureDependencies checks that feature dependency management aggregates errors correctly.
func Test_manageFeatureDependencies(t *testing.T) {
	dummyLogger := logr.Discard()
	dummyResMgrs := feature.NewResourceManagers(nil) // passing nil for simplicity
	dummyRequiredComponents := feature.RequiredComponents{}

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
	err := r.manageFeatureDependencies(dummyLogger, []feature.Feature{f1}, dummyRequiredComponents, dummyResMgrs)
	require.NoError(t, err)

	// Test with one failing feature.
	err = r.manageFeatureDependencies(dummyLogger, []feature.Feature{f1, f2}, dummyRequiredComponents, dummyResMgrs)
	require.Error(t, err)
	require.Contains(t, err.Error(), "fail dependency")
}
