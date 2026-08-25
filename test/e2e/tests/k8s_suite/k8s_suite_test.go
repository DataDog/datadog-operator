// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package k8ssuite

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-operator/test/e2e/common"
	"github.com/DataDog/datadog-operator/test/e2e/provisioners"
	"github.com/DataDog/datadog-operator/test/e2e/tests/utils"

	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/operatorparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

var (
	matchTags = []*regexp.Regexp{regexp.MustCompile("kube_container_name:.*")}
	matchOpts = []client.MatchOpt[*aggregator.MetricSeries]{client.WithMatchingTags[*aggregator.MetricSeries](matchTags)}
)

const (
	coreAgentContainerName = "agent"
	adpContainerName       = "agent-data-plane"
	dsdSocketVolumeName    = "dsdsocket"
	dsdSocketMountPath     = "/var/run/datadog"
	dsdSocketHostPath      = "/var/run/datadog"
	dsdPort                = int32(8125)
)

func agentStatusCommand(args ...string) []string {
	command := []string{"env", "DD_LOG_LEVEL=off", "agent", "status"}
	return append(command, args...)
}

type k8sSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
	local bool
}

func (s *k8sSuite) TestGenericK8s() {
	defaultOperatorOpts := []operatorparams.Option{
		operatorparams.WithNamespace(common.NamespaceName),
		operatorparams.WithOperatorFullImagePath(common.OperatorImageName),
		// RBAC/CRDs are installed via our e2e kustomize (`config/new-e2e`, namePrefix: datadog-operator-e2e-).
		// Ensure the Helm-installed operator uses the same ServiceAccount (and doesn't create its own RBAC),
		// otherwise it may run under a different SA (e.g. datadog-operator-linux) lacking new permissions.
		operatorparams.WithHelmValues(`installCRDs: false
rbac:
  create: false
serviceAccount:
  create: false
  name: datadog-operator-e2e-controller-manager
`),
	}

	defaultProvisionerOpts := []provisioners.KubernetesProvisionerOption{
		provisioners.WithK8sVersion(common.K8sVersion),
		provisioners.WithOperatorOptions(defaultOperatorOpts...),
		provisioners.WithLocal(s.local),
	}

	defaultDDAOpts := []agentwithoperatorparams.Option{
		agentwithoperatorparams.WithNamespace(common.NamespaceName),
	}

	// --- Suite-level cleanup (registered before any subtests run) ---
	//
	// We need to ensure the final env of the suite is left without a DatadogAgent before the
	// underlying Pulumi teardown happens; otherwise CRD deletion may race with DDA deletion.
	//
	// This runs ONCE, at the very end of the whole suite (not after each subtest).
	t := s.T()
	var lastTestName string
	updateEnv := func(testName string, opts []provisioners.KubernetesProvisionerOption) {
		lastTestName = testName
		s.UpdateEnv(provisioners.KubernetesProvisioner(opts...))
	}
	// applyDDA tears down any in-stack DatadogAgent before installing the new
	// one. Used instead of updateEnv whenever a subtest applies a DDA.
	//
	// Why the explicit two-step swap: applying a new DDA on top of a previous
	// one does delete+create concurrently, which can leave the new agent
	// DaemonSet pod stuck on resources still owned by the previous DDA. The
	// most visible cases are:
	//   - K8s <1.20: the legacy SA-token controller can't keep up with the SA
	//     delete+create churn during the swap, so the new agent pod sits in
	//     FailedMount on its auto-generated <sa>-token-<rand> Secret;
	//   - host-port subtests (APM hostPort, DSD UDP): the previous agent pod
	//     hasn't released the port yet when the new pod tries to bind.
	// Both manifest as "agent pod never reaches Running" on the new DDA.
	applyDDA := func(testName string, opts []provisioners.KubernetesProvisionerOption) {
		cleanupOpts := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName(testName),
			provisioners.WithoutDDA(),
		}
		cleanupOpts = append(cleanupOpts, defaultProvisionerOpts...)
		updateEnv(testName, cleanupOpts)
		updateEnv(testName, opts)
	}
	t.Cleanup(func() {
		if lastTestName == "" {
			return
		}

		// Delete all Datadog custom resources before Pulumi stack destroy
		// to avoid CRD deletion timeout due to finalizers.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		if k8sConfig := s.Env().KubernetesCluster.KubernetesClient.K8sConfig; k8sConfig != nil {
			if err := utils.DeleteAllDatadogResources(ctx, k8sConfig, common.NamespaceName); err != nil {
				t.Logf("Warning: failed to delete Datadog resources during cleanup: %v", err)
			}
		}
	})

	s.T().Run("Verify Operator", func(t *testing.T) {
		s.Assert().EventuallyWithT(func(c *assert.CollectT) {
			utils.VerifyOperator(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client())
		}, 300*time.Second, 15*time.Second, "Could not validate operator pod in time")
	})

	s.T().Run("Minimal DDA config", func(t *testing.T) {
		ddaConfigPath, err := common.GetAbsPath(common.DdaMinimalPath)
		assert.NoError(s.T(), err)

		ddaOpts := []agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
				Name:         "dda-minimum",
				YamlFilePath: ddaConfigPath,
			}),
		}
		ddaOpts = append(ddaOpts, defaultDDAOpts...)

		provisionerOptions := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-minimal-dda"),
			provisioners.WithK8sVersion(common.K8sVersion),
			provisioners.WithOperatorOptions(defaultOperatorOpts...),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithLocal(s.local),
		}

		applyDDA("e2e-operator-minimal-dda", provisionerOptions)

		err = s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
		s.Assert().NoError(err)

		s.Assert().EventuallyWithT(func(c *assert.CollectT) {
			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), common.NodeAgentSelector+",agent.datadoghq.com/name=dda-minimum")
			utils.VerifyNumPodsForSelector(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), 1, common.ClusterAgentSelector+",agent.datadoghq.com/name=dda-minimum")

			agentPods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(context.TODO(), metav1.ListOptions{LabelSelector: common.NodeAgentSelector + ",agent.datadoghq.com/name=dda-minimum",
				FieldSelector: "status.phase=Running"})
			assert.NoError(s.T(), err)

			for _, pod := range agentPods.Items {
				output, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(common.NamespaceName, pod.Name, "agent", agentStatusCommand("collector", "-j"))
				assert.NoError(c, err)
				utils.VerifyCheck(c, output, "kubelet")
			}

			metricNames, err := s.Env().FakeIntake.Client().GetMetricNames()
			s.Assert().NoError(err)
			assert.Contains(c, metricNames, "kubernetes.cpu.usage.total")

			metrics, err := s.Env().FakeIntake.Client().FilterMetrics("kubernetes.cpu.usage.total", matchOpts...)
			s.Assert().NoError(err)

			for _, metric := range metrics {
				for _, points := range metric.Points {
					s.Assert().Greater(points.Value, float64(0))
				}
			}

			clusterAgentPods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(context.TODO(), metav1.ListOptions{LabelSelector: common.ClusterAgentSelector + ",agent.datadoghq.com/e2e-test=datadog-agent-minimum"})
			assert.NoError(s.T(), err)

			for _, pod := range clusterAgentPods.Items {
				output, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(common.NamespaceName, pod.Name, "agent", agentStatusCommand("collector", "-j"))
				assert.NoError(c, err)
				utils.VerifyCheck(c, output, "kubernetes_state_core")
			}

			s.verifyKSMCheck(c)
		}, 10*time.Minute, 30*time.Second, "could not validate KSM (cluster check) metrics in time")

	})

	s.T().Run("KSM check works cluster check runner", func(t *testing.T) {
		ddaConfigPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "datadog-agent-ccr-enabled.yaml"))
		assert.NoError(s.T(), err)

		ddaOpts := []agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
				Name:         "datadog-ccr-enabled",
				YamlFilePath: ddaConfigPath,
			}),
		}
		ddaOpts = append(ddaOpts, defaultDDAOpts...)

		provisionerOptions := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-ksm-ccr"),
			provisioners.WithK8sVersion(common.K8sVersion),
			provisioners.WithOperatorOptions(defaultOperatorOpts...),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithLocal(s.local),
		}

		applyDDA("e2e-operator-ksm-ccr", provisionerOptions)

		err = s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
		s.Assert().NoError(err)

		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), "app.kubernetes.io/instance=datadog-ccr-enabled-agent")

			utils.VerifyNumPodsForSelector(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), 1, "app.kubernetes.io/instance=datadog-ccr-enabled-cluster-checks-runner")

			ccrPods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(context.TODO(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/instance=datadog-ccr-enabled-cluster-checks-runner"})
			assert.NoError(s.T(), err)

			for _, ccr := range ccrPods.Items {
				output, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(common.NamespaceName, ccr.Name, "agent", agentStatusCommand("collector", "-j"))
				assert.NoError(c, err)
				utils.VerifyCheck(c, output, "kubernetes_state_core")
			}

			s.verifyKSMCheck(c, "kubernetes_state_customresource.uptodateagents")
		}, 15*time.Minute, 15*time.Second, "could not validate kubernetes_state_core (cluster check on CCR) check in time")
	})

	s.T().Run("Autodiscovery works", func(t *testing.T) {
		ddaConfigPath, err := common.GetAbsPath(common.DdaMinimalPath)
		assert.NoError(s.T(), err)

		ddaOpts := []agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{Name: "dda-autodiscovery", YamlFilePath: ddaConfigPath}),
		}
		ddaOpts = append(ddaOpts, defaultDDAOpts...)

		provisionerOptions := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-autodiscovery"),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithYAMLWorkload(provisioners.YAMLWorkload{Name: "nginx", Path: strings.Join([]string{common.ManifestsPath, "autodiscovery-annotation.yaml"}, "/")}),
			provisioners.WithLocal(s.local),
		}
		provisionerOptions = append(provisionerOptions, defaultProvisionerOpts...)

		// Add nginx with annotations
		applyDDA("e2e-operator-autodiscovery", provisionerOptions)

		err = s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
		s.Assert().NoError(err)

		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			utils.VerifyNumPodsForSelector(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), 1, "app=nginx")

			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), common.NodeAgentSelector+",agent.datadoghq.com/name=dda-autodiscovery")

			// check agent pods for http check
			agentPods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(context.TODO(), metav1.ListOptions{LabelSelector: common.NodeAgentSelector + ",agent.datadoghq.com/name=dda-autodiscovery",
				FieldSelector: "status.phase=Running"})
			assert.NoError(c, err)

			for _, pod := range agentPods.Items {
				output, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(common.NamespaceName, pod.Name, "agent", agentStatusCommand("collector", "-j"))
				assert.NoError(c, err)

				utils.VerifyCheck(c, output, "http_check")
			}

			s.verifyHTTPCheck(c)
		}, 900*time.Second, 15*time.Second, "could not validate http_check in time")
	})

	s.T().Run("Logs collection works", func(t *testing.T) {
		ddaConfigPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "datadog-agent-logs.yaml"))
		assert.NoError(s.T(), err)

		ddaOpts := []agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
				Name:         "datadog-agent-logs",
				YamlFilePath: ddaConfigPath,
			}),
		}
		ddaOpts = append(ddaOpts, defaultDDAOpts...)

		provisionerOptions := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-logs-collection"),
			provisioners.WithK8sVersion(common.K8sVersion),
			provisioners.WithOperatorOptions(defaultOperatorOpts...),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithLocal(s.local),
		}

		applyDDA("e2e-operator-logs-collection", provisionerOptions)

		err = s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
		s.Assert().NoError(err)

		// Verify logs collection on agent pod
		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), "app.kubernetes.io/instance=datadog-agent-logs-agent")

			agentPods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(context.TODO(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/instance=datadog-agent-logs-agent"})
			assert.NoError(c, err)

			for _, pod := range agentPods.Items {
				output, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(common.NamespaceName, pod.Name, "agent", agentStatusCommand("logs agent", "-j"))
				assert.NoError(c, err)
				utils.VerifyAgentPodLogs(c, output)
			}

			s.verifyAPILogs(c)
		}, 900*time.Second, 15*time.Second, "could not validate logs collection in time")
	})

	s.T().Run("APM hostPort k8s service UDP works", func(t *testing.T) {
		var apmAgentSelector = ",agent.datadoghq.com/name=datadog-agent-apm"
		ddaConfigPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "apm", "datadog-agent-apm.yaml"))
		assert.NoError(s.T(), err)

		ddaOpts := []agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
				Name:         "datadog-agent-apm",
				YamlFilePath: ddaConfigPath,
			}),
		}
		ddaOpts = append(ddaOpts, defaultDDAOpts...)

		ddaProvisionerOptions := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-apm"),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithYAMLWorkload(provisioners.YAMLWorkload{
				Name: "tracegen-deploy",
				Path: strings.Join([]string{common.ManifestsPath, "apm", "tracegen-deploy.yaml"}, "/"),
			}),
			provisioners.WithLocal(s.local),
		}
		ddaProvisionerOptions = append(ddaProvisionerOptions, defaultProvisionerOpts...)

		// Deploy APM DatadogAgent and tracegen
		applyDDA("e2e-operator-apm", ddaProvisionerOptions)

		// Verify traces collection on agent pod
		s.EventuallyWithTf(func(c *assert.CollectT) {
			// Verify tracegen deployment is running
			utils.VerifyNumPodsForSelector(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), 1, "app=tracegen-tribrid")

			// Verify agent pods are running
			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), common.NodeAgentSelector+apmAgentSelector)
			agentPods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(context.TODO(), metav1.ListOptions{LabelSelector: common.NodeAgentSelector + apmAgentSelector, FieldSelector: "status.phase=Running"})
			assert.NoError(c, err)

			// This works because we have a single Agent pod (so located on same node as tracegen)
			// Otherwise, we would need to deploy tracegen on the same node as the Agent pod / as a DaemonSet
			for _, pod := range agentPods.Items {
				output, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(common.NamespaceName, pod.Name, "agent", agentStatusCommand("apm agent", "-j"))
				assert.NoError(c, err)

				utils.VerifyAgentTraces(c, output)
			}

			// Verify traces collection ingestion by fakeintake
			s.verifyAPITraces(c)
		}, 600*time.Second, 15*time.Second, "could not validate traces on agent pod") // TODO: check duration
	})

	// --- DogStatsD subtests ---

	// --- Subtest: DSD UDP, ADP disabled ---
	s.T().Run("DSD UDP without ADP", func(t *testing.T) {
		ddaConfigPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "dogstatsd", "datadog-agent-dsd-udp.yaml"))
		assert.NoError(s.T(), err)
		senderPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "dogstatsd", "dsd-udp-sender.yaml"))
		assert.NoError(s.T(), err)

		ddaOpts := append([]agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
				Name:         "dda-dsd-udp",
				YamlFilePath: ddaConfigPath,
			}),
		}, defaultDDAOpts...)

		provisionerOpts := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-dsd-udp"),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithYAMLWorkload(provisioners.YAMLWorkload{Name: "dsd-udp-sender", Path: senderPath}),
		}
		provisionerOpts = append(provisionerOpts, defaultProvisionerOpts...)
		applyDDA("e2e-operator-dsd-udp", provisionerOpts)

		err = s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
		s.Assert().NoError(err)

		agentSelector := common.NodeAgentSelector + ",agent.datadoghq.com/name=dda-dsd-udp"

		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), agentSelector)

			pods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(
				context.TODO(), metav1.ListOptions{LabelSelector: agentSelector, FieldSelector: "status.phase=Running"})
			assert.NoError(c, err)

			for _, pod := range pods.Items {
				assertContainerAbsent(c, pod, adpContainerName)
				assertContainerHasUDPHostPort(c, pod, coreAgentContainerName, dsdPort)
			}
		}, 5*time.Minute, 15*time.Second, "DSD UDP without ADP: pod spec verification failed")

		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			s.verifyDSDMetrics(c, "e2e.dsd.udp.counter")
		}, 3*time.Minute, 10*time.Second, "DSD UDP without ADP: metrics not received by fakeintake")
	})

	// --- Subtest: DSD UDP, ADP enabled ---
	s.T().Run("DSD UDP with ADP", func(t *testing.T) {
		ddaConfigPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "dogstatsd", "datadog-agent-dsd-udp-adp.yaml"))
		assert.NoError(s.T(), err)
		senderPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "dogstatsd", "dsd-udp-sender.yaml"))
		assert.NoError(s.T(), err)

		ddaOpts := append([]agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
				Name:         "dda-dsd-udp-adp",
				YamlFilePath: ddaConfigPath,
			}),
		}, defaultDDAOpts...)

		provisionerOpts := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-dsd-udp-adp"),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithYAMLWorkload(provisioners.YAMLWorkload{Name: "dsd-udp-sender", Path: senderPath}),
		}
		provisionerOpts = append(provisionerOpts, defaultProvisionerOpts...)
		applyDDA("e2e-operator-dsd-udp-adp", provisionerOpts)

		err = s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
		s.Assert().NoError(err)

		agentSelector := common.NodeAgentSelector + ",agent.datadoghq.com/name=dda-dsd-udp-adp"

		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), agentSelector)

			pods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(
				context.TODO(), metav1.ListOptions{LabelSelector: agentSelector, FieldSelector: "status.phase=Running"})
			assert.NoError(c, err)

			for _, pod := range pods.Items {
				assertContainerPresent(c, pod, adpContainerName)
				assertContainerHasUDPHostPort(c, pod, adpContainerName, dsdPort)
				assertContainerDoesNotHaveHostPort(c, pod, coreAgentContainerName, dsdPort)
			}
		}, 5*time.Minute, 15*time.Second, "DSD UDP with ADP: pod spec verification failed")

		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			s.verifyDSDMetrics(c, "e2e.dsd.udp.counter")
		}, 3*time.Minute, 10*time.Second, "DSD UDP with ADP: metrics not received by fakeintake")
	})

	// --- Subtest: DSD UDP, single-container ADP enabled by the Operator default ---
	s.T().Run("Single-container DSD UDP uses the Operator ADP default", func(t *testing.T) {
		ddaConfigPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "dogstatsd", "datadog-agent-dsd-udp-single-adp-default.yaml"))
		assert.NoError(s.T(), err)
		senderPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "dogstatsd", "dsd-udp-sender.yaml"))
		assert.NoError(s.T(), err)

		ddaOpts := append([]agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
				Name:         "dda-dsd-udp-single-adp-default",
				YamlFilePath: ddaConfigPath,
			}),
		}, defaultDDAOpts...)

		operatorOpts := append([]operatorparams.Option{}, defaultOperatorOpts...)
		operatorOpts = append(operatorOpts, operatorparams.WithHelmValues(`env:
  - name: DD_DEFAULT_DATA_PLANE_LINUX_ENABLED
    value: "true"
`))

		provisionerOpts := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-dsd-udp-single-adp-default"),
			provisioners.WithOperatorOptions(operatorOpts...),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithYAMLWorkload(provisioners.YAMLWorkload{Name: "dsd-udp-sender", Path: senderPath}),
			provisioners.WithLocal(s.local),
		}
		applyDDA("e2e-operator-dsd-udp-single-adp-default", provisionerOpts)

		err = s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
		s.Assert().NoError(err)

		agentSelector := common.NodeAgentSelector + ",agent.datadoghq.com/name=dda-dsd-udp-single-adp-default"

		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), agentSelector)

			pods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(
				context.TODO(), metav1.ListOptions{LabelSelector: agentSelector})
			if !assert.NoError(c, err) {
				return
			}
			if !assert.Len(c, pods.Items, 1, "expected one single-container Agent pod") {
				for _, pod := range pods.Items {
					assert.Failf(c, "unexpected Agent pod count", "pod %s diagnostics:\n%s", pod.Name, s.singleContainerADPDiagnostics(pod))
				}
				return
			}

			pod := pods.Items[0]
			if pod.Status.Phase != corev1.PodRunning {
				assert.Failf(c, "Agent pod is not running", "pod %s is %s:\n%s", pod.Name, pod.Status.Phase, s.singleContainerADPDiagnostics(pod))
				return
			}
			s.assertSingleContainerADPRuntime(c, pod)
		}, 5*time.Minute, 15*time.Second, "single-container DSD UDP ADP default runtime verification failed")

		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			s.assertDSDMetricsWithRuntimeDiagnostics(c, agentSelector, "e2e.dsd.udp.counter")
		}, 3*time.Minute, 10*time.Second, "single-container DSD UDP ADP default metrics not received by fakeintake")
	})

	// --- Subtest: DSD UDS, ADP disabled ---
	s.T().Run("DSD UDS without ADP", func(t *testing.T) {
		ddaConfigPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "dogstatsd", "datadog-agent-dsd-uds.yaml"))
		assert.NoError(s.T(), err)
		senderPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "dogstatsd", "dsd-uds-sender.yaml"))
		assert.NoError(s.T(), err)

		ddaOpts := append([]agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
				Name:         "dda-dsd-uds",
				YamlFilePath: ddaConfigPath,
			}),
		}, defaultDDAOpts...)

		provisionerOpts := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-dsd-uds"),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithYAMLWorkload(provisioners.YAMLWorkload{Name: "dsd-uds-sender", Path: senderPath}),
		}
		provisionerOpts = append(provisionerOpts, defaultProvisionerOpts...)
		applyDDA("e2e-operator-dsd-uds", provisionerOpts)

		err = s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
		s.Assert().NoError(err)

		agentSelector := common.NodeAgentSelector + ",agent.datadoghq.com/name=dda-dsd-uds"

		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), agentSelector)

			pods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(
				context.TODO(), metav1.ListOptions{LabelSelector: agentSelector, FieldSelector: "status.phase=Running"})
			assert.NoError(c, err)

			for _, pod := range pods.Items {
				assertContainerAbsent(c, pod, adpContainerName)
				assertContainerHasVolumeMount(c, pod, coreAgentContainerName, dsdSocketVolumeName, dsdSocketMountPath)
				assertPodHasHostPathVolume(c, pod, dsdSocketVolumeName, dsdSocketHostPath)
			}
		}, 5*time.Minute, 15*time.Second, "DSD UDS without ADP: pod spec verification failed")

		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			s.verifyDSDMetrics(c, "e2e.dsd.uds.counter")
		}, 3*time.Minute, 10*time.Second, "DSD UDS without ADP: metrics not received by fakeintake")
	})

	// --- Subtest: DSD UDS, ADP enabled ---
	s.T().Run("DSD UDS with ADP", func(t *testing.T) {
		ddaConfigPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "dogstatsd", "datadog-agent-dsd-uds-adp.yaml"))
		assert.NoError(s.T(), err)
		senderPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "dogstatsd", "dsd-uds-sender.yaml"))
		assert.NoError(s.T(), err)

		ddaOpts := append([]agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
				Name:         "dda-dsd-uds-adp",
				YamlFilePath: ddaConfigPath,
			}),
		}, defaultDDAOpts...)

		provisionerOpts := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-dsd-uds-adp"),
			provisioners.WithDDAOptions(ddaOpts...),
			provisioners.WithYAMLWorkload(provisioners.YAMLWorkload{Name: "dsd-uds-sender", Path: senderPath}),
		}
		provisionerOpts = append(provisionerOpts, defaultProvisionerOpts...)
		applyDDA("e2e-operator-dsd-uds-adp", provisionerOpts)

		err = s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
		s.Assert().NoError(err)

		agentSelector := common.NodeAgentSelector + ",agent.datadoghq.com/name=dda-dsd-uds-adp"

		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), agentSelector)

			pods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(
				context.TODO(), metav1.ListOptions{LabelSelector: agentSelector, FieldSelector: "status.phase=Running"})
			assert.NoError(c, err)

			for _, pod := range pods.Items {
				assertContainerPresent(c, pod, adpContainerName)
				assertContainerHasVolumeMount(c, pod, adpContainerName, dsdSocketVolumeName, dsdSocketMountPath)
			}
		}, 5*time.Minute, 15*time.Second, "DSD UDS with ADP: pod spec verification failed")

		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			s.verifyDSDMetrics(c, "e2e.dsd.uds.counter")
		}, 3*time.Minute, 10*time.Second, "DSD UDS with ADP: metrics not received by fakeintake")
	})
}

