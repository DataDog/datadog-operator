// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component"
	componentdca "github.com/DataDog/datadog-operator/controllers/datadogagent/component/clusteragent"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object"
	objectvolume "github.com/DataDog/datadog-operator/controllers/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/orchestrator"
	cilium "github.com/DataDog/datadog-operator/pkg/cilium/v1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
)

func (r *Reconciler) reconcileClusterChecksRunner(logger logr.Logger, features []feature.Feature, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	result, err := r.manageClusterChecksRunnerDependencies(logger, dda)
	if utils.ShouldReturn(result, err) {
		return result, err
	}

	if !needClusterChecksRunner(dda) {
		return r.cleanupClusterChecksRunner(logger, dda, newStatus)
	}

	if newStatus.ClusterChecksRunner != nil &&
		newStatus.ClusterChecksRunner.DeploymentName != "" &&
		newStatus.ClusterChecksRunner.DeploymentName != getClusterChecksRunnerName(dda) {
		return result, fmt.Errorf("datadog cluster checks runner Deployment cannot be renamed once created")
	}

	nsName := types.NamespacedName{
		Name:      getClusterChecksRunnerName(dda),
		Namespace: dda.Namespace,
	}
	// ClusterChecksRunnerDeployment attached to this instance
	ClusterChecksRunnerDeployment := &appsv1.Deployment{}
	if needClusterChecksRunner(dda) {
		err := r.client.Get(context.TODO(), nsName, ClusterChecksRunnerDeployment)
		if err != nil {
			if errors.IsNotFound(err) {
				logger.Info("ClusterChecksRunner deployment not found", "name", nsName.Name, "namespace", nsName.Namespace)
				// Create and attach a ClusterChecksRunner Deployment
				var result reconcile.Result
				result, err = r.createNewClusterChecksRunnerDeployment(logger, dda, newStatus)
				return result, err
			}
			return reconcile.Result{}, err
		}

		result, err := r.updateClusterChecksRunnerDeployment(logger, dda, ClusterChecksRunnerDeployment, newStatus)
		return result, err
	}
	return reconcile.Result{}, nil
}

func needClusterChecksRunner(dda *datadoghqv1alpha1.DatadogAgent) bool {
	if isClusterAgentEnabled(dda.Spec.ClusterAgent) &&
		apiutils.BoolValue(dda.Spec.ClusterChecksRunner.Enabled) &&
		dda.Spec.ClusterAgent.Config != nil &&
		apiutils.BoolValue(dda.Spec.ClusterAgent.Config.ClusterChecksEnabled) {
		return true
	}

	return false
}

