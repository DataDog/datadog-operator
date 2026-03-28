package fleet

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
)

func init() {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
}

func testDaemon(objs ...runtime.Object) *Daemon {
	s := runtime.NewScheme()
	_ = v2alpha1.AddToScheme(s)
	builder := fake.NewClientBuilder().WithScheme(s)
	for _, obj := range objs {
		builder = builder.WithRuntimeObjects(obj)
	}
	builder = builder.WithStatusSubresource(&v2alpha1.DatadogAgent{})
	return &Daemon{
		logger:     logr.New(logf.NullLogSink{}),
		kubeClient: builder.Build(),
		configs:    make(map[string]installerConfig),
	}
}

func baseDDA() *v2alpha1.DatadogAgent {
	return &v2alpha1.DatadogAgent{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogAgent",
			APIVersion: "datadoghq.com/v2alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog-agent",
			Namespace: "datadog",
		},
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				APM: &v2alpha1.APMFeatureConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
				NPM: &v2alpha1.NPMFeatureConfig{
					Enabled: apiutils.NewBoolPointer(true),
				},
			},
		},
	}
}

func makeConfig(id string) installerConfig {
	return installerConfig{
		ID: id,
		Operations: []fleetManagementOperation{
			{
				Operation:        operationUpdate,
				GroupVersionKind: schema.GroupVersionKind{Group: "datadoghq.com", Version: "v2alpha1", Kind: "DatadogAgent"},
				NamespacedName:   types.NamespacedName{Namespace: "datadog", Name: "datadog-agent"},
				Config:           json.RawMessage(`{"spec":{"features":{"apm":{"enabled":true}}}}`),
			},
		},
	}
}

func getDDAFromDaemon(t *testing.T, d *Daemon) *v2alpha1.DatadogAgent {
	t.Helper()
	dda := &v2alpha1.DatadogAgent{}
	err := d.kubeClient.Get(context.TODO(), types.NamespacedName{Name: "datadog-agent", Namespace: "datadog"}, dda)
	require.NoError(t, err)
	return dda
}

func TestExtractDDAPatch_Success(t *testing.T) {
	cfg := makeConfig("cfg-1")
	target, patch, err := extractDDAPatch(cfg)
	require.NoError(t, err)
	assert.Equal(t, "datadog", target.Namespace)
	assert.Equal(t, "datadog-agent", target.Name)
	assert.NotEmpty(t, patch)
}

func TestExtractDDAPatch_NoMatch(t *testing.T) {
	cfg := installerConfig{ID: "cfg-1"}
	_, _, err := extractDDAPatch(cfg)
	require.Error(t, err)
}

func TestStartExperiment_Success(t *testing.T) {
	dda := baseDDA()
	d := testDaemon(dda)
	d.configs["cfg-1"] = makeConfig("cfg-1")

	err := d.startDatadogAgentExperiment(remoteAPIRequest{
		ID:            "exp-001",
		ExpectedState: expectedState{ExperimentConfig: "cfg-1"},
	})
	require.NoError(t, err)

	updated := getDDAFromDaemon(t, d)
	assert.True(t, apiutils.BoolValue(updated.Spec.Features.APM.Enabled))
	assert.True(t, apiutils.BoolValue(updated.Spec.Features.NPM.Enabled), "NPM preserved")
	require.NotNil(t, updated.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, updated.Status.Experiment.Phase)
	assert.Equal(t, "exp-001", updated.Status.Experiment.ID)
}

