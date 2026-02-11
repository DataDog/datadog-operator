// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogstatsdsuite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/operatorparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-operator/test/e2e/common"
	"github.com/DataDog/datadog-operator/test/e2e/provisioners"
	"github.com/DataDog/datadog-operator/test/e2e/tests/utils"
)

const (
	coreAgentContainerName = "agent"
	adpContainerName       = "agent-data-plane"
	dsdSocketVolumeName    = "dsdsocket"
	dsdSocketMountPath     = "/var/run/datadog"
	dsdSocketHostPath      = "/var/run/datadog"
	dsdPort                = int32(8125)
)

type dogstatsdSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
	local bool
}

func (s *dogstatsdSuite) TestDogstatsd() {
	defaultOperatorOpts := []operatorparams.Option{
		operatorparams.WithNamespace(common.NamespaceName),
		operatorparams.WithOperatorFullImagePath(common.OperatorImageName),
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

	t := s.T()
	var lastTestName string
	updateEnv := func(testName string, opts []provisioners.KubernetesProvisionerOption) {
		lastTestName = testName
		s.UpdateEnv(provisioners.KubernetesProvisioner(opts...))
	}
	t.Cleanup(func() {
		if lastTestName == "" {
			return
		}
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

	// --- Subtest 1: DSD UDP, ADP disabled ---
	s.T().Run("DSD UDP without ADP", func(t *testing.T) {
		// Deploy without DDA first to avoid host port binding races
		withoutDDAOpts := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-dsd-udp"),
			provisioners.WithoutDDA(),
		}
		withoutDDAOpts = append(withoutDDAOpts, defaultProvisionerOpts...)
		updateEnv("e2e-operator-dsd-udp", withoutDDAOpts)

		ddaConfigPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "dogstatsd", "datadog-agent-dsd-udp.yaml"))
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
		}
		provisionerOpts = append(provisionerOpts, defaultProvisionerOpts...)
		updateEnv("e2e-operator-dsd-udp", provisionerOpts)

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
	})

	// --- Subtest 2: DSD UDP, ADP enabled ---
	s.T().Run("DSD UDP with ADP", func(t *testing.T) {
		// Deploy without DDA first to avoid host port binding races
		withoutDDAOpts := []provisioners.KubernetesProvisionerOption{
			provisioners.WithTestName("e2e-operator-dsd-udp-adp"),
			provisioners.WithoutDDA(),
		}
		withoutDDAOpts = append(withoutDDAOpts, defaultProvisionerOpts...)
		updateEnv("e2e-operator-dsd-udp-adp", withoutDDAOpts)

		ddaConfigPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "dogstatsd", "datadog-agent-dsd-udp-adp.yaml"))
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
		}
		provisionerOpts = append(provisionerOpts, defaultProvisionerOpts...)
		updateEnv("e2e-operator-dsd-udp-adp", provisionerOpts)

		agentSelector := common.NodeAgentSelector + ",agent.datadoghq.com/name=dda-dsd-udp-adp"

		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), agentSelector)

			pods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(
				context.TODO(), metav1.ListOptions{LabelSelector: agentSelector, FieldSelector: "status.phase=Running"})
			assert.NoError(c, err)

			for _, pod := range pods.Items {
				assertContainerPresent(c, pod, adpContainerName)
				assertContainerHasUDPHostPort(c, pod, adpContainerName, dsdPort)
				assertContainerHasEnvVar(c, pod, coreAgentContainerName, "DD_USE_DOGSTATSD", "false")
				assertContainerDoesNotHaveHostPort(c, pod, coreAgentContainerName, dsdPort)
			}
		}, 5*time.Minute, 15*time.Second, "DSD UDP with ADP: pod spec verification failed")
	})

	// --- Subtest 3: DSD UDS, ADP disabled ---
	s.T().Run("DSD UDS without ADP", func(t *testing.T) {
		ddaConfigPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "dogstatsd", "datadog-agent-dsd-uds.yaml"))
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
		}
		provisionerOpts = append(provisionerOpts, defaultProvisionerOpts...)
		updateEnv("e2e-operator-dsd-uds", provisionerOpts)

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
	})

	// --- Subtest 4: DSD UDS, ADP enabled ---
	s.T().Run("DSD UDS with ADP", func(t *testing.T) {
		ddaConfigPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "dogstatsd", "datadog-agent-dsd-uds-adp.yaml"))
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
		}
		provisionerOpts = append(provisionerOpts, defaultProvisionerOpts...)
		updateEnv("e2e-operator-dsd-uds-adp", provisionerOpts)

		agentSelector := common.NodeAgentSelector + ",agent.datadoghq.com/name=dda-dsd-uds-adp"

		s.Assert().EventuallyWithTf(func(c *assert.CollectT) {
			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), agentSelector)

			pods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(
				context.TODO(), metav1.ListOptions{LabelSelector: agentSelector, FieldSelector: "status.phase=Running"})
			assert.NoError(c, err)

			for _, pod := range pods.Items {
				assertContainerPresent(c, pod, adpContainerName)
				assertContainerHasVolumeMount(c, pod, adpContainerName, dsdSocketVolumeName, dsdSocketMountPath)
				assertContainerHasEnvVar(c, pod, coreAgentContainerName, "DD_USE_DOGSTATSD", "false")
			}
		}, 5*time.Minute, 15*time.Second, "DSD UDS with ADP: pod spec verification failed")
	})
}

// --- Assertion helpers ---

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