func (r *Reconciler) createNewClusterChecksRunnerDeployment(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	newDCAW, hash, err := newClusterChecksRunnerDeploymentFromInstance(dda, nil)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Set ClusterChecksRunner Deployment instance as the owner and controller
	if err = controllerutil.SetControllerReference(dda, newDCAW, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.Info("Creating a new Cluster Checks Runner Deployment", "deployment.Namespace", newDCAW.Namespace, "deployment.Name", newDCAW.Name, "agentdeployment.Status.ClusterChecksRunner.CurrentHash", hash)
	newStatus.ClusterChecksRunner = &datadoghqv1alpha1.DeploymentStatus{}
	err = r.client.Create(context.TODO(), newDCAW)
	now := metav1.NewTime(time.Now())
	if err != nil {
		updateStatusWithClusterChecksRunner(nil, newStatus, &now)
		return reconcile.Result{}, err
	}

	updateStatusWithClusterChecksRunner(newDCAW, newStatus, &now)
	event := buildEventInfo(newDCAW.Name, newDCAW.Namespace, deploymentKind, datadog.CreationEvent)
	r.recordEvent(dda, event)
	return reconcile.Result{}, nil
}

func updateStatusWithClusterChecksRunner(dcaw *appsv1.Deployment, newStatus *datadoghqv1alpha1.DatadogAgentStatus, updateTime *metav1.Time) {
	newStatus.ClusterChecksRunner = updateDeploymentStatus(dcaw, newStatus.ClusterChecksRunner, updateTime)
}

func (r *Reconciler) updateClusterChecksRunnerDeployment(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, dep *appsv1.Deployment, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	newCLCR, hash, err := newClusterChecksRunnerDeploymentFromInstance(dda, dep.Spec.Selector)
	if err != nil {
		return reconcile.Result{}, err
	}

	needUpdate := !comparison.IsSameSpecMD5Hash(hash, dep.GetAnnotations())

	updateStatusWithClusterChecksRunner(dep, newStatus, nil)

	if !needUpdate {
		return reconcile.Result{}, nil
	}

	logger.Info("update Cluster Checks Runner deployment", "name", dep.Name, "namespace", dep.Namespace)

	// Set DatadogAgent instance  instance as the owner and controller
	if err = controllerutil.SetControllerReference(dda, dep, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.Info("Updating an existing Cluster Checks Runner Deployment", "deployment.Namespace", newCLCR.Namespace, "deployment.Name", newCLCR.Name, "currentHash", hash)

	// Copy possibly changed fields
	updateCLCR := dep.DeepCopy()
	updateCLCR.Spec = *newCLCR.Spec.DeepCopy()
	updateCLCR.Spec.Replicas = getReplicas(dep.Spec.Replicas, updateCLCR.Spec.Replicas)
	for k, v := range newCLCR.Annotations {
		updateCLCR.Annotations[k] = v
	}
	for k, v := range newCLCR.Labels {
		updateCLCR.Labels[k] = v
	}

	now := metav1.NewTime(time.Now())
	err = kubernetes.UpdateFromObject(context.TODO(), r.client, updateCLCR, dep.ObjectMeta)
	if err != nil {
		return reconcile.Result{}, err
	}
	event := buildEventInfo(updateCLCR.Name, updateCLCR.Namespace, deploymentKind, datadog.UpdateEvent)
	r.recordEvent(dda, event)
	updateStatusWithClusterChecksRunner(updateCLCR, newStatus, &now)
	return reconcile.Result{}, nil
}

// newClusterChecksRunnerDeploymentFromInstance creates a Cluster Agent Deployment from a given DatadogAgent
func newClusterChecksRunnerDeploymentFromInstance(dda *datadoghqv1alpha1.DatadogAgent, selector *metav1.LabelSelector) (*appsv1.Deployment, string, error) {
	clc := component.NewDeployment(dda, apicommon.DefaultClusterChecksRunnerResourceSuffix, getClusterChecksRunnerName(dda), getClusterChecksRunnerVersion(dda), selector)

	clc.Spec.Template = newClusterChecksRunnerPodTemplate(dda, clc.GetLabels(), clc.GetAnnotations())
	clc.Spec.Replicas = dda.Spec.ClusterChecksRunner.Replicas

	hash, err := comparison.SetMD5DatadogAgentGenerationAnnotation(&clc.ObjectMeta, dda.Spec.ClusterChecksRunner)
	return clc, hash, err
}

func (r *Reconciler) manageClusterChecksRunnerDependencies(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	result, err := r.manageAgentSecret(logger, dda)
	if utils.ShouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageClusterChecksRunnerPDB(logger, dda)
	if utils.ShouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageConfigMap(logger, dda, getClusterChecksRunnerCustomConfigConfigMapName(dda), buildClusterChecksRunnerConfigurationConfigMap)
	if utils.ShouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageClusterChecksRunnerRBACs(logger, dda)
	if utils.ShouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageConfigMap(logger, dda, component.GetInstallInfoConfigMapName(dda), buildInstallInfoConfigMap)
	if utils.ShouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageClusterChecksRunnerNetworkPolicy(logger, dda)
	if utils.ShouldReturn(result, err) {
		return result, err
	}

	return reconcile.Result{}, nil
}

func (r *Reconciler) cleanupClusterChecksRunner(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	nsName := types.NamespacedName{
		Name:      getClusterChecksRunnerName(dda),
		Namespace: dda.Namespace,
	}

	// ClusterChecksRunnerDeployment attached to this instance
	ClusterChecksRunnerDeployment := &appsv1.Deployment{}
	if err := r.client.Get(context.TODO(), nsName, ClusterChecksRunnerDeployment); err != nil {
		if !errors.IsNotFound(err) {
			return reconcile.Result{}, err
		}
	} else {
		logger.Info("Deleting Cluster Checks Runner Deployment", "deployment.Namespace", ClusterChecksRunnerDeployment.Namespace, "deployment.Name", ClusterChecksRunnerDeployment.Name)
		event := buildEventInfo(ClusterChecksRunnerDeployment.Name, ClusterChecksRunnerDeployment.Namespace, deploymentKind, datadog.DeletionEvent)
		r.recordEvent(dda, event)
		if err := r.client.Delete(context.TODO(), ClusterChecksRunnerDeployment); err != nil {
			return reconcile.Result{}, err
		}
	}

	newStatus.ClusterChecksRunner = nil
	return reconcile.Result{}, nil
}

// newClusterChecksRunnerPodTemplate generates a PodTemplate from a DatadogClusterChecksRunnerDeployment spec
func newClusterChecksRunnerPodTemplate(dda *datadoghqv1alpha1.DatadogAgent, labels, annotations map[string]string) corev1.PodTemplateSpec {
	// copy Spec to configure the Cluster Checks Runner Pod Template
	clusterChecksRunnerSpec := dda.Spec.ClusterChecksRunner.DeepCopy()

	spec := &dda.Spec
	volumeMounts := getVolumeMountsForClusterChecksRunner(dda)
	envVars := getEnvVarsForClusterChecksRunner(dda)
	image := getImage(clusterChecksRunnerSpec.Image, spec.Registry)

	newPodTemplate := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: getClusterChecksRunnerServiceAccount(dda),
			InitContainers: []corev1.Container{
				getInitContainer(
					spec, "init-config",
					[]string{"for script in $(find /etc/cont-init.d/ -type f -name '*.sh' | sort) ; do bash $script ; done"},
					volumeMounts, envVars,
					image,
				),
			},
			Containers: []corev1.Container{
				{
					Name:            "cluster-checks-runner",
					Image:           image,
					ImagePullPolicy: *clusterChecksRunnerSpec.Image.PullPolicy,
					Env:             envVars,
					VolumeMounts:    volumeMounts,
					LivenessProbe:   dda.Spec.Agent.Config.LivenessProbe,
					ReadinessProbe:  dda.Spec.Agent.Config.ReadinessProbe,
					Command:         getDefaultIfEmpty(dda.Spec.ClusterChecksRunner.Config.Command, []string{"agent", "run"}),
					Args:            getDefaultIfEmpty(dda.Spec.ClusterChecksRunner.Config.Args, nil),
					SecurityContext: &corev1.SecurityContext{
						ReadOnlyRootFilesystem:   apiutils.NewBoolPointer(true),
						AllowPrivilegeEscalation: apiutils.NewBoolPointer(false),
					},
				},
			},
			Volumes:           getVolumesForClusterChecksRunner(dda),
			Affinity:          getPodAffinity(clusterChecksRunnerSpec.Affinity),
			Tolerations:       clusterChecksRunnerSpec.Tolerations,
			PriorityClassName: clusterChecksRunnerSpec.PriorityClassName,
			// To be uncommented when the agent Dockerfile will be updated to use a non-root user by default
			// SecurityContext: &corev1.PodSecurityContext{
			// 	RunAsNonRoot: apiutils.NewBoolPointer(true),
			// },
		},
	}

	for key, val := range clusterChecksRunnerSpec.AdditionalLabels {
		newPodTemplate.Labels[key] = val
	}

	for key, val := range clusterChecksRunnerSpec.AdditionalAnnotations {
		newPodTemplate.Annotations[key] = val
	}

	if clusterChecksRunnerSpec.Config.Resources != nil {
		newPodTemplate.Spec.Containers[0].Resources = *clusterChecksRunnerSpec.Config.Resources
	}

	if clusterChecksRunnerSpec.Config.SecurityContext != nil {
		newPodTemplate.Spec.SecurityContext = clusterChecksRunnerSpec.Config.SecurityContext.DeepCopy()
	}

	return newPodTemplate
}

