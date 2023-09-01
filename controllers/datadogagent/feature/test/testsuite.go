package test

import (
	"testing"

	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	testutils "github.com/DataDog/datadog-operator/controllers/datadogagent/testutils"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FeatureTestSuite use define several tests on a Feature
// how to define a test:
//
//		func Test_MyFeature_(t *testing.T) {
//			tests := test.FeatureTestSuite{}
//	  	tests.Run(t, myFeatureBuildFunc)
//		}
type FeatureTestSuite []FeatureTest

// FeatureTest use to define a Feature test
type FeatureTest struct {
	Name string
	// Inputs
	DDAv2   *v2alpha1.DatadogAgent
	DDAv1   *v1alpha1.DatadogAgent
	Options *Options
	// Dependencies Store
	StoreOption        *dependencies.StoreOptions
	StoreInitFunc      func(store dependencies.StoreClient)
	RequiredComponents feature.RequiredComponents
	// Test configuration
	Agent               *ComponentTest
	ClusterAgent        *ComponentTest
	ClusterChecksRunner *ComponentTest
	// Want
	WantConfigure             bool
	WantManageDependenciesErr bool
	WantDependenciesFunc      func(testing.TB, dependencies.StoreClient)
}

// Options use to provide some option to the test.
type Options struct{}

// ComponentTest use to configure how to test a component (Cluster-Agent, Agent, ClusterChecksRunner)
type ComponentTest struct {
	CreateFunc func(testing.TB) feature.PodTemplateManagers
	WantFunc   func(testing.TB, feature.PodTemplateManagers)
}

// NewDefaultComponentTest returns a default ComponentTest
func NewDefaultComponentTest() *ComponentTest {
	return &ComponentTest{
		CreateFunc: func(t testing.TB) feature.PodTemplateManagers {
			return fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{})
		},
	}
}

// WithCreateFunc sets CreateFunc
func (ct *ComponentTest) WithCreateFunc(f func(testing.TB) feature.PodTemplateManagers) *ComponentTest {
	ct.CreateFunc = f
	return ct
}

// WithWantFunc sets WantFunc
func (ct *ComponentTest) WithWantFunc(f func(testing.TB, feature.PodTemplateManagers)) *ComponentTest {
	ct.WantFunc = f
	return ct
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
	var dda metav1.Object
	var isV2 bool
	if tt.DDAv2 != nil {
		gotConfigure = f.Configure(tt.DDAv2)
		dda = tt.DDAv2
		isV2 = true
	} else if tt.DDAv1 != nil {
		gotConfigure = f.ConfigureV1(tt.DDAv1)
		dda = tt.DDAv1
	} else {
		t.Fatal("No DatadogAgent CRD provided")
	}

	if gotConfigure.IsEnabled() != tt.WantConfigure {
		t.Errorf("feature.Configure() = %v, want %v", gotConfigure.IsEnabled(), tt.WantConfigure)
	}

	if !gotConfigure.IsEnabled() {
		// If the feature is not enabled return now
		return
	}

	if tt.StoreOption == nil {
		tt.StoreOption = &dependencies.StoreOptions{
			Logger: logger,
		}
	}
	if tt.StoreOption.Scheme == nil {
		tt.StoreOption.Scheme = testutils.TestScheme(isV2)
	}

	// dependencies
	store := dependencies.NewStore(dda, tt.StoreOption)
	if tt.StoreInitFunc != nil {
		tt.StoreInitFunc(store)
	}
	depsManager := feature.NewResourceManagers(store)

	if err := f.ManageDependencies(depsManager, tt.RequiredComponents); (err != nil) != tt.WantManageDependenciesErr {
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

	if tt.ClusterChecksRunner != nil {
		tplManager := tt.ClusterChecksRunner.CreateFunc(t)
		_ = f.ManageClusterChecksRunner(tplManager)
		tt.ClusterChecksRunner.WantFunc(t, tplManager)
	}
}
