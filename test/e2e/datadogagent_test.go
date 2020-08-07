// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package e2e

import (
	"context"
	goctx "context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/DataDog/datadog-operator/pkg/apis"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/test/e2e/utils"

	framework "github.com/operator-framework/operator-sdk/pkg/test"
	"github.com/operator-framework/operator-sdk/pkg/test/e2eutil"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	dynclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	retryInterval        = time.Second * 5
	timeout              = time.Minute * 2
	cleanupRetryInterval = time.Second * 1
	cleanupTimeout       = time.Second * 60

	ddAPIKey = ""
)

func init() {
	ddAPIKey = os.Getenv("DD_API_KEY")
}

func TestDatadogAgent(t *testing.T) {
	datadogagentList := &datadoghqv1alpha1.DatadogAgentList{}
	err := framework.AddToFrameworkScheme(apis.AddToScheme, datadogagentList)
	if err != nil {
		t.Fatalf("failed to add custom resource scheme to framework: %v", err)
	}
	// run subtests
	t.Run("dd-group", func(t *testing.T) {
		t.Run("DeploymentDaemonset", DeploymentDaemonset)
		t.Run("DCADeployment", DeploymentWithClusterAgentEnabled)
		//t.Run("DeploymentEDS", DeploymentEDS)
	})
}