func buildClusterChecksRunnerConfigurationConfigMap(dda *datadoghqv1alpha1.DatadogAgent) (*corev1.ConfigMap, error) {
	if !apiutils.BoolValue(dda.Spec.ClusterChecksRunner.Enabled) {
		return nil, nil
	}
	return buildConfigurationConfigMap(dda, datadoghqv1alpha1.ConvertCustomConfig(dda.Spec.ClusterChecksRunner.CustomConfig), getClusterChecksRunnerCustomConfigConfigMapName(dda), datadoghqv1alpha1.AgentCustomConfigVolumeSubPath)
}

// getEnvVarsForClusterChecksRunner converts Cluster Checks Runner Config into container env vars
func getEnvVarsForClusterChecksRunner(dda *datadoghqv1alpha1.DatadogAgent) []corev1.EnvVar {
	spec := &dda.Spec
	envVars := []corev1.EnvVar{
		{
			Name:  apicommon.DDClusterChecksEnabled,
			Value: "true",
		},
		{
			Name:  apicommon.DDClusterAgentEnabled,
			Value: "true",
		},
		{
			Name:  apicommon.DDClusterAgentKubeServiceName,
			Value: componentdca.GetClusterAgentServiceName(dda),
		},
		{
			Name:  apicommon.DDExtraConfigProviders,
			Value: apicommon.ClusterChecksConfigProvider,
		},
		{
			Name:  apicommon.DDHealthPort,
			Value: strconv.Itoa(int(*spec.ClusterChecksRunner.Config.HealthPort)),
		},
		{
			Name:  apicommon.DDAPMEnabled,
			Value: "false",
		},
		{
			Name:  apicommon.DDProcessAgentEnabled,
			Value: "false",
		},
		{
			Name:  apicommon.DDLogsEnabled,
			Value: "false",
		},
		{
			Name:  apicommon.DDDogstatsdEnabled,
			Value: "false",
		},
		{
			Name:  apicommon.DDEnableMetadataCollection,
			Value: "false",
		},
		{
			Name:  apicommon.DDClcRunnerEnabled,
			Value: "true",
		},
		{
			Name: apicommon.DDClcRunnerHost,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: apicommon.FieldPathStatusPodIP,
				},
			},
		},
		{
			Name: apicommon.DDHostname,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: apicommon.FieldPathSpecNodeName,
				},
			},
		},
		{
			Name: apicommon.DDClcRunnerID,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: apicommon.FieldPathMetaName,
				},
			},
		},
	}

	envVars = append(envVars, corev1.EnvVar{
		Name:      apicommon.DDAPIKey,
		ValueFrom: getAPIKeyFromSecret(dda),
	})

	envVars = append(envVars, corev1.EnvVar{
		Name:      apicommon.DDClusterAgentAuthToken,
		ValueFrom: getClusterAgentAuthToken(dda),
	})

	if spec.ClusterName != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  apicommon.DDClusterName,
			Value: spec.ClusterName,
		})
	}

	if spec.Site != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  apicommon.DDSite,
			Value: spec.Site,
		})
	}

	envVars = append(envVars, corev1.EnvVar{
		Name:  apicommon.DDLogLevel,
		Value: *spec.ClusterChecksRunner.Config.LogLevel,
	})

	if isOrchestratorExplorerEnabled(dda) {
		envs, _ := orchestrator.EnvVars(dda.Spec.Features.OrchestratorExplorer)

		envVars = append(envVars, envs...)

		// The orchestrator ckeck retrieves the cluster id from the Cluster Agent
		envVars = append(envVars, envForClusterAgentConnection(dda)...)
	}

	if spec.Agent.Config.DDUrl != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  apicommon.DDddURL,
			Value: *spec.Agent.Config.DDUrl,
		})
	}

	return append(envVars, spec.ClusterChecksRunner.Config.Env...)
}

