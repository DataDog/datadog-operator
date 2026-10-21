// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package componenthealthsuite contains end-to-end tests for the operator's
// ComponentHealth controller (--componentHealthEnabled / --componentHealthIntakeEnabled).
// The suite provisions a kind cluster + fakeintake, deploys the operator, points
// its agenthealth intake at the fakeintake (via --componentHealthIntakeURL), then
// injects faults on the managed Cluster Agent (via a DatadogAgent whose component
// is deliberately broken) and asserts the operator emits the expected
// HealthReport issues by reading them back from the fakeintake.
//
// Credentials are dummy on purpose: the fakeintake does not validate the API
// key, and the faults under test (image pull, scheduling) never bring the
// Cluster Agent to a running state.
package componenthealthsuite

import (
	"context"
	_ "embed"
	"slices"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/operatorparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/test/e2e/common"
	"github.com/DataDog/datadog-operator/test/e2e/provisioners"
	"github.com/DataDog/datadog-operator/test/e2e/tests/utils"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ddaName is the DatadogAgent the faults are applied to.
	ddaName = "datadog"
	// The operator Helm chart labels its Deployment and names its container.
	operatorDeploymentSelector = "app.kubernetes.io/name=datadog-operator"
	operatorContainerName      = "datadog-operator"

	// agenthealthPath is the fakeintake route that captures agent-health payloads
	// (matches the operator's agenthealthPath and fakeintake's agentHealthEndpoint).
	agenthealthPath = "/api/v2/agenthealth"

	// Env the suite injects onto the operator to redirect the intake at the
	// fakeintake once its (dynamic) URL is known.
	envIntakeEnabled = "DD_COMPONENT_HEALTH_INTAKE_ENABLED"
	envIntakeURL     = "DD_COMPONENT_HEALTH_INTAKE_URL"

	// dummy credentials: the fakeintake does not validate the key, and the faults
	// under test never bring the Cluster Agent to a running state.
	dummyAPIKey = "00000000000000000000000000000000"
	dummyAppKey = "0000000000000000000000000000000000000000"

	// imagePullBadTag is a Cluster Agent image tag that does not exist, forcing
	// ImagePullBackOff.
	imagePullBadTag = "this-tag-does-not-exist-e2e"

	detectTimeout  = 5 * time.Minute
	resolveTimeout = 5 * time.Minute
	pollInterval   = 10 * time.Second
)

// Operator Helm values live in testdata and are embedded at build time. The
// ${NAMESPACE} placeholder is substituted with the operator/DDA namespace.
//
//go:embed testdata/operator-values.yaml
var operatorValues string

func operatorHelmValues() string {
	return strings.ReplaceAll(operatorValues, "${NAMESPACE}", common.NamespaceName)
}

// componentHealthSuite validates component-level issue detection, emission, and
// resolution end-to-end. local selects the local kind provisioner (vs AWS
// kind-on-VM in CI).
type componentHealthSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
	local bool

	kubeClient k8sclient.Interface
	ddaClient  client.Client
}

func (s *componentHealthSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	s.kubeClient = s.Env().KubernetesCluster.Client()
	scheme := runtime.NewScheme()
	s.Require().NoError(v2alpha1.AddToScheme(scheme))
	ddaClient, err := client.New(s.Env().KubernetesCluster.KubernetesClient.K8sConfig, client.Options{Scheme: scheme})
	s.Require().NoError(err, "failed to build DatadogAgent client")
	s.ddaClient = ddaClient

	s.pointIntakeAtFakeintake()
}

// buildProvisionerOptions deploys the operator (with the ComponentHealth
// controller enabled via Helm values) and no DatadogAgent; the suite applies the
// fault DatadogAgents itself. The fakeintake is provisioned by default.
func buildProvisionerOptions(local bool) []provisioners.KubernetesProvisionerOption {
	operatorOptions := []operatorparams.Option{
		operatorparams.WithNamespace(common.NamespaceName),
		operatorparams.WithOperatorFullImagePath(common.OperatorImageName),
		operatorparams.WithHelmValues(operatorHelmValues()),
	}
	return []provisioners.KubernetesProvisionerOption{
		provisioners.WithTestName("e2e-componenthealth"),
		provisioners.WithOperatorOptions(operatorOptions...),
		provisioners.WithoutDDA(),
		provisioners.WithLocal(local),
	}
}