func DeploymentDaemonset(t *testing.T) {
	namespace, ctx, f := initTestFwkResources(t, "datadog-operator")
	var err error
	defer ctx.Cleanup()

	name := "foo"
	options := &utils.NewDatadogAgentOptions{
		UseEDS: false,
		APIKey: ddAPIKey,
	}

	agentdeployment := utils.NewDatadogAgent(namespace, name, fmt.Sprintf("datadog/agent:%s", "6.14.0"), options)
	err = f.Client.Create(goctx.TODO(), agentdeployment, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	if err != nil {
		exportPodsLogs(t, f, namespace, err)
		t.Fatal(err)
	}

	isOK := func(ad *datadoghqv1alpha1.DatadogAgent) (bool, error) {
		if ad.Status.Agent == nil {
			return false, nil
		}
		if ad.Status.Agent.CurrentHash == "" {
			return false, nil
		}
		for _, condition := range ad.Status.Conditions {
			if condition.Type == datadoghqv1alpha1.ConditionTypeActive && condition.Status == corev1.ConditionTrue {
				return true, nil
			}
		}

		return false, nil
	}
	err = utils.WaitForFuncOnDatadogAgent(t, f.Client, namespace, name, isOK, retryInterval, timeout)
	if err != nil {
		exportPodsLogs(t, f, namespace, err)
		t.Fatal(err)
	}

	// check if the Daemonset was created properly
	isDaemonsetOK := func(ds *appsv1.DaemonSet) (bool, error) {
		if ds.Status.NumberReady == ds.Status.DesiredNumberScheduled && ds.Status.DesiredNumberScheduled > 0 {
			return true, nil
		}
		t.Logf("status false %#v", ds.Status)
		return false, nil
	}
	dsName := fmt.Sprintf("%s-%s", name, "agent")
	err = utils.WaitForFuncOnDaemonSet(t, f.Client, namespace, dsName, isDaemonsetOK, retryInterval, timeout)
	if err != nil {
		exportPodsLogs(t, f, namespace, err)
		t.Fatal(err)
	}

	// get DatadogAgent
	agentdeploymentKey := dynclient.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
	err = f.Client.Get(goctx.TODO(), agentdeploymentKey, agentdeployment)
	if err != nil {
		t.Fatal(err)
	}
	firstHash := agentdeployment.Status.Agent.CurrentHash
	// update the DatadogAgent and check that the status is updated
	updateImage := func(ad *datadoghqv1alpha1.DatadogAgent) {
		updatedImageTag := "6.15.0"
		ad.Spec.Agent.Image.Name = fmt.Sprintf("datadog/agent:%s", updatedImageTag)
	}
	err = utils.UpdateDatadogAgentFunc(f, namespace, name, updateImage, retryInterval, timeout)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("check if the Daemonset was updated properly")
	currentHash := ""
	isDaemonsetUpdated := func(ds *appsv1.DaemonSet) (bool, error) {
		currentHash = ds.Annotations[datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey]
		if currentHash != firstHash && currentHash != "" {
			return true, nil
		}
		return false, nil
	}
	err = utils.WaitForFuncOnDaemonSet(t, f.Client, namespace, dsName, isDaemonsetUpdated, retryInterval, timeout)
	if err != nil {
		t.Fatal(err)
	}
	err = utils.WaitForFuncOnDaemonSet(t, f.Client, namespace, dsName, isDaemonsetOK, retryInterval, timeout)
	if err != nil {
		exportPodsLogs(t, f, namespace, err)
		t.Fatal(err)
	}

	t.Logf("Update the DatadogAgent to activate APM")
	updateWithAPM := func(ad *datadoghqv1alpha1.DatadogAgent) {
		ad.Spec.Agent.Apm.Enabled = datadoghqv1alpha1.NewBoolPointer(true)
	}
	err = utils.UpdateDatadogAgentFunc(f, namespace, name, updateWithAPM, retryInterval, timeout)
	if err != nil {
		printDaemonSet(t, f, namespace)
		exportPodsLogs(t, f, namespace, err)
		t.Fatal(err)
	}

	withAPMHash := ""
	isApmActivatedAndRunning := func(ds *appsv1.DaemonSet) (bool, error) {
		withAPMHash = ds.Annotations[datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey]
		if withAPMHash != currentHash && withAPMHash != "" {
			return true, nil
		}
		return false, nil
	}
	err = utils.WaitForFuncOnDaemonSet(t, f.Client, namespace, dsName, isApmActivatedAndRunning, retryInterval, timeout)
	if err != nil {
		exportPodsLogs(t, f, namespace, err)
		t.Fatal(err)
	}
	err = utils.WaitForFuncOnDaemonSet(t, f.Client, namespace, dsName, isDaemonsetOK, retryInterval, timeout)
	if err != nil {
		printDaemonSet(t, f, namespace)
		exportPodsLogs(t, f, namespace, err)
		t.Fatal(err)
	}

	t.Logf("Update the DatadogAgent to activate Process")
	updateWithProcess := func(ad *datadoghqv1alpha1.DatadogAgent) {
		ad.Spec.Agent.Process.Enabled = datadoghqv1alpha1.NewBoolPointer(true)
	}
	err = utils.UpdateDatadogAgentFunc(f, namespace, name, updateWithProcess, retryInterval, timeout)
	if err != nil {
		exportPodsLogs(t, f, namespace, err)
		t.Fatal(err)
	}

	withProcessHash := ""
	isProcessActivatedAndRunning := func(ds *appsv1.DaemonSet) (bool, error) {
		withProcessHash = ds.Annotations[datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey]
		if withProcessHash != withAPMHash && withProcessHash != "" {
			return true, nil
		}
		return false, nil
	}
	err = utils.WaitForFuncOnDaemonSet(t, f.Client, namespace, dsName, isProcessActivatedAndRunning, retryInterval, timeout)
	if err != nil {
		exportPodsLogs(t, f, namespace, err)
		t.Fatal(err)
	}
	err = utils.WaitForFuncOnDaemonSet(t, f.Client, namespace, dsName, isDaemonsetOK, retryInterval, timeout)
	if err != nil {
		exportPodsLogs(t, f, namespace, err)
		t.Fatal(err)
	}

	t.Logf("Update the DatadogAgent to activate SystemProbe")
	updateWithSystemProbe := func(ad *datadoghqv1alpha1.DatadogAgent) {
		ad.Spec.Agent.SystemProbe.Enabled = datadoghqv1alpha1.NewBoolPointer(true)
	}
	err = utils.UpdateDatadogAgentFunc(f, namespace, name, updateWithSystemProbe, retryInterval, timeout)
	if err != nil {
		exportPodsLogs(t, f, namespace, err)
		t.Fatal(err)
	}

	withSystemProbeHash := ""
	isSystemProbeActivatedAndRunning := func(ds *appsv1.DaemonSet) (bool, error) {
		withSystemProbeHash = ds.Annotations[datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey]
		if withSystemProbeHash != withProcessHash && withSystemProbeHash != "" {
			return true, nil
		}
		t.Logf("Daemonset pod not ready %#v", ds.Status)
		return false, nil
	}
	err = utils.WaitForFuncOnDaemonSet(t, f.Client, namespace, dsName, isSystemProbeActivatedAndRunning, retryInterval, timeout)
	if err != nil {
		exportPodsLogs(t, f, namespace, err)
		t.Fatal(err)
	}

	t.Logf("Update the DatadogAgent with custom conf.d and checks.d")
	updateWithConfigMaps := func(ad *datadoghqv1alpha1.DatadogAgent) {
		ad.Spec.Agent.Config.Confd = &datadoghqv1alpha1.ConfigDirSpec{
			ConfigMapName: confdConfigMapName,
		}
		ad.Spec.Agent.Config.Checksd = &datadoghqv1alpha1.ConfigDirSpec{
			ConfigMapName: checksdConfigMapName,
		}
	}
	err = utils.UpdateDatadogAgentFunc(f, namespace, name, updateWithConfigMaps, retryInterval, timeout)
	if err != nil {
		exportPodsLogs(t, f, namespace, err)
		t.Fatal(err)
	}

	areConfigMapsMounted := func(pod *corev1.Pod) (bool, error) {
		needConfigMaps := map[string]bool{
			confdConfigMapName:   true,
			checksdConfigMapName: true,
		}
		count := 0
		name := pod.ObjectMeta.Name

		if pod.Status.Phase != corev1.PodRunning {
			t.Logf("pod %s is not Running (yet?)", name)
			return false, nil
		}

		for _, vol := range pod.Spec.Volumes {
			if vol.ConfigMap != nil && needConfigMaps[vol.ConfigMap.Name] {
				count++
			}
			if count == len(needConfigMaps) {
				t.Logf("config maps properly mounted in pod %s", name)
				return true, nil
			}
		}
		t.Logf("config maps not mounted in pod %s", name)
		return false, nil
	}

	err = utils.WaitForFuncOnPods(t, f.Client, namespace, agentPodLabelSelector, areConfigMapsMounted, retryInterval, timeout)
	if err != nil {
		exportPodsLogs(t, f, namespace, err)
		t.Fatal(err)
	}

	pods, err := utils.FindPodsByLabels(t, f.Client, namespace, agentPodLabelSelector)
	if err != nil {
		t.Fatalf("failed to find agent pods: %+v", err)
	}

	if len(pods.Items) == 0 {
		t.Fatal("no agent pods found")
	}

	t.Logf("running check '%s' in pod %s", checkName, pods.Items[0].Name)
	stdout, _, err := utils.ExecInPod(t, f, namespace, pods.Items[0].Name, "agent", []string{"agent", "check", checkName})
	if err != nil {
		t.Logf("exec failed in pod %s: %+v", pods.Items[0].Name, err)
		t.Fatal(err)
	}

	pattern := fmt.Sprintf(regexp.QuoteMeta(`Configuration Source: file:/etc/datadog-agent/conf.d/%s.yaml`), checkName)
	matched, err := regexp.MatchString(pattern, stdout)
	if err != nil {
		t.Fatalf("regex match failed %+v", err)
	}

	if !matched {
		t.Logf("failed to match stdout with check run:\n%s", stdout)
		t.Fatalf("failed to match stdout with check - expected to find '%s'", pattern)
	}
}

/*
func DeploymentEDS(t *testing.T) {
	namespace, ctx, f := initTestFwkResources(t, "datadog-operator")
	defer ctx.Cleanup()

	name := "foo"
	options := &utils.NewDatadogAgentOptions{
		UseEDS: true,
	}
	agentdeployment, firstHash, _ := utils.NewDatadogAgent(namespace, name, fmt.Sprintf("datadog/agent:%s", "6.12.0"), options)
	err := f.Client.Create(goctx.TODO(), agentdeployment, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	if err != nil {
		t.Fatal(err)
	}

	isOK := func(ad *datadoghqv1alpha1.DatadogAgent) (bool, error) {
		if ad.Status.Agent != nil && ad.Status.Agent.CurrentHash != firstHash {
			return true, nil
		}
		return false, nil
	}
	err = utils.WaitForFuncOnDatadogAgent(t, f.Client, namespace, name, isOK, retryInterval, timeout)
	if err != nil {
		t.Fatal(err)
	}

	// get DatadogAgent
	agentdeploymentKey := dynclient.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
	err = f.Client.Get(goctx.TODO(), agentdeploymentKey, agentdeployment)
	t.Log("error:", err)
	if err != nil {
		t.Fatal(err)
	}

	// check if the ExtendedDaemonset was created properly
	isExtendedDaemonsetOK := func(eds *edsdatadoghqv1alpha1.ExtendedDaemonSet) (bool, error) {
		// Assert the ExtendedDaemonset has the correct spec
		if eds.Spec.Strategy.Canary.Replicas.IntVal == replicas && eds.Annotations[datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey] == firstHash {
			// Update status
			eds.Status.Current = replicas
			eds.Status.Desired = replicas
			return true, nil
		}
		return false, nil
	}
	err = utils.WaitForFuncOnExtendedDaemonSet(t, f.Client, namespace, name, isExtendedDaemonsetOK, retryInterval, timeout)
	if err != nil {
		t.Fatal(err)
	}

	// update the DatadogAgent and check that the status is updated
	updateImage := func(ad *datadoghqv1alpha1.DatadogAgent) {
		updatedImageTag := "6.13.0"
		ad.Spec.Agent.Image.Name = fmt.Sprintf("datadog/agent:%s", updatedImageTag)
	}
	err = utils.UpdateDatadogAgentFunc(f, namespace, agentdeployment.Name, updateImage, retryInterval, timeout)
	if err != nil {
		t.Fatal(err)
	}

	isUpdated := func(ad *datadoghqv1alpha1.DatadogAgent) (bool, error) {
		// hash must be updated and different compared to the initial hash
		if ad.Status.Agent != nil && ad.Status.Agent.CurrentHash != firstHash {
			return true, nil
		}

		return false, nil
	}
	err = utils.WaitForFuncOnDatadogAgent(t, f.Client, namespace, name, isUpdated, retryInterval, timeout)
	if err != nil {
		t.Fatal(err)
	}

	// check if the ExtendedDaemonset was updated properly
	isExtendedDaemonsetUpdated := func(eds *edsdatadoghqv1alpha1.ExtendedDaemonSet) (bool, error) {
		edsHash := eds.Annotations[datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey]
		if edsHash != firstHash && edsHash != "" {
			return true, nil
		}
		return false, nil
	}
	err = utils.WaitForFuncOnExtendedDaemonSet(t, f.Client, namespace, name, isExtendedDaemonsetUpdated, retryInterval, timeout)
	if err != nil {
		t.Fatal(err)
	}
}
*/

func DeploymentWithClusterAgentEnabled(t *testing.T) {
	namespace, ctx, f := initTestFwkResources(t, "datadog-operator")
	var err error
	defer ctx.Cleanup()

	name := "foo"
	agentdeployment := utils.NewDatadogAgent(namespace, name, fmt.Sprintf("datadog/agent:%s", "6.14.0"), &utils.NewDatadogAgentOptions{ClusterAgentEnabled: true})
	err = f.Client.Create(goctx.TODO(), agentdeployment, &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	if err != nil {
		t.Fatal(err)
	}

	isOK := func(ad *datadoghqv1alpha1.DatadogAgent) (bool, error) {
		if ad.Status.Agent == nil || ad.Status.ClusterAgent == nil {
			return false, nil
		}
		if ad.Status.Agent.CurrentHash == "" || ad.Status.ClusterAgent.CurrentHash == "" {
			return false, nil
		}

		for _, condition := range ad.Status.Conditions {
			if condition.Type == datadoghqv1alpha1.ConditionTypeActive && condition.Status == corev1.ConditionTrue {
				return true, nil
			}
		}

		return false, nil
	}
	err = utils.WaitForFuncOnDatadogAgent(t, f.Client, namespace, name, isOK, retryInterval, timeout)
	if err != nil {
		exportPodsLogs(t, f, namespace, err)
		t.Fatal(err)
	}

	// get DatadogAgent
	agentdeploymentKey := dynclient.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
	err = f.Client.Get(goctx.TODO(), agentdeploymentKey, agentdeployment)
	if err != nil {
		exportPodsLogs(t, f, namespace, err)
		t.Fatal(err)
	}
	clusterAgentFirstHash := agentdeployment.Status.ClusterAgent.CurrentHash
	clusterAgentName := fmt.Sprintf("%s-cluster-agent", name)
	// check if the Cluster Agent Deployment was created properly
	isClusterAgentOK := func(dca *appsv1.Deployment) (bool, error) {
		// Assert the Deployment has the correct spec
		if agentdeployment.Spec.ClusterAgent == nil {
			return false, nil
		}

		if agentdeployment.Spec.ClusterAgent.Replicas == nil {
			return false, nil
		}

		if *dca.Spec.Replicas == *agentdeployment.Spec.ClusterAgent.Replicas && dca.Annotations[datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey] == clusterAgentFirstHash {
			return true, nil
		}
		return false, nil
	}
	err = utils.WaitForFuncOnClusterAgentDeployment(t, f.Client, namespace, clusterAgentName, isClusterAgentOK, retryInterval, timeout)
	if err != nil {
		exportPodsLogs(t, f, namespace, err)
		t.Fatal(err)
	}

	// Get last version of DatadogAgent
	agentdeployment = &datadoghqv1alpha1.DatadogAgent{}
	err = f.Client.Get(goctx.TODO(), agentdeploymentKey, agentdeployment)
	if err != nil {
		t.Fatal(err)
	}

	// update the Cluster Agent Deployment Spec and check that the status is updated
	updateImage := func(ad *datadoghqv1alpha1.DatadogAgent) {
		updatedImageTag := "1.3.0"
		ad.Spec.ClusterAgent.Image.Name = fmt.Sprintf("datadog/cluster-agent:%s", updatedImageTag)
		ad.Spec.ClusterAgent.Config.ClusterChecksEnabled = datadoghqv1alpha1.NewBoolPointer(true)
		ad.Spec.ClusterChecksRunner = &datadoghqv1alpha1.DatadogAgentSpecClusterChecksRunnerSpec{}
		ad.Spec.ClusterChecksRunner.Image.Name = "datadog/agent:6.15.0"
	}
	err = utils.UpdateDatadogAgentFunc(f, namespace, name, updateImage, retryInterval, timeout)
	if err != nil {
		t.Fatal(err)
	}

	isUpdated := func(ad *datadoghqv1alpha1.DatadogAgent) (bool, error) {
		// hash must be updated and different compared to the initial hash
		clusterAgentStatusOK := false
		if ad.Status.ClusterAgent != nil && ad.Status.ClusterAgent.CurrentHash != clusterAgentFirstHash {
			clusterAgentStatusOK = true
		}
		ClusterChecksRunnerStatusOK := false
		if ad.Status.ClusterChecksRunner != nil {
			ClusterChecksRunnerStatusOK = true
		}
		return clusterAgentStatusOK && ClusterChecksRunnerStatusOK, nil
	}
	err = utils.WaitForFuncOnDatadogAgent(t, f.Client, namespace, name, isUpdated, retryInterval, timeout)
	if err != nil {
		exportPodsLogs(t, f, namespace, err)
		t.Fatal(err)
	}

	// check if the Cluster Agent Deployment was updated properly
	isClusterAgentUpdated := func(dca *appsv1.Deployment) (bool, error) {
		dcaHash := dca.Annotations[datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey]
		if dcaHash != clusterAgentFirstHash && dcaHash != "" {
			return true, nil
		}
		return false, nil
	}
	err = utils.WaitForFuncOnClusterAgentDeployment(t, f.Client, namespace, clusterAgentName, isClusterAgentUpdated, retryInterval, timeout)
	if err != nil {
		exportPodsLogs(t, f, namespace, err)
		t.Fatal(err)
	}
}

func initTestFwkResources(t *testing.T, deploymentName string) (string, *framework.TestCtx, *framework.Framework) {
	ctx := framework.NewTestCtx(t)
	err := ctx.InitializeClusterResources(&framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval})
	if err != nil {
		t.Fatalf("failed to initialize cluster resources: %v", err)
	}
	t.Log("Initialized cluster resources")
	namespace, err := ctx.GetOperatorNamespace()
	if err != nil {
		t.Fatal(err)
	}

	err = GenerateClusterRoleManifest(t, ctx, namespace, ctx.GetID(), deployDirPath)
	if err != nil {
		t.Fatal(err)
	}

	err = LoadConfigMaps(t, ctx, namespace, deployDirPath)
	if err != nil {
		t.Fatal(err)
	}

	// get global framework variables
	f := framework.Global
	// wait for datadog-controller to be ready
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, deploymentName, 1, retryInterval, timeout)
	if err != nil {
		t.Fatal(err)
	}

	kubesystempods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kube-scheduler-kind-control-plane",
				Namespace: "kube-system",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kube-controller-manager-kind-control-plane",
				Namespace: "kube-system",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kube-apiserver-kind-control-plane",
				Namespace: "kube-system",
			},
		},
	}

	options := &corev1.PodLogOptions{
		Follow:    true,
		SinceTime: &metav1.Time{Time: time.Now()},
	}
	for _, pod := range kubesystempods {
		req := f.KubeClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, options)
		go func(t *testing.T, namespace, name string) {
			t.Logf("Add logger for pod:[%s/%s]", namespace, name)
			readCloser, err := req.Stream(context.TODO())
			if err != nil {
				return
			}
			ctx.AddCleanupFn(
				func() error {
					_ = readCloser.Close()
					t.Logf("end reader [%s]", name)
					return nil
				})
			w := &logWriter{
				name:      name,
				namespace: namespace,
				t:         t,
			}
			_, _ = io.Copy(w, readCloser)
		}(t, pod.Namespace, pod.Name)
	}

	return namespace, ctx, f
}

