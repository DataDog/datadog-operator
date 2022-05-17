package test

import (
	"testing"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
)

// FeatureTestSuite use define several tests on a Feature
// how to define a test:
// 	func Test_MyFeature_(t *testing.T) {
//		tests := test.FeatureTestSuite{}
//   	tests.Run(t, myFeatureBuildFunc)
//	}
type FeatureTestSuite []FeatureTest

// FeatureTest use to define a Feature test
type FeatureTest struct {
	Name string
	// Inputs
	DDAv2   *v2alpha1.DatadogAgent
	DDAv1   *v1alpha1.DatadogAgent
	Options *Options
	// Dependencies Store
	StoreOption   *dependencies.StoreOptions
	StoreInitFunc func(store dependencies.StoreClient)
	// Test configuration
	Agent              *ComponentTest
	ClusterAgent       *ComponentTest
	ClusterCheckRunner *ComponentTest
	// Want
	WantConfigure             bool
	WantManageDependenciesErr bool
	WantDependenciesFunc      func(testing.TB, dependencies.StoreClient)
}

// Options use to provide some option to the test.
type Options struct{}

// ComponentTest use to configure how to test a component (Cluster-Agent, Agent, ClusterCheckRunner)
type ComponentTest struct {
	CreateFunc func(testing.TB) feature.PodTemplateManagers
	WantFunc   func(testing.TB, feature.PodTemplateManagers)
}

// Run use to run the Feature test suite.
func (suite FeatureTestSuite) Run(t *testing.T, buildFunc feature.BuildFunc) {
	for _, test := range suite {
		runTest(t, test, buildFunc)
	}
}

func runTest(t *testing.T, tt FeatureTest, buildFunc feature.BuildFunc) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	logger := logf.Log.WithName(tt.Name)

	f := buildFunc(&feature.Options{
		Logger: logger,
	})

	// check feature Configure function
	var gotConfigure feature.RequiredComponents
	if tt.DDAv2 != nil {
		gotConfigure = f.Configure(tt.DDAv2)
	} else if tt.DDAv1 != nil {
		gotConfigure = f.ConfigureV1(tt.DDAv1)
	} else {
		t.Fatal("No DatadogAgent CRD provided")
	}

	if gotConfigure.IsEnabled() != tt.WantConfigure {
		t.Errorf("feature.Configure() = %v, want %v", gotConfigure, tt.WantConfigure)
	}

	if !gotConfigure.IsEnabled() {
		// If the feature is now enable return now
		return
	}

	// dependencies
	store := dependencies.NewStore(tt.StoreOption)
	if tt.StoreInitFunc != nil {
		tt.StoreInitFunc(store)
	}
	depsManager := feature.NewResourceManagers(store)

	if err := f.ManageDependencies(depsManager); (err != nil) != tt.WantManageDependenciesErr {
		t.Errorf("feature.ManageDependencies() error = %v, wantErr %v", err, tt.WantManageDependenciesErr)
		return
	}

	if tt.WantDependenciesFunc != nil {
		tt.WantDependenciesFunc(t, store)
	}

	// check Manage functions
	if tt.ClusterAgent != nil {
		tplManager := tt.ClusterAgent.CreateFunc(t)
		_ = f.ManageClusterAgent(tplManager)
		tt.ClusterAgent.WantFunc(t, tplManager)
	}

	if tt.Agent != nil {
		tplManager := tt.Agent.CreateFunc(t)
		_ = f.ManageNodeAgent(tplManager)
		tt.Agent.WantFunc(t, tplManager)
	}

	if tt.ClusterCheckRunner != nil {
		tplManager := tt.ClusterCheckRunner.CreateFunc(t)
		_ = f.ManageClusterChecksRunner(tplManager)
		tt.ClusterCheckRunner.WantFunc(t, tplManager)
	}
}