func TestStartExperiment_MissingConfig(t *testing.T) {
	d := testDaemon(baseDDA())
	err := d.startDatadogAgentExperiment(remoteAPIRequest{
		ID:            "exp-001",
		ExpectedState: expectedState{ExperimentConfig: "nonexistent"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestStartExperiment_MissingID(t *testing.T) {
	d := testDaemon(baseDDA())
	err := d.startDatadogAgentExperiment(remoteAPIRequest{ID: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing ID")
}

func TestStartExperiment_DDANotFound(t *testing.T) {
	d := testDaemon() // no DDA
	d.configs["cfg-1"] = makeConfig("cfg-1")
	err := d.startDatadogAgentExperiment(remoteAPIRequest{
		ID:            "exp-001",
		ExpectedState: expectedState{ExperimentConfig: "cfg-1"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get DDA")
}

func TestStartExperiment_AlreadyRunning(t *testing.T) {
	dda := baseDDA()
	dda.Status.Experiment = &v2alpha1.ExperimentStatus{Phase: v2alpha1.ExperimentPhaseRunning, ID: "exp-001"}
	d := testDaemon(dda)
	d.configs["cfg-1"] = makeConfig("cfg-1")
	err := d.startDatadogAgentExperiment(remoteAPIRequest{
		ID:            "exp-002",
		ExpectedState: expectedState{ExperimentConfig: "cfg-1"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "active")
}

func TestStartExperiment_AfterAborted(t *testing.T) {
	dda := baseDDA()
	dda.Status.Experiment = &v2alpha1.ExperimentStatus{Phase: v2alpha1.ExperimentPhaseAborted, ID: "exp-001"}
	d := testDaemon(dda)
	d.configs["cfg-1"] = makeConfig("cfg-1")
	err := d.startDatadogAgentExperiment(remoteAPIRequest{
		ID:            "exp-002",
		ExpectedState: expectedState{ExperimentConfig: "cfg-1"},
	})
	require.NoError(t, err)
	updated := getDDAFromDaemon(t, d)
	assert.Equal(t, "exp-002", updated.Status.Experiment.ID)
}

func TestStopExperiment_Running(t *testing.T) {
	dda := baseDDA()
	dda.Status.Experiment = &v2alpha1.ExperimentStatus{Phase: v2alpha1.ExperimentPhaseRunning, ID: "exp-001", Generation: 1}
	d := testDaemon(dda)
	err := d.stopDatadogAgentExperiment(remoteAPIRequest{ID: "exp-001"})
	require.NoError(t, err)
	updated := getDDAFromDaemon(t, d)
	assert.Equal(t, v2alpha1.ExperimentPhaseStopped, updated.Status.Experiment.Phase)
}

func TestStopExperiment_NoRunning(t *testing.T) {
	d := testDaemon(baseDDA())
	err := d.stopDatadogAgentExperiment(remoteAPIRequest{ID: "exp-001"})
	require.NoError(t, err)
}

func TestStopExperiment_IDMismatch(t *testing.T) {
	dda := baseDDA()
	dda.Status.Experiment = &v2alpha1.ExperimentStatus{Phase: v2alpha1.ExperimentPhaseRunning, ID: "exp-001"}
	d := testDaemon(dda)
	err := d.stopDatadogAgentExperiment(remoteAPIRequest{ID: "exp-999"})
	require.NoError(t, err)
	updated := getDDAFromDaemon(t, d)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, updated.Status.Experiment.Phase)
}

func TestPromoteExperiment_Running(t *testing.T) {
	dda := baseDDA()
	dda.Status.Experiment = &v2alpha1.ExperimentStatus{Phase: v2alpha1.ExperimentPhaseRunning, ID: "exp-001"}
	d := testDaemon(dda)
	err := d.promoteDatadogAgentExperiment(remoteAPIRequest{ID: "exp-001"})
	require.NoError(t, err)
	updated := getDDAFromDaemon(t, d)
	assert.Equal(t, v2alpha1.ExperimentPhasePromoted, updated.Status.Experiment.Phase)
}

func TestPromoteExperiment_NoRunning(t *testing.T) {
	d := testDaemon(baseDDA())
	err := d.promoteDatadogAgentExperiment(remoteAPIRequest{ID: "exp-001"})
	require.NoError(t, err)
}