func (s *k8sSuite) verifyDSDMetrics(c *assert.CollectT, metricName string) {
	metricNames, err := s.Env().FakeIntake.Client().GetMetricNames()
	assert.NoError(c, err)
	assert.Contains(c, metricNames, metricName)

	metrics, err := s.Env().FakeIntake.Client().FilterMetrics(metricName)
	assert.NoError(c, err)
	assert.NotEmptyf(c, metrics, "expected metric series for %s to be non-empty", metricName)
}

func (s *k8sSuite) verifyAPILogs(t assert.TestingT) {
	logs, err := s.Env().FakeIntake.Client().FilterLogs("agent")
	assert.NoError(t, err)
	assert.NotEmptyf(t, logs, "Expected fake intake-ingested logs to not be empty")
}

func (s *k8sSuite) verifyAPITraces(c *assert.CollectT) {
	traces, err := s.Env().FakeIntake.Client().GetTraces()
	assert.NoError(c, err)
	assert.NotEmptyf(c, traces, fmt.Sprintf("Expected fake intake-ingested traces to not be empty: %s", err))
}

func (s *k8sSuite) verifyKSMCheck(c *assert.CollectT, expectedMetrics ...string) {
	metricNames, err := s.Env().FakeIntake.Client().GetMetricNames()
	assert.NoError(c, err)
	assert.Contains(c, metricNames, "kubernetes_state.container.running")
	for _, metric := range expectedMetrics {
		assert.Contains(c, metricNames, metric)
	}

	metrics, err := s.Env().FakeIntake.Client().FilterMetrics("kubernetes_state.container.running", matchOpts...)
	assert.NoError(c, err)
	assert.NotEmptyf(c, metrics, fmt.Sprintf("expected metric series to not be empty: %s", err))
}