// pointIntakeAtFakeintake patches the known operator container to enable the
// intake emitter and target the dynamically provisioned fakeintake, then waits
// for the updated Deployment to become available.
func (s *componentHealthSuite) pointIntakeAtFakeintake() {
	s.Require().NotNil(s.Env().FakeIntake, "fakeintake must be provisioned")
	url := strings.TrimRight(s.Env().FakeIntake.URL, "/") + agenthealthPath
	s.T().Logf("Pointing operator agenthealth intake at fakeintake: %s", url)

	deps := s.kubeClient.AppsV1().Deployments(common.NamespaceName)
	list, err := deps.List(s.T().Context(), metav1.ListOptions{LabelSelector: operatorDeploymentSelector})
	s.Require().NoError(err, "list operator deployment")
	s.Require().Len(list.Items, 1, "expected exactly one operator deployment")
	dep := &list.Items[0]
	s.Require().True(slices.ContainsFunc(dep.Spec.Template.Spec.Containers, func(c corev1.Container) bool {
		return c.Name == operatorContainerName
	}), "operator container not found in %s", dep.Name)

	patch, err := operatorIntakePatch(url)
	s.Require().NoError(err, "build operator intake patch")
	dep, err = deps.Patch(s.T().Context(), dep.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	s.Require().NoError(err, "patch operator deployment with intake env")

	s.Require().Eventuallyf(func() bool {
		current, err := deps.Get(s.T().Context(), dep.Name, metav1.GetOptions{})
		return err == nil && operatorRolloutAvailable(current, dep.Generation)
	}, 5*time.Minute, 10*time.Second, "operator deployment did not become available after the intake patch")
}

// operatorIntakePatch changes only the named container's intake env vars.
// Strategic merge preserves other containers and environment variables by name.
func operatorIntakePatch(url string) ([]byte, error) {
	base := &appsv1.Deployment{}
	updated := base.DeepCopy()
	updated.Spec.Template.Spec.Containers = []corev1.Container{{
		Name: operatorContainerName,
		Env: []corev1.EnvVar{
			{Name: envIntakeEnabled, Value: "true"},
			{Name: envIntakeURL, Value: url},
		},
	}}
	return client.StrategicMergeFrom(base).Data(updated)
}

// operatorRolloutAvailable requires availability of the updated replicas, so an
// old available pod cannot satisfy the wait while the new pod is still starting.
func operatorRolloutAvailable(dep *appsv1.Deployment, generation int64) bool {
	replicas := ptr.Deref(dep.Spec.Replicas, 1)
	return replicas > 0 &&
		dep.Status.ObservedGeneration >= generation &&
		dep.Status.ObservedGeneration >= dep.Generation &&
		dep.Status.UpdatedReplicas == replicas &&
		dep.Status.Replicas == replicas &&
		dep.Status.AvailableReplicas == replicas
}

// registerDatadogResourceCleanup deletes the DatadogAgent/DatadogAgentInternal
// CRs while the operator is still running, so CRD teardown during Pulumi destroy
// does not deadlock on operator finalizers (mirrors the untaint suite).
func (s *componentHealthSuite) registerDatadogResourceCleanup() {
	s.T().Cleanup(func() {
		k8sConfig := s.Env().KubernetesCluster.KubernetesClient.K8sConfig
		if k8sConfig == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := utils.DeleteAllDatadogResources(ctx, k8sConfig, common.NamespaceName); err != nil {
			s.T().Logf("Warning: failed to delete Datadog resources during cleanup: %v", err)
		}
	})
}

// applyDatadogAgent creates the test DatadogAgent or patches its Cluster Agent
// override. A nil override removes the previous fault while preserving defaults
// and other fields populated by the operator.
func (s *componentHealthSuite) applyDatadogAgent(override *v2alpha1.DatadogAgentComponentOverride) {
	ctx := s.T().Context()
	dda := &v2alpha1.DatadogAgent{}
	err := s.ddaClient.Get(ctx, types.NamespacedName{Name: ddaName, Namespace: common.NamespaceName}, dda)
	if apierrors.IsNotFound(err) {
		dda = &v2alpha1.DatadogAgent{
			ObjectMeta: metav1.ObjectMeta{Name: ddaName, Namespace: common.NamespaceName},
			Spec: v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Registry:    ptr.To("public.ecr.aws/datadog"),
					Kubelet:     &v2alpha1.KubeletConfig{TLSVerify: ptr.To(false)},
					Credentials: &v2alpha1.DatadogCredentials{APIKey: ptr.To(dummyAPIKey), AppKey: ptr.To(dummyAppKey)},
				},
			},
		}
		if override != nil {
			dda.Spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.ClusterAgentComponentName: override,
			}
		}
		s.Require().NoError(s.ddaClient.Create(ctx, dda), "create DatadogAgent")
		return
	}
	s.Require().NoError(err, "get DatadogAgent")

	before := dda.DeepCopy()
	delete(dda.Spec.Override, v2alpha1.ClusterAgentComponentName)
	if override != nil {
		if dda.Spec.Override == nil {
			dda.Spec.Override = make(map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride)
		}
		dda.Spec.Override[v2alpha1.ClusterAgentComponentName] = override
	}
	s.Require().NoError(s.ddaClient.Patch(ctx, dda, client.MergeFrom(before)), "patch DatadogAgent override")
}

// clusterAgentImagePullFault pins the Cluster Agent image to a nonexistent tag
// (ImagePullBackOff -> image_pull_failure).
func clusterAgentImagePullFault() *v2alpha1.DatadogAgentComponentOverride {
	return &v2alpha1.DatadogAgentComponentOverride{
		Image: &v2alpha1.AgentImageConfig{
			Name: "cluster-agent", Tag: imagePullBadTag, PullPolicy: ptr.To(corev1.PullAlways),
		},
	}
}

// clusterAgentUnschedulableFault pins the Cluster Agent to a nodeSelector no node
// satisfies (Pending/Unschedulable -> component_unschedulable).
func clusterAgentUnschedulableFault() *v2alpha1.DatadogAgentComponentOverride {
	return &v2alpha1.DatadogAgentComponentOverride{
		NodeSelector: map[string]string{"componenthealth.datadoghq.com/no-such-node": "true"},
	}
}

// waitForIssueState polls the fakeintake until a HealthReport issue of issueType
// is present with the given lifecycle state, failing with msg on timeout.
func (s *componentHealthSuite) waitForIssueState(issueType string, state healthplatform.IssueState, timeout time.Duration, msg string) {
	s.Require().EventuallyWithTf(func(c *assert.CollectT) {
		payloads, err := s.Env().FakeIntake.Client().GetAgentHealth()
		if !assert.NoError(c, err, "GetAgentHealth") {
			return
		}
		found := false
		for _, p := range payloads {
			if p == nil {
				continue
			}
			for _, iss := range p.GetIssues() {
				if iss.GetIssueType() == issueType && iss.GetPersistedIssue().GetState() == state {
					found = true
				}
			}
		}
		assert.Truef(c, found, "no %q issue with state %s in fakeintake yet", issueType, state)
	}, timeout, pollInterval, "%s", msg)
}