// GenerateCombinedNamespacedManifest creates a temporary manifest yaml
// by combining all standard namespaced resource manifests in deployDir.
func GenerateClusterRoleManifest(t *testing.T, ctx *framework.TestCtx, namespace, id, deployDir string) error {
	saByte, err := ioutil.ReadFile(filepath.Join(deployDir, serviceAccountYamlFile))
	if err != nil {
		t.Logf("Could not find the serviceaccount manifest: (%v)", err)
	}
	roleByte, err := ioutil.ReadFile(filepath.Join(deployDir, clusterRoleYamlFile))
	if err != nil {
		t.Logf("Could not find role manifest: (%v)", err)
	}
	roleBindingByte, err := ioutil.ReadFile(filepath.Join(deployDir, clusterRoleBindingYamlFile))
	if err != nil {
		t.Logf("Could not find role_binding manifest: (%v)", err)
	}

	var sa *corev1.ServiceAccount
	var clusterRole *rbacv1.ClusterRole
	var clusterRoleBinding *rbacv1.ClusterRoleBinding
	for _, fileByte := range [][]byte{saByte, roleByte, roleBindingByte} {
		decode := scheme.Codecs.UniversalDeserializer().Decode
		obj, _, _ := decode(fileByte, nil, nil)

		switch o := obj.(type) {
		case *corev1.ServiceAccount:
			sa = o
		case *rbacv1.ClusterRole:
			clusterRole = o
		case *rbacv1.ClusterRoleBinding:
			clusterRoleBinding = o
		default:
			fmt.Println("default case")
		}
	}

	clusterRole.Name = fmt.Sprintf("%s-%s", clusterRole.Name, id)
	clusterRoleBinding.Name = fmt.Sprintf("%s-%s", clusterRoleBinding.Name, id)
	{
		clusterRoleBinding.RoleRef.Name = clusterRole.Name

		for i, subject := range clusterRoleBinding.Subjects {
			if subject.Kind == "ServiceAccount" && subject.Name == sa.Name {
				clusterRoleBinding.Subjects[i].Namespace = namespace
			}
		}
	}
	cleanupOption := &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval}

	if err = framework.Global.Client.Create(goctx.TODO(), clusterRole, cleanupOption); err != nil {
		return err
	}
	if err = framework.Global.Client.Create(goctx.TODO(), clusterRoleBinding, cleanupOption); err != nil {
		return err
	}

	return nil
}