func (s *k8sSuite) verifyHTTPCheck(c *assert.CollectT) {
	metricNames, err := s.Env().FakeIntake.Client().GetMetricNames()
	assert.NoError(c, err)
	assert.Contains(c, metricNames, "network.http.can_connect")
	metrics, err := s.Env().FakeIntake.Client().FilterMetrics("network.http.can_connect")
	assert.NoError(c, err)
	assert.Greater(c, len(metrics), 0)
	for _, metric := range metrics {
		for _, points := range metric.Points {
			assert.Greater(c, points.Value, float64(0))
		}
	}
}

// --- DogStatsD assertion helpers ---

// findContainer returns the container with the given name, or nil if not found.
func findContainer(pod corev1.Pod, name string) *corev1.Container {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == name {
			return &pod.Spec.Containers[i]
		}
	}
	return nil
}

func assertContainerPresent(c *assert.CollectT, pod corev1.Pod, containerName string) {
	container := findContainer(pod, containerName)
	assert.NotNilf(c, container, "expected container %q to be present in pod %s", containerName, pod.Name)
}

func assertContainerAbsent(c *assert.CollectT, pod corev1.Pod, containerName string) {
	container := findContainer(pod, containerName)
	assert.Nilf(c, container, "expected container %q to be absent in pod %s", containerName, pod.Name)
}