func getClusterChecksRunnerVersion(dda *datadoghqv1alpha1.DatadogAgent) string {
	// TODO implement this method
	return ""
}

func getClusterChecksRunnerName(dda *datadoghqv1alpha1.DatadogAgent) string {
	name := fmt.Sprintf("%s-%s", dda.Name, "cluster-checks-runner")
	if apiutils.BoolValue(dda.Spec.ClusterChecksRunner.Enabled) && dda.Spec.ClusterChecksRunner.DeploymentName != "" {
		name = dda.Spec.ClusterChecksRunner.DeploymentName
	}
	return name
}

// getVolumesForClusterChecksRunner defines volumes for the Cluster Checks Runner
func getVolumesForClusterChecksRunner(dda *datadoghqv1alpha1.DatadogAgent) []corev1.Volume {
	volumes := []corev1.Volume{
		getVolumeForChecksd(dda),
		component.GetVolumeForConfig(),
		component.GetVolumeForLogs(),

		// /tmp is needed because some versions of the DCA (at least until
		// 1.19.0) write to it.
		// In some code paths, the klog lib writes to /tmp instead of using the
		// standard datadog logs path.
		// In some envs like Openshift, when running as non-root, the pod will
		// not have permissions to write on /tmp, that's why we need to mount
		// it with write perms.
		component.GetVolumeForTmp(),
		component.GetVolumeInstallInfo(dda),
		component.GetVolumeForRmCorechecks(),
	}

	if dda.Spec.ClusterChecksRunner.CustomConfig != nil {
		volume := objectvolume.GetVolumeFromCustomConfigSpec(datadoghqv1alpha1.ConvertCustomConfig(dda.Spec.ClusterChecksRunner.CustomConfig), getClusterChecksRunnerCustomConfigConfigMapName(dda), datadoghqv1alpha1.AgentCustomConfigVolumeName)
		volumes = append(volumes, volume)
	}
	return append(volumes, dda.Spec.ClusterChecksRunner.Config.Volumes...)
}

