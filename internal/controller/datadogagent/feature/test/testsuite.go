package test

import (
	"testing"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	testutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/testutils"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
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
	DDA            *v2alpha1.DatadogAgent
	Options        *Options
	FeatureOptions *feature.Options
	// Dependencies Store
	StoreOption        *store.StoreOptions
	StoreInitFunc      func(store store.StoreClient)
	RequiredComponents feature.RequiredComponents
	// Test configuration
	Agent               *ComponentTest
	ClusterAgent        *ComponentTest
	ClusterChecksRunner *ComponentTest
	// Want
	WantConfigure             bool
	WantManageDependenciesErr bool
	WantDependenciesFunc      func(testing.TB, store.StoreClient)
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
			// Bare minimum PodTemplateSpec with one agent container
			newPTS := v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:    string(apicommon.CoreAgentContainerName),
							Image:   images.GetLatestAgentImage(),
							Command: []string{"agent", "run"},
						},
					},
				},
			}
			return fake.NewPodTemplateManagers(t, newPTS), defaultProvider
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
	t.Helper()

	for _, test := range suite {
		t.Run(test.Name, func(tt *testing.T) {
			runTest(tt, test)
		})
	}
}

func runTest(t *testing.T, tt FeatureTest) {
	t.Helper()
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	logger := logf.Log.WithName(tt.Name)

	// Use feature.BuildFeatures to get list of features and required components.
	// If feature is not enabled, features slice will be empty.
	// For a given test only one feature will be registered with the `featureBuilders`.
	var features []feature.Feature
	var gotConfigure feature.RequiredComponents
	var dda metav1.Object
	featureOptions := &feature.Options{}
	if tt.FeatureOptions != nil {
		featureOptions = tt.FeatureOptions
	}
	featureOptions.Logger = logger
	if tt.DDA != nil {
		var configuredFeatures []feature.Feature
		var enabledFeatures []feature.Feature
		configuredFeatures, enabledFeatures, gotConfigure = feature.BuildFeatures(tt.DDA, featureOptions)
		features = append(configuredFeatures, enabledFeatures...)
		dda = tt.DDA
	} else {
		t.Fatal("No DatadogAgent CRD provided")
	}

	// verify features and required components produced aligns with test expecations
	verifyFeatures(t, tt, features, gotConfigure)

	// dependencies
	store, depsManager := initDependencies(tt, logger, dda)

	for _, feat := range features {
		if err := feat.ManageDependencies(depsManager); (err != nil) != tt.WantManageDependenciesErr {
			t.Errorf("feature.ManageDependencies() error = %v, wantErr %v", err, tt.WantManageDependenciesErr)
			return
		}

		if tt.WantDependenciesFunc != nil {
			tt.WantDependenciesFunc(t, store)
		}

		// check Manage functions
		if tt.ClusterAgent != nil {
			tplManager, _ := tt.ClusterAgent.CreateFunc(t)
			_ = feat.ManageClusterAgent(tplManager, "")
			tt.ClusterAgent.WantFunc(t, tplManager)
		}

		if tt.Agent != nil {
			tplManager, provider := tt.Agent.CreateFunc(t)
			if len(gotConfigure.Agent.Containers) > 0 && gotConfigure.Agent.Containers[0] == common.UnprivilegedSingleAgentContainerName {
				_ = feat.ManageSingleContainerNodeAgent(tplManager, provider)
			} else {
				_ = feat.ManageNodeAgent(tplManager, provider)
			}

			tt.Agent.WantFunc(t, tplManager)
		}

		if tt.ClusterChecksRunner != nil {
			tplManager, _ := tt.ClusterChecksRunner.CreateFunc(t)
			_ = feat.ManageClusterChecksRunner(tplManager)
			tt.ClusterChecksRunner.WantFunc(t, tplManager)
		}
	}
}

func initDependencies(tt FeatureTest, logger logr.Logger, dda metav1.Object) (*store.Store, feature.ResourceManagers) {
	if tt.StoreOption == nil {
		tt.StoreOption = &store.StoreOptions{
			Logger: logger,
		}
	}
	if tt.StoreOption.Scheme == nil {
		tt.StoreOption.Scheme = testutils.TestScheme()
	}

	store := store.NewStore(dda, tt.StoreOption)
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

	if tt.WantConfigure && gotConfigure.IsEnabled() && len(features) == 0 {
		t.Errorf("feature wanted but feature.BuildFeatures() return empty slice")
	}

	if gotConfigure.IsConfigured() != tt.WantConfigure {
		t.Errorf("feature.Configure() = %v, want %v", gotConfigure.IsConfigured(), tt.WantConfigure)
	}
}