// LoadConfigMaps loads config maps from yaml to use in tests
func LoadConfigMaps(t *testing.T, ctx *framework.TestCtx, namespace, deployDir string) error {
	configMaps := []string{
		confdConfigMapYamlFile,
		checksdConfigMapYamlFile,
	}

	for _, cm := range configMaps {
		cmByte, err := ioutil.ReadFile(filepath.Join(deployDir, cm))
		if err != nil {
			t.Logf("failed to load config map %s %s", cm, err)
			return err
		}

		decode := scheme.Codecs.UniversalDeserializer().Decode
		obj, _, _ := decode(cmByte, nil, nil)

		if cmObj, ok := obj.(*corev1.ConfigMap); ok {
			cmObj.ObjectMeta.Namespace = namespace
			cleanupOption := &framework.CleanupOptions{TestContext: ctx, Timeout: cleanupTimeout, RetryInterval: cleanupRetryInterval}

			if err = framework.Global.Client.Create(goctx.TODO(), cmObj, cleanupOption); err != nil {
				return err
			}
		}
	}

	return nil
}

const (
	deployDirPath              = "deploy"
	serviceAccountYamlFile     = "service_account.yaml"
	clusterRoleYamlFile        = "clusterrole.yaml"
	clusterRoleBindingYamlFile = "clusterrole_binding.yaml"
	confdConfigMapYamlFile     = "confd_configmap.yaml"
	confdConfigMapName         = "confd-config"
	checksdConfigMapYamlFile   = "checksd_configmap.yaml"
	checksdConfigMapName       = "checksd-config"
	checkName                  = "hello"
	agentPodLabelSelector      = "app.kubernetes.io/name=datadog-agent-deployment"
)