// getVolumeMountsForClusterChecksRunner defines volume mounts for the Cluster Checks Runner
func getVolumeMountsForClusterChecksRunner(dda *datadoghqv1alpha1.DatadogAgent) []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		getVolumeMountForChecksd(),
		component.GetVolumeMountForLogs(),
		component.GetVolumeMountForTmp(),
		component.GetVolumeMountForInstallInfo(),
		component.GetVolumeMountForRmCorechecks(),
	}

	// Add configuration volumesMount default and custom config (datadog.yaml) volume
	volumeMounts = append(volumeMounts, getVolumeMountForConfig(dda.Spec.ClusterChecksRunner.CustomConfig)...)

	return append(volumeMounts, dda.Spec.ClusterChecksRunner.Config.VolumeMounts...)
}

func getClusterChecksRunnerCustomConfigConfigMapName(dda *datadoghqv1alpha1.DatadogAgent) string {
	return fmt.Sprintf("%s-runner-datadog-yaml", dda.Name)
}

// getPodAffinity returns the pod anti affinity of the cluster check runner pods
// the default anti affinity prefers scheduling the runners on different nodes if possible
// for better checks stability in case of node failure.
func getPodAffinity(affinity *corev1.Affinity) *corev1.Affinity {
	if affinity != nil {
		return affinity
	}

	return &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
				{
					Weight: 50,
					PodAffinityTerm: corev1.PodAffinityTerm{
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								apicommon.AgentDeploymentComponentLabelKey: apicommon.DefaultClusterChecksRunnerResourceSuffix,
							},
						},
						TopologyKey: "kubernetes.io/hostname",
					},
				},
			},
		},
	}
}

func (r *Reconciler) manageClusterChecksRunnerNetworkPolicy(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	spec := dda.Spec.ClusterChecksRunner
	builder := clusterChecksRunnerNetworkPolicyBuilder{dda, spec.NetworkPolicy}
	if !apiutils.BoolValue(spec.Enabled) || spec.NetworkPolicy == nil || !apiutils.BoolValue(spec.NetworkPolicy.Create) {
		return r.cleanupNetworkPolicy(logger, dda, builder.Name())
	}

	return r.ensureNetworkPolicy(logger, dda, builder)
}

type clusterChecksRunnerNetworkPolicyBuilder struct {
	dda *datadoghqv1alpha1.DatadogAgent
	np  *datadoghqv1alpha1.NetworkPolicySpec
}

func (b clusterChecksRunnerNetworkPolicyBuilder) Name() string {
	return fmt.Sprintf("%s-%s", b.dda.Name, apicommon.DefaultClusterChecksRunnerResourceSuffix)
}

func (b clusterChecksRunnerNetworkPolicyBuilder) NetworkPolicySpec() *datadoghqv1alpha1.NetworkPolicySpec {
	return b.np
}

