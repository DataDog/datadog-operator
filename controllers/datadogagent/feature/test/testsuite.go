package test

import (
	"testing"

	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"

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
	StoreOption        dependencies.StoreOptions
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
			return fake.NewPodTemplateManagers(t)
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

// testScheme return a runtime.Scheme for testing purpose
func testScheme(isV2 bool) *runtime.Scheme {
	s := runtime.NewScheme()
	s.AddKnownTypes(edsdatadoghqv1alpha1.GroupVersion, &edsdatadoghqv1alpha1.ExtendedDaemonSet{})
	s.AddKnownTypes(appsv1.SchemeGroupVersion, &appsv1.DaemonSet{})
	s.AddKnownTypes(appsv1.SchemeGroupVersion, &appsv1.Deployment{})
	s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Secret{})
	s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.ServiceAccount{})
	s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.ConfigMap{})
	s.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.ClusterRoleBinding{})
	s.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.ClusterRole{})
	s.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.Role{})
	s.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.RoleBinding{})
	s.AddKnownTypes(policyv1.SchemeGroupVersion, &policyv1.PodDisruptionBudget{})
	s.AddKnownTypes(apiregistrationv1.SchemeGroupVersion, &apiregistrationv1.APIServiceList{})
	s.AddKnownTypes(apiregistrationv1.SchemeGroupVersion, &apiregistrationv1.APIService{})
	s.AddKnownTypes(networkingv1.SchemeGroupVersion, &networkingv1.NetworkPolicy{})

	if isV2 {
		s.AddKnownTypes(v2alpha1.GroupVersion, &v2alpha1.DatadogAgent{})
	} else {
		s.AddKnownTypes(v1alpha1.GroupVersion, &v1alpha1.DatadogAgent{})
	}

	return s
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
		t.Errorf("feature.Configure() = %v, want %v", gotConfigure, tt.WantConfigure)
	}

	if !gotConfigure.IsEnabled() {
		// If the feature is now enable return now
		return
	}

	if (tt.StoreOption == dependencies.StoreOptions{}) {
		tt.StoreOption = dependencies.StoreOptions{
			Logger: logger,
		}
	}
	if tt.StoreOption.Scheme == nil {
		tt.StoreOption.Scheme = testScheme(isV2)
	}

	// dependencies
	store := dependencies.NewStore(dda, &tt.StoreOption)
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