type logWriter struct {
	name      string
	namespace string
	container string
	t         *testing.T
}

func (l *logWriter) Write(b []byte) (int, error) {
	l.t.Helper()
	l.t.Logf("pod [%s/%s - %s]: %s", l.namespace, l.name, l.container, string(b))
	return len(b), nil
}

func exportPodsLogs(t *testing.T, f *framework.Framework, namespace string, err error) {
	t.Helper()
	if err == nil {
		return
	}
	printPods(t, f, namespace)

	// setup streaming operator pod's logs for simplify investigation
	pods, err2 := f.KubeClient.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
	if err2 != nil {
		t.Fatal(err2)
	}

	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			options := &corev1.PodLogOptions{
				Container: container.Name,
			}
			t.Logf("Add logger for pod:[%s/%s], container: %s", pod.Namespace, pod.Name, container.Name)
			req := f.KubeClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, options)
			readCloser, err := req.Stream(context.TODO())
			if err != nil {
				t.Errorf("unable to stream log for pod:[%s/%s], err:%v", pod.Namespace, pod.Name, err)
				return
			}
			w := &logWriter{
				name:      pod.Name,
				namespace: pod.Namespace,
				container: container.Name,
				t:         t,
			}
			_, _ = io.Copy(w, readCloser)
		}

	}
}

func printPods(t *testing.T, f *framework.Framework, namespace string) {
	t.Helper()
	podList := &corev1.PodList{}
	namespaceOption := &dynclient.ListOptions{Namespace: namespace}
	_ = f.Client.List(goctx.TODO(), podList, namespaceOption)
	for _, pod := range podList.Items {
		b, err2 := json.Marshal(pod)
		if err2 != nil {
			t.Errorf("unable pr marshal pod, err: %v", err2)
		}
		t.Logf("pod [%s]: ", string(b))
	}
}

func printDaemonSet(t *testing.T, f *framework.Framework, namespace string) {
	t.Helper()
	dsList := &appsv1.DaemonSetList{}
	namespaceOption := &dynclient.ListOptions{Namespace: namespace}
	_ = f.Client.List(goctx.TODO(), dsList, namespaceOption)
	for _, ds := range dsList.Items {
		b, err2 := json.Marshal(ds)
		if err2 != nil {
			t.Errorf("unable pr marshal Daemonset, err: %v", err2)
		}
		t.Logf("ds [%s]: ", string(b))
	}
}
