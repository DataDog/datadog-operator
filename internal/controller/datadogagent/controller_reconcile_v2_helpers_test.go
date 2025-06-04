package datadogagent

import (
	"fmt"
	"testing"

	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"

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
func (df *dummyFeature) ManageDependencies(managers feature.ResourceManagers) error {
	return df.ManageDependenciesError
}

// ManageClusterAgent returns a predefined error (or nil for success).
func (df *dummyFeature) ManageClusterAgent(managers feature.PodTemplateManagers, provider string) error {
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
	err := r.manageFeatureDependencies(dummyLogger, []feature.Feature{f1}, dummyResMgrs)
	require.NoError(t, err)

	// Test with one failing feature.
	err = r.manageFeatureDependencies(dummyLogger, []feature.Feature{f1, f2}, dummyResMgrs)
	require.Error(t, err)
	require.Contains(t, err.Error(), "fail dependency")
}
