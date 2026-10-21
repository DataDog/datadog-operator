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
	// operatorPodSelector selects the operator manager pod/deployment.
	operatorPodSelector = "app.kubernetes.io/name=datadog-operator"

	// agenthealthPath is the fakeintake route that captures agent-health payloads
	// (matches the operator's agenthealthPath and fakeintake's agentHealthEndpoint).
	agenthealthPath = "/api/v2/agenthealth"

	// Env the suite injects onto the operator to redirect the intake at the
	// fakeintake once its (dynamic) URL is known.
	envDetectEnabled = "DD_COMPONENT_HEALTH_ENABLED"
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

	s.waitForOperator()
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

// waitForOperator waits until the operator pod is Running.
func (s *componentHealthSuite) waitForOperator() {
	s.Require().Eventuallyf(func() bool {
		pods, err := s.kubeClient.CoreV1().Pods(common.NamespaceName).List(s.T().Context(), metav1.ListOptions{
			LabelSelector: operatorPodSelector,
			FieldSelector: "status.phase=Running",
		})
		return err == nil && len(pods.Items) >= 1
	}, 5*time.Minute, 10*time.Second, "operator pod did not reach Running")
}

// pointIntakeAtFakeintake patches the operator Deployment to enable the intake
// emitter and target the fakeintake, then waits for the new pod to roll out. The
// fakeintake URL is only known after provisioning, so it is injected here rather
// than baked into the static Helm values.
func (s *componentHealthSuite) pointIntakeAtFakeintake() {
	s.Require().NotNil(s.Env().FakeIntake, "fakeintake must be provisioned")
	url := strings.TrimRight(s.Env().FakeIntake.URL, "/") + agenthealthPath
	s.T().Logf("Pointing operator agenthealth intake at fakeintake: %s", url)

	deps := s.kubeClient.AppsV1().Deployments(common.NamespaceName)
	list, err := deps.List(s.T().Context(), metav1.ListOptions{LabelSelector: operatorPodSelector})
	s.Require().NoError(err, "list operator deployment")
	s.Require().NotEmpty(list.Items, "operator deployment not found")
	dep := &list.Items[0]

	// Find the operator container (the one carrying DD_COMPONENT_HEALTH_ENABLED
	// from the Helm values) and add the intake env to it.
	ci := operatorContainerIndex(dep)
	s.Require().GreaterOrEqual(ci, 0, "operator container (with %s) not found in %s", envDetectEnabled, dep.Name)
	setEnv(&dep.Spec.Template.Spec.Containers[ci], envIntakeEnabled, "true")
	setEnv(&dep.Spec.Template.Spec.Containers[ci], envIntakeURL, url)

	_, err = deps.Update(s.T().Context(), dep, metav1.UpdateOptions{})
	s.Require().NoError(err, "update operator deployment with intake env")

	// Wait for the rolled-out pod (one carrying the new intake URL env) to run.
	s.Require().Eventuallyf(func() bool {
		pods, err := s.kubeClient.CoreV1().Pods(common.NamespaceName).List(s.T().Context(), metav1.ListOptions{
			LabelSelector: operatorPodSelector,
			FieldSelector: "status.phase=Running",
		})
		if err != nil {
			return false
		}
		for i := range pods.Items {
			if podHasEnv(&pods.Items[i], envIntakeURL) {
				return true
			}
		}
		return false
	}, 5*time.Minute, 10*time.Second, "operator did not roll out with the intake URL env")
}

// operatorContainerIndex returns the index of the container carrying the
// component-health detection env, or -1.
func operatorContainerIndex(dep *appsv1.Deployment) int {
	for i := range dep.Spec.Template.Spec.Containers {
		for _, e := range dep.Spec.Template.Spec.Containers[i].Env {
			if e.Name == envDetectEnabled {
				return i
			}
		}
	}
	return -1
}

// setEnv sets (or replaces) an env var on a container.
func setEnv(c *corev1.Container, name, value string) {
	for i := range c.Env {
		if c.Env[i].Name == name {
			c.Env[i].Value = value
			c.Env[i].ValueFrom = nil
			return
		}
	}
	c.Env = append(c.Env, corev1.EnvVar{Name: name, Value: value})
}

// podHasEnv reports whether any container in the pod declares the given env var.
func podHasEnv(pod *corev1.Pod, name string) bool {
	for i := range pod.Spec.Containers {
		for _, e := range pod.Spec.Containers[i].Env {
			if e.Name == name {
				return true
			}
		}
	}
	return false
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