func assertContainerHasUDPHostPort(c *assert.CollectT, pod corev1.Pod, containerName string, port int32) {
	container := findContainer(pod, containerName)
	if !assert.NotNilf(c, container, "container %q not found in pod %s", containerName, pod.Name) {
		return
	}
	for _, p := range container.Ports {
		if p.Protocol == corev1.ProtocolUDP && p.HostPort == port {
			return
		}
	}
	assert.Failf(c, "host port not found", "expected container %q in pod %s to have UDP host port %d", containerName, pod.Name, port)
}

func assertContainerDoesNotHaveHostPort(c *assert.CollectT, pod corev1.Pod, containerName string, port int32) {
	container := findContainer(pod, containerName)
	if container == nil {
		return // container absent means no host port
	}
	for _, p := range container.Ports {
		if p.Protocol == corev1.ProtocolUDP && p.HostPort == port {
			assert.Failf(c, "unexpected host port", "expected container %q in pod %s to NOT have UDP host port %d", containerName, pod.Name, port)
			return
		}
	}
}

func (s *k8sSuite) assertDSDMetricsWithRuntimeDiagnostics(c *assert.CollectT, agentSelector, metricName string) {
	var failures []string

	metricNames, err := s.Env().FakeIntake.Client().GetMetricNames()
	if err != nil {
		failures = append(failures, fmt.Sprintf("could not list fakeintake metrics: %v", err))
	} else if !containsString(metricNames, metricName) {
		failures = append(failures, fmt.Sprintf("fakeintake does not contain metric %q; available metrics: %s", metricName, strings.Join(metricNames, ", ")))
	}

	metrics, err := s.Env().FakeIntake.Client().FilterMetrics(metricName)
	if err != nil {
		failures = append(failures, fmt.Sprintf("could not filter fakeintake metric %q: %v", metricName, err))
	} else if len(metrics) == 0 {
		failures = append(failures, fmt.Sprintf("fakeintake metric %q has no series", metricName))
	}

	if len(failures) == 0 {
		return
	}

	pods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(
		context.TODO(), metav1.ListOptions{LabelSelector: agentSelector})
	if err != nil {
		failures = append(failures, fmt.Sprintf("could not list Agent pods for diagnostics: %v", err))
	} else {
		for _, pod := range pods.Items {
			failures = append(failures, fmt.Sprintf("pod %s diagnostics:\n%s", pod.Name, s.singleContainerADPDiagnostics(pod)))
		}
	}
	assert.Failf(c, "single-container DogStatsD metric verification failed", "%s", strings.Join(failures, "\n\n"))
}

