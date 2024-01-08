package test

import (
	"testing"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	testutils "github.com/DataDog/datadog-operator/controllers/datadogagent/testutils"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
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
	CreateFunc func(testing.TB) (feature.PodTemplateManagers, string)
	WantFunc   func(testing.TB, feature.PodTemplateManagers)
}

// NewDefaultComponentTest returns a default ComponentTest
func NewDefaultComponentTest() *ComponentTest {
	defaultProvider := kubernetes.DefaultProvider
	return &ComponentTest{
		CreateFunc: func(t testing.TB) (feature.PodTemplateManagers, string) {
			return fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{}), defaultProvider
		},
	}
}

// WithCreateFunc sets CreateFunc
func (ct *ComponentTest) WithCreateFunc(f func(testing.TB) (feature.PodTemplateManagers, string)) *ComponentTest {
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

	// Use feature.BuildFeatures to get list of features and required components.
	// If feature is not enabled, features slice will be empty.
	// For a given test only one feature will be registered with the `featureBuilders`.
	var features []feature.Feature
	var gotConfigure feature.RequiredComponents
	var dda metav1.Object
	var isV2 bool
	if tt.DDAv2 != nil {
		features, gotConfigure = feature.BuildFeatures(tt.DDAv2, &feature.Options{
			Logger: logger,
		})
		dda = tt.DDAv2
		isV2 = true
	} else if tt.DDAv1 != nil {
		features, gotConfigure = feature.BuildFeaturesV1(tt.DDAv1, &feature.Options{
			Logger: logger,
		})
		dda = tt.DDAv1
	} else {
		t.Fatal("No DatadogAgent CRD provided")
	}

	// verify features and required components produced aligns with test expecations
	verifyFeatures(t, tt, features, gotConfigure)

	// dependencies
	store, depsManager := initDependencies(tt, logger, isV2, dda)

	for _, feat := range features {
		if err := feat.ManageDependencies(depsManager, tt.RequiredComponents); (err != nil) != tt.WantManageDependenciesErr {
			t.Errorf("feature.ManageDependencies() error = %v, wantErr %v", err, tt.WantManageDependenciesErr)
			return
		}

		if tt.WantDependenciesFunc != nil {
			tt.WantDependenciesFunc(t, store)
		}

		// check Manage functions
		if tt.ClusterAgent != nil {
			tplManager, _ := tt.ClusterAgent.CreateFunc(t)
			_ = feat.ManageClusterAgent(tplManager)
			tt.ClusterAgent.WantFunc(t, tplManager)
		}

		if tt.Agent != nil {
			tplManager, provider := tt.Agent.CreateFunc(t)
			_ = feat.ManageNodeAgent(tplManager, provider)
			tt.Agent.WantFunc(t, tplManager)
		}

		if tt.ClusterChecksRunner != nil {
			tplManager, _ := tt.ClusterChecksRunner.CreateFunc(t)
			_ = feat.ManageClusterChecksRunner(tplManager)
			tt.ClusterChecksRunner.WantFunc(t, tplManager)
		}
	}
}

func initDependencies(tt FeatureTest, logger logr.Logger, isV2 bool, dda metav1.Object) (*dependencies.Store, feature.ResourceManagers) {
	if tt.StoreOption == nil {
		tt.StoreOption = &dependencies.StoreOptions{
			Logger: logger,
		}
	}
	if tt.StoreOption.Scheme == nil {
		tt.StoreOption.Scheme = testutils.TestScheme(isV2)
	}

	store := dependencies.NewStore(dda, tt.StoreOption)
	if tt.StoreInitFunc != nil {
		tt.StoreInitFunc(store)
	}
	depsManager := feature.NewResourceManagers(store)
	return store, depsManager
}

func verifyFeatures(t *testing.T, tt FeatureTest, features []feature.Feature, gotConfigure feature.RequiredComponents) {
	// Each test should test single feature
	if len(features) > 1 {
		t.Errorf("feature.BuildFeatures() produced more than one feature, " +
			"it should be 1 if when features is enabled, 0 otherwise. Check code")
	}

	if tt.WantConfigure && len(features) == 0 {
		t.Errorf("feature wanted but feature.BuildFeatures() return empty slice")
	}

	if gotConfigure.IsEnabled() != tt.WantConfigure {
		t.Errorf("feature.Configure() = %v, want %v", gotConfigure.IsEnabled(), tt.WantConfigure)
	}
}