func (b clusterChecksRunnerNetworkPolicyBuilder) BuildKubernetesPolicy() *networkingv1.NetworkPolicy {
	dda := b.dda
	name := b.Name()

	egressRules := []networkingv1.NetworkPolicyEgressRule{
		// Egress to datadog intake and kubeapi server
		{
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Port: &intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 443,
					},
				},
			},
		},
	}

	if apiutils.BoolValue(dda.Spec.ClusterChecksRunner.Enabled) {
		egressRules = append(egressRules, networkingv1.NetworkPolicyEgressRule{
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Port: &intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: apicommon.DefaultClusterAgentServicePort,
					},
				},
			},
			To: []networkingv1.NetworkPolicyPeer{
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": fmt.Sprintf("%s-%s", dda.Name, apicommon.DefaultClusterAgentResourceSuffix),
						},
					},
				},
			},
		})
	}

	// The cluster check runners are susceptible to connect to any service
	// that would be annotated with auto-discovery annotations.
	//
	// When a user wants to add a check on one of its service, he needs to
	// * annotate its service
	// * add an ingress policy from the CLC on its own pod
	// In order to not ask end-users to inject NetworkPolicy on the agent in
	// the agent namespace, the agent must be allowed to probe any service.
	egressRules = append(egressRules, networkingv1.NetworkPolicyEgressRule{})

	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    object.GetDefaultLabels(dda, apicommon.DefaultClusterChecksRunnerResourceSuffix, getClusterChecksRunnerVersion(dda)),
			Name:      name,
			Namespace: dda.Namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: b.PodSelector(),
			Egress:      egressRules,
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
		},
	}

	return policy
}

func (b clusterChecksRunnerNetworkPolicyBuilder) PodSelector() metav1.LabelSelector {
	return metav1.LabelSelector{
		MatchLabels: map[string]string{
			kubernetes.AppKubernetesInstanceLabelKey: getClusterChecksRunnerName(b.dda),
			kubernetes.AppKubernetesPartOfLabelKey:   object.NewPartOfLabelValue(b.dda).String(),
		},
	}
}

func (b clusterChecksRunnerNetworkPolicyBuilder) ddFQDNs() []cilium.FQDNSelector {
	selectors := []cilium.FQDNSelector{}

	ddURL := b.dda.Spec.Agent.Config.DDUrl
	if ddURL != nil {
		selectors = append(selectors, cilium.FQDNSelector{
			MatchName: strings.TrimPrefix(*ddURL, "https://"),
		})
	}

	var site string
	if b.dda.Spec.Site != "" {
		site = b.dda.Spec.Site
	} else {
		site = defaultSite
	}

	selectors = append(selectors, []cilium.FQDNSelector{
		{
			MatchPattern: fmt.Sprintf("*-app.agent.%s", site),
		},
	}...)

	return selectors
}

func (b clusterChecksRunnerNetworkPolicyBuilder) BuildCiliumPolicy() *cilium.NetworkPolicy {
	return &cilium.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    object.GetDefaultLabels(b.dda, apicommon.DefaultClusterChecksRunnerResourceSuffix, getClusterChecksRunnerVersion(b.dda)),
			Name:      b.Name(),
			Namespace: b.dda.Namespace,
		},
		Specs: []cilium.NetworkPolicySpec{
			ciliumEgressMetadataServerRule(b),
			ciliumEgressDNS(b),
			{
				Description:      "Egress to Datadog intake",
				EndpointSelector: b.PodSelector(),
				Egress: []cilium.EgressRule{
					{
						ToFQDNs: b.ddFQDNs(),
						ToPorts: []cilium.PortRule{
							{
								Ports: []cilium.PortProtocol{
									{
										Port:     "443",
										Protocol: cilium.ProtocolTCP,
									},
								},
							},
						},
					},
				},
			},
			{
				Description:      "Egress to cluster agent",
				EndpointSelector: b.PodSelector(),
				Egress: []cilium.EgressRule{
					{
						ToPorts: []cilium.PortRule{
							{
								Ports: []cilium.PortProtocol{
									{
										Port:     "5005",
										Protocol: cilium.ProtocolTCP,
									},
								},
							},
						},
						ToEndpoints: []metav1.LabelSelector{
							{
								MatchLabels: map[string]string{
									kubernetes.AppKubernetesInstanceLabelKey: apicommon.DefaultClusterAgentResourceSuffix,
									kubernetes.AppKubernetesPartOfLabelKey:   fmt.Sprintf("%s-%s", b.dda.Namespace, b.dda.Name),
								},
							},
						},
					},
				},
			},
			ciliumEgressChecks(b),
		},
	}
}