func (s *k8sSuite) assertSingleContainerADPRuntime(c *assert.CollectT, pod corev1.Pod) {
	var failures []string

	if len(pod.Spec.Containers) != 1 {
		failures = append(failures, fmt.Sprintf("expected one Agent container, got %d", len(pod.Spec.Containers)))
	}

	container := findContainer(pod, "unprivileged-single-agent")
	if container == nil {
		failures = append(failures, "unprivileged-single-agent container is missing")
	} else {
		for _, env := range []string{
			"DD_DATA_PLANE_ENABLED",
			"DD_DATA_PLANE_DOGSTATSD_ENABLED",
			"DD_DATA_PLANE_REMOTE_AGENT_ENABLED",
			"DD_DATA_PLANE_USE_NEW_CONFIG_STREAM_ENDPOINT",
		} {
			value, found := containerEnvValue(*container, env)
			if !found || value != "true" {
				failures = append(failures, fmt.Sprintf("expected %s=true, got %q (present=%t)", env, value, found))
			}
		}
		if !containerHasUDPHostPort(*container, dsdPort) {
			failures = append(failures, fmt.Sprintf("unprivileged-single-agent does not expose UDP HostPort %d", dsdPort))
		}
	}

	socketOutput, socketErr, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(
		common.NamespaceName, pod.Name, "unprivileged-single-agent", []string{"sh", "-c", "ss -lunp 'sport = :8125'"})
	socketDetails := strings.TrimSpace(strings.Join([]string{socketOutput, socketErr}, "\n"))
	if err != nil {
		failures = append(failures, fmt.Sprintf("could not inspect UDP/8125 with ss: %v; output: %s", err, socketDetails))
	} else {
		if !strings.Contains(socketDetails, ":8125") {
			failures = append(failures, fmt.Sprintf("ss did not report UDP/8125: %s", socketDetails))
		}
		if !strings.Contains(socketDetails, "agent-data-plan") {
			failures = append(failures, fmt.Sprintf("ss did not identify agent-data-plane as the UDP/8125 owner: %s", socketDetails))
		}
	}

	if len(failures) > 0 {
		assert.Failf(c, "single-container ADP runtime verification failed", "%s\n\nDiagnostics:\n%s", strings.Join(failures, "\n"), s.singleContainerADPDiagnostics(pod))
	}
}

