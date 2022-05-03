package datadogagent

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	test "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1/test"
	assert "github.com/stretchr/testify/require"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	testutils "github.com/DataDog/datadog-operator/controllers/datadogagent/testutils"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type extendeddaemonsetFromInstanceTest struct {
	name            string
	agentdeployment *datadoghqv1alpha1.DatadogAgent
	selector        *metav1.LabelSelector
	checkEDSFuncs   []testutils.CheckExtendedDaemonSetFunc
	wantErr         bool
}

func (test extendeddaemonsetFromInstanceTest) Run(t *testing.T) {
	t.Helper()
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	logger := logf.Log.WithName(t.Name())
	got, _, err := newExtendedDaemonSetFromInstance(logger, test.agentdeployment, test.selector)
	if test.wantErr {
		assert.Error(t, err, "newExtendedDaemonSetFromInstance() expected an error")
	} else {
		assert.NoError(t, err, "newExtendedDaemonSetFromInstance() unexpected error: %v", err)
	}

	// Remove the generated hash before comparison because it is not easy generate it in the test definition.
	delete(got.Annotations, apicommon.MD5AgentDeploymentAnnotationKey)

	for _, checkFunc := range test.checkEDSFuncs {
		checkFunc(t, got)
	}
}

func Test_EDSFromDefaultAgent(t *testing.T) {
	defaultDatadogAgent := test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{UseEDS: true, ClusterAgentEnabled: true})

	test1 := extendeddaemonsetFromInstanceTest{
		name:            "defaulted case",
		agentdeployment: defaultDatadogAgent,
		checkEDSFuncs: []testutils.CheckExtendedDaemonSetFunc{
			testutils.CheckMetadaInEDS(&testutils.CheckNameNamespace{Namespace: "bar", Name: "foo-agent"}),
			// check labels
			testutils.CheckMetadaInEDS(&testutils.CheckLabelIsPresent{Key: "agent.datadoghq.com/name", Value: "foo"}),
			testutils.CheckMetadaInEDS(&testutils.CheckLabelIsPresent{Key: "agent.datadoghq.com/component", Value: "agent"}),
			testutils.CheckMetadaInEDS(&testutils.CheckLabelIsPresent{Key: "app.kubernetes.io/instance", Value: "agent"}),
			testutils.CheckMetadaInEDS(&testutils.CheckLabelIsPresent{Key: "app.kubernetes.io/managed-by", Value: "datadog-operator"}),
			// check containers creation
			testutils.CheckPodTemplateInEDS(&testutils.CheckContainerNameIsPresentFunc{Name: "agent"}),
			testutils.CheckPodTemplateInEDS(&testutils.CheckContainerNameIsPresentFunc{Name: "process-agent"}),
		},
		wantErr: false,
	}
	test1.Run(t)
}