func (s *k8sSuite) singleContainerADPDiagnostics(pod corev1.Pod) string {
	var diagnostics []string

	podJSON, err := json.MarshalIndent(pod, "", "  ")
	if err != nil {
		diagnostics = append(diagnostics, fmt.Sprintf("could not marshal pod: %v", err))
	} else {
		diagnostics = append(diagnostics, "Pod:\n"+string(podJSON))
	}

	processOutput, processErr, processExecErr := s.Env().KubernetesCluster.KubernetesClient.PodExec(
		common.NamespaceName, pod.Name, "unprivileged-single-agent", []string{"sh", "-c", "ps -ef; printf '\\n--- UDP sockets ---\\n'; ss -lunp"})
	if processExecErr != nil {
		diagnostics = append(diagnostics, fmt.Sprintf("process/socket inspection failed: %v\n%s\n%s", processExecErr, processOutput, processErr))
	} else {
		diagnostics = append(diagnostics, "Processes and UDP sockets:\n"+strings.Join([]string{processOutput, processErr}, "\n"))
	}

	tailLines := int64(200)
	logs, logsErr := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).GetLogs(pod.Name, &corev1.PodLogOptions{
		Container: "unprivileged-single-agent",
		TailLines: &tailLines,
	}).DoRaw(context.Background())
	if logsErr != nil {
		diagnostics = append(diagnostics, fmt.Sprintf("could not fetch unprivileged-single-agent logs: %v", logsErr))
	} else {
		diagnostics = append(diagnostics, "unprivileged-single-agent logs:\n"+string(logs))
	}

	return strings.Join(diagnostics, "\n\n")
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containerEnvValue(container corev1.Container, name string) (string, bool) {
	for _, env := range container.Env {
		if env.Name == name {
			return env.Value, true
		}
	}
	return "", false
}

func containerHasUDPHostPort(container corev1.Container, port int32) bool {
	for _, containerPort := range container.Ports {
		if containerPort.Protocol == corev1.ProtocolUDP && containerPort.HostPort == port {
			return true
		}
	}
	return false
}

func assertContainerHasEnvVar(c *assert.CollectT, pod corev1.Pod, containerName, envName, envValue string) {
	container := findContainer(pod, containerName)
	if !assert.NotNilf(c, container, "container %q not found in pod %s", containerName, pod.Name) {
		return
	}
	for _, env := range container.Env {
		if env.Name == envName {
			assert.Equalf(c, envValue, env.Value, "env var %s in container %q of pod %s", envName, containerName, pod.Name)
			return
		}
	}
	assert.Failf(c, "env var not found", "expected container %q in pod %s to have env var %s=%s", containerName, pod.Name, envName, envValue)
}

func assertContainerHasVolumeMount(c *assert.CollectT, pod corev1.Pod, containerName, volumeName, mountPath string) {
	container := findContainer(pod, containerName)
	if !assert.NotNilf(c, container, "container %q not found in pod %s", containerName, pod.Name) {
		return
	}
	for _, vm := range container.VolumeMounts {
		if vm.Name == volumeName && vm.MountPath == mountPath {
			return
		}
	}
	assert.Failf(c, "volume mount not found", "expected container %q in pod %s to have volume mount %s at %s", containerName, pod.Name, volumeName, mountPath)
}

func assertPodHasHostPathVolume(c *assert.CollectT, pod corev1.Pod, volumeName, hostPath string) {
	for _, vol := range pod.Spec.Volumes {
		if vol.Name == volumeName {
			if !assert.NotNilf(c, vol.VolumeSource.HostPath, "volume %q in pod %s is not a HostPath volume", volumeName, pod.Name) {
				return
			}
			assert.Equalf(c, hostPath, vol.VolumeSource.HostPath.Path, "volume %q hostPath in pod %s", volumeName, pod.Name)
			return
		}
	}
	assert.Failf(c, "volume not found", "expected pod %s to have volume %s with hostPath %s", pod.Name, volumeName, hostPath)
}
