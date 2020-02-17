// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

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
	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
)

func (r *ReconcileDatadogAgent) reconcileClusterAgent(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	result, err := r.manageClusterAgentDependencies(logger, dda, newStatus)
	if shouldReturn(result, err) {
		return result, err
	}
	if dda.Spec.ClusterAgent == nil {
		result, err := r.cleanupClusterAgent(logger, dda, newStatus)
		return result, err
	}

	// Generate a Token for clusterAgent-Agent communication if not provided
	if dda.Spec.Credentials.Token == "" {
		if newStatus.ClusterAgent == nil {
			newStatus.ClusterAgent = &datadoghqv1alpha1.DeploymentStatus{}
		}
		if newStatus.ClusterAgent.GeneratedToken == "" {
			newStatus.ClusterAgent.GeneratedToken = generateRandomString(32)
			return reconcile.Result{}, nil
		}
	}

	if newStatus.ClusterAgent != nil &&
		newStatus.ClusterAgent.DeploymentName != "" &&
		newStatus.ClusterAgent.DeploymentName != getClusterAgentName(dda) {
		return result, fmt.Errorf("Datadog cluster agent Deployment cannot be renamed once created")
	}

	nsName := types.NamespacedName{
		Name:      getClusterAgentName(dda),
		Namespace: dda.Namespace,
	}
	// ClusterAgentDeployment attached to this instance
	clusterAgentDeployment := &appsv1.Deployment{}
	if dda.Spec.ClusterAgent != nil {
		err := r.client.Get(context.TODO(), nsName, clusterAgentDeployment)
		if err != nil {
			if errors.IsNotFound(err) {
				logger.Info("ClusterAgent deployment not found", "name", nsName.Name, "namespace", nsName.Namespace)
				// Create and attach a ClusterAgentDeployment
				return r.createNewClusterAgentDeployment(logger, dda, newStatus)
			}
			return reconcile.Result{}, err
		}

		if result, err = r.updateClusterAgentDeployment(logger, dda, clusterAgentDeployment, newStatus); err != nil {
			return result, err
		}

		// Make sure we have at least one Cluster Agent available replica
		if clusterAgentDeployment.Status.AvailableReplicas == 0 {
			return reconcile.Result{RequeueAfter: defaultRequeuPeriod}, fmt.Errorf("cluster agent deployment is not ready yet: 0 pods available out of %d", clusterAgentDeployment.Status.Replicas)
		}

		return reconcile.Result{}, nil
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileDatadogAgent) createNewClusterAgentDeployment(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	newDCA, hash, err := newClusterAgentDeploymentFromInstance(logger, agentdeployment, newStatus, nil)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Set DatadogAgent instance  instance as the owner and controller
	if err = controllerutil.SetControllerReference(agentdeployment, newDCA, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.Info("Creating a new Cluster Agent Deployment", "deployment.Namespace", newDCA.Namespace, "deployment.Name", newDCA.Name, "agentdeployment.Status.ClusterAgent.CurrentHash", hash)
	newStatus.ClusterAgent = &datadoghqv1alpha1.DeploymentStatus{}
	err = r.client.Create(context.TODO(), newDCA)
	now := metav1.NewTime(time.Now())
	if err != nil {
		updateStatusWithClusterAgent(nil, newStatus, &now)
		return reconcile.Result{}, err
	}

	updateStatusWithClusterAgent(newDCA, newStatus, &now)
	eventInfo := buildEventInfo(newDCA.Name, newDCA.Namespace, deploymentKind, datadog.CreationEvent)
	r.recordEvent(agentdeployment, eventInfo)
	return reconcile.Result{}, nil
}

func updateStatusWithClusterAgent(dca *appsv1.Deployment, newStatus *datadoghqv1alpha1.DatadogAgentStatus, updateTime *metav1.Time) {
	newStatus.ClusterAgent = updateDeploymentStatus(dca, newStatus.ClusterAgent, updateTime)
}

func (r *ReconcileDatadogAgent) updateClusterAgentDeployment(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgent, dca *appsv1.Deployment, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	newDCA, hash, err := newClusterAgentDeploymentFromInstance(logger, agentdeployment, newStatus, dca.Spec.Selector)
	if err != nil {
		return reconcile.Result{}, err
	}

	var needUpdate bool
	if !comparison.IsSameSpecMD5Hash(hash, dca.GetAnnotations()) {
		needUpdate = true
	}

	updateStatusWithClusterAgent(dca, newStatus, nil)

	if !needUpdate {
		return reconcile.Result{}, nil
	}
	logger.Info("update ClusterAgent deployment", "name", dca.Name, "namespace", dca.Namespace)
	// Set DatadogAgent instance  instance as the owner and controller
	if err = controllerutil.SetControllerReference(agentdeployment, dca, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.Info("Updating an existing Cluster Agent Deployment", "deployment.Namespace", newDCA.Namespace, "deployment.Name", newDCA.Name, "currentHash", hash)

	// Copy possibly changed fields
	updateDca := dca.DeepCopy()
	updateDca.Spec = *newDCA.Spec.DeepCopy()
	for k, v := range newDCA.Annotations {
		updateDca.Annotations[k] = v
	}
	for k, v := range newDCA.Labels {
		updateDca.Labels[k] = v
	}

	now := metav1.NewTime(time.Now())
	err = r.client.Update(context.TODO(), updateDca)
	if err != nil {
		return reconcile.Result{}, err
	}
	eventInfo := buildEventInfo(updateDca.Name, updateDca.Namespace, deploymentKind, datadog.UpdateEvent)
	r.recordEvent(agentdeployment, eventInfo)
	updateStatusWithClusterAgent(updateDca, newStatus, &now)
	return reconcile.Result{}, nil
}

// newClusterAgentDeploymentFromInstance creates a Cluster Agent Deployment from a given DatadogAgent
func newClusterAgentDeploymentFromInstance(logger logr.Logger,
	agentdeployment *datadoghqv1alpha1.DatadogAgent,
	newStatus *datadoghqv1alpha1.DatadogAgentStatus,
	selector *metav1.LabelSelector) (*appsv1.Deployment, string, error) {
	labels := map[string]string{
		datadoghqv1alpha1.AgentDeploymentNameLabelKey:      agentdeployment.Name,
		datadoghqv1alpha1.AgentDeploymentComponentLabelKey: datadoghqv1alpha1.DefaultClusterAgentResourceSuffix,
	}
	for key, val := range agentdeployment.Labels {
		labels[key] = val
	}
	for key, val := range getDefaultLabels(agentdeployment, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(agentdeployment)) {
		labels[key] = val
	}

	if selector != nil {
		for key, val := range selector.MatchLabels {
			labels[key] = val
		}
	} else {
		selector = &metav1.LabelSelector{
			MatchLabels: map[string]string{
				datadoghqv1alpha1.AgentDeploymentNameLabelKey:      agentdeployment.Name,
				datadoghqv1alpha1.AgentDeploymentComponentLabelKey: datadoghqv1alpha1.DefaultClusterAgentResourceSuffix,
			},
		}
	}

	annotations := map[string]string{}
	for key, val := range agentdeployment.Annotations {
		annotations[key] = val
	}

	dca := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        getClusterAgentName(agentdeployment),
			Namespace:   agentdeployment.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Template: newClusterAgentPodTemplate(logger, agentdeployment, labels, annotations),
			Replicas: agentdeployment.Spec.ClusterAgent.Replicas,
			Selector: selector,
		},
	}
	hash, err := comparison.SetMD5GenerationAnnotation(&dca.ObjectMeta, agentdeployment.Spec.ClusterAgent)
	return dca, hash, err
}

func (r *ReconcileDatadogAgent) manageClusterAgentDependencies(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	result, err := r.manageClusterAgentSecret(logger, dda, newStatus)
	if shouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageClusterAgentService(logger, dda, newStatus)
	if shouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageMetricsServerService(logger, dda, newStatus)
	if shouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageClusterAgentPDB(logger, dda, newStatus)
	if shouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageClusterAgentRBACs(logger, dda)
	if shouldReturn(result, err) {
		return result, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileDatadogAgent) cleanupClusterAgent(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	nsName := types.NamespacedName{
		Name:      getClusterAgentName(dda),
		Namespace: dda.Namespace,
	}
	// ClusterAgentDeployment attached to this instance
	clusterAgentDeployment := &appsv1.Deployment{}
	if err := r.client.Get(context.TODO(), nsName, clusterAgentDeployment); err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	logger.Info("Deleting Cluster Agent Deployment", "deployment.Namespace", clusterAgentDeployment.Namespace, "deployment.Name", clusterAgentDeployment.Name)
	eventInfo := buildEventInfo(clusterAgentDeployment.Name, clusterAgentDeployment.Namespace, clusterRoleBindingKind, datadog.DeletionEvent)
	r.recordEvent(dda, eventInfo)
	if err := r.client.Delete(context.TODO(), clusterAgentDeployment); err != nil {
		return reconcile.Result{}, err
	}
	newStatus.ClusterAgent = nil
	return reconcile.Result{Requeue: true}, nil
}

// newClusterAgentPodTemplate generates a PodTemplate from a DatadogClusterAgentDeployment spec
func newClusterAgentPodTemplate(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgent, labels, annotations map[string]string) corev1.PodTemplateSpec {
	// copy Spec to configure the Cluster Agent Pod Template
	clusterAgentSpec := agentdeployment.Spec.ClusterAgent.DeepCopy()

	newPodTemplate := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: labels,
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: getClusterAgentServiceAccount(agentdeployment),
			Containers: []corev1.Container{
				{
					Name:            "cluster-agent",
					Image:           clusterAgentSpec.Image.Name,
					ImagePullPolicy: *clusterAgentSpec.Image.PullPolicy,
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 5005,
							Name:          "agentport",
							Protocol:      "TCP",
						},
					},
					Env:          getEnvVarsForClusterAgent(logger, agentdeployment),
					VolumeMounts: agentdeployment.Spec.ClusterAgent.Config.VolumeMounts,
				},
			},
			Affinity:    getPodAffinity(clusterAgentSpec.Affinity, getClusterAgentName(agentdeployment)),
			Tolerations: clusterAgentSpec.Tolerations,
			Volumes:     agentdeployment.Spec.ClusterAgent.Config.Volumes,
		},
	}

	container := &newPodTemplate.Spec.Containers[0]

	if datadoghqv1alpha1.BoolValue(agentdeployment.Spec.ClusterAgent.Config.MetricsProviderEnabled) {
		port := getClusterAgentMetricsProviderPort(agentdeployment.Spec.ClusterAgent.Config)
		container.Ports = append(container.Ports, corev1.ContainerPort{
			ContainerPort: port,
			Name:          "metricsapi",
			Protocol:      "TCP",
		})
		probe := &corev1.Probe{
			Handler: corev1.Handler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.IntOrString{
						IntVal: port,
					},
					Scheme: corev1.URISchemeHTTPS,
				},
			},
		}
		container.LivenessProbe = probe
		container.ReadinessProbe = probe
	}

	if clusterAgentSpec.Config.Resources != nil {
		container.Resources = *clusterAgentSpec.Config.Resources
	}

	return newPodTemplate
}

// getEnvVarsForClusterAgent converts Cluster Agent Config into container env vars
func getEnvVarsForClusterAgent(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) []corev1.EnvVar {
	spec := &dda.Spec
	envVars := []corev1.EnvVar{
		{
			Name:  datadoghqv1alpha1.DDClusterName,
			Value: spec.ClusterName,
		},
		{
			Name:  datadoghqv1alpha1.DDSite,
			Value: spec.Site,
		},
		{
			Name:  datadoghqv1alpha1.DDddURL,
			Value: *spec.Agent.Config.DDUrl,
		},
		{
			Name:  datadoghqv1alpha1.DDClusterChecksRunnerEnabled,
			Value: strconv.FormatBool(*spec.ClusterAgent.Config.ClusterChecksRunnerEnabled),
		},
		{
			Name:  datadoghqv1alpha1.DDClusterAgentKubeServiceName,
			Value: getClusterAgentServiceName(dda),
		},
		{
			Name:      datadoghqv1alpha1.DDClusterAgentAuthToken,
			ValueFrom: getClusterAgentAuthToken(dda),
		},
		{
			Name:  datadoghqv1alpha1.DDLeaderElection,
			Value: "true",
		},
	}

	if spec.ClusterAgent.Config.LogLevel != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDLogLevel,
			Value: *spec.ClusterAgent.Config.LogLevel,
		})
	}

	if needClusterAgentSecret(dda) {
		if spec.Credentials.APIKeyExistingSecret != "" {
			envVars = append(envVars, corev1.EnvVar{
				Name:      datadoghqv1alpha1.DDAPIKey,
				ValueFrom: getAPIKeyFromSecret(dda),
			})
		} else {
			envVars = append(envVars, corev1.EnvVar{
				Name:  datadoghqv1alpha1.DDAPIKey,
				Value: spec.Credentials.APIKey,
			})
		}
		if isMetricsProviderEnabled(spec.ClusterAgent) {
			envVars = append(envVars, corev1.EnvVar{
				Name:  datadoghqv1alpha1.DDMetricsProviderEnabled,
				Value: strconv.FormatBool(*spec.ClusterAgent.Config.MetricsProviderEnabled),
			})

			envVars = append(envVars, corev1.EnvVar{
				Name:  datadoghqv1alpha1.DDMetricsProviderPort,
				Value: strconv.Itoa(int(getClusterAgentMetricsProviderPort(spec.ClusterAgent.Config))),
			})
			if spec.Credentials.APIKeyExistingSecret != "" {
				envVars = append(envVars, corev1.EnvVar{
					Name:      datadoghqv1alpha1.DDAppKey,
					ValueFrom: getAppKeyFromSecret(dda),
				})
			} else {
				envVars = append(envVars, corev1.EnvVar{
					Name:  datadoghqv1alpha1.DDAppKey,
					Value: spec.Credentials.AppKey,
				})
			}
		}
	}

	// Cluster Checks Runner config
	if *spec.ClusterAgent.Config.ClusterChecksRunnerEnabled {
		envVars = append(envVars, []corev1.EnvVar{
			{
				Name:  datadoghqv1alpha1.DDExtraConfigProviders,
				Value: datadoghqv1alpha1.KubeServicesConfigProvider,
			},
			{
				Name:  datadoghqv1alpha1.DDExtraListeners,
				Value: strings.Join([]string{datadoghqv1alpha1.KubeServicesListener, datadoghqv1alpha1.KubeEndpointsListener}, " "),
			},
		}...)
	}
	return append(envVars, spec.Agent.Config.Env...)
}

func getClusterAgentName(dda *datadoghqv1alpha1.DatadogAgent) string {
	if dda.Spec.ClusterAgent != nil && dda.Spec.ClusterAgent.DeploymentName != "" {
		return dda.Spec.ClusterAgent.DeploymentName
	}
	return fmt.Sprintf("%s-%s", dda.Name, "cluster-agent")
}

func getClusterAgentMetricsProviderPort(config datadoghqv1alpha1.ClusterAgentConfig) int32 {
	if config.MetricsProviderPort != nil {
		return *config.MetricsProviderPort
	}
	return datadoghqv1alpha1.DefaultMetricsServerServicePort
}

// manageClusterAgentRBACs creates deletes and updates the RBACs for the Cluster Agent
func (r *ReconcileDatadogAgent) manageClusterAgentRBACs(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	if dda.Spec.ClusterAgent == nil {
		return r.cleanupClusterAgentRbacResources(logger, dda)
	}

	if !isCreateRBACEnabled(dda.Spec.ClusterAgent.Rbac) {
		return reconcile.Result{}, nil
	}

	rbacResourcesName := getClusterAgentRbacResourcesName(dda)
	clusterAgentVersion := getClusterAgentVersion(dda)
	// Create or update ClusterRole
	clusterRole := &rbacv1.ClusterRole{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName}, clusterRole); err != nil {
		if errors.IsNotFound(err) {
			return r.createClusterAgentClusterRole(logger, dda, rbacResourcesName, clusterAgentVersion)
		}
		return reconcile.Result{}, err
	}
	if result, err := r.updateIfNeededClusterAgentClusterRole(logger, dda, rbacResourcesName, clusterAgentVersion, clusterRole); err != nil {
		return result, err
	}

	// Create ClusterRoleBinding
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName}, clusterRoleBinding); err != nil {
		if errors.IsNotFound(err) {
			return r.createClusterRoleBinding(logger, dda, roleBindingInfo{
				name:               rbacResourcesName,
				roleName:           rbacResourcesName,
				serviceAccountName: getClusterAgentServiceAccount(dda),
			}, clusterAgentVersion)
		}
		return reconcile.Result{}, err
	}

	// Create or delete HPA ClusterRoleBindig
	hpaClusterRoleBindingName := getHPAClusterRoleBindingName(dda)
	hpaClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	if datadoghqv1alpha1.BoolValue(dda.Spec.ClusterAgent.Config.MetricsProviderEnabled) {
		if err := r.client.Get(context.TODO(), types.NamespacedName{Name: hpaClusterRoleBindingName}, hpaClusterRoleBinding); err != nil {
			if errors.IsNotFound(err) {
				return r.createHPAClusterRoleBinding(logger, dda, hpaClusterRoleBindingName, clusterAgentVersion)
			}
			return reconcile.Result{}, err
		}
	} else {
		if result, err := r.deleteIfNeededHpaClusterRoleBinding(logger, dda, hpaClusterRoleBindingName, clusterAgentVersion, hpaClusterRoleBinding); err != nil {
			return result, err
		}
	}

	// Create ServiceAccount
	serviceAccount := &corev1.ServiceAccount{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName, Namespace: dda.Namespace}, serviceAccount); err != nil {
		if errors.IsNotFound(err) {
			return r.createServiceAccount(logger, dda, rbacResourcesName, clusterAgentVersion)
		}
		return reconcile.Result{}, err
	}

	// Create or update Role
	role := &rbacv1.Role{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName, Namespace: dda.Namespace}, role); err != nil {
		if errors.IsNotFound(err) {
			return r.createClusterAgentRole(logger, dda, rbacResourcesName, clusterAgentVersion)
		}
		return reconcile.Result{}, err
	}
	if result, err := r.updateIfNeededClusterAgentRole(logger, dda, rbacResourcesName, clusterAgentVersion, role); err != nil {
		return result, err
	}
	// Create or update RoleBinding
	roleBinding := &rbacv1.RoleBinding{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName, Namespace: dda.Namespace}, roleBinding); err != nil {
		if errors.IsNotFound(err) {
			info := roleBindingInfo{
				name:               rbacResourcesName,
				roleName:           rbacResourcesName,
				serviceAccountName: getClusterAgentServiceAccount(dda),
			}
			return r.createClusterAgentRoleBinding(logger, dda, info, clusterAgentVersion)
		}
		return reconcile.Result{}, err
	}
	if result, err := r.updateIfNeededClusterAgentRoleBinding(logger, dda, rbacResourcesName, clusterAgentVersion, roleBinding); err != nil {
		return result, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileDatadogAgent) createClusterAgentClusterRole(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string) (reconcile.Result, error) {
	clusterRole := buildClusterAgentClusterRole(dda, name, agentVersion)
	if err := SetOwnerReference(dda, clusterRole, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.V(1).Info("createClusterAgentClusterRole", "clusterRole.name", clusterRole.Name)
	eventInfo := buildEventInfo(clusterRole.Name, clusterRole.Namespace, clusterRoleKind, datadog.CreationEvent)
	r.recordEvent(dda, eventInfo)
	return reconcile.Result{Requeue: true}, r.client.Create(context.TODO(), clusterRole)
}

func (r *ReconcileDatadogAgent) createClusterAgentRole(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string) (reconcile.Result, error) {
	role := buildClusterAgentRole(dda, name, agentVersion)
	if err := controllerutil.SetControllerReference(dda, role, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.V(1).Info("createClusterAgentRole", "role.name", role.Name)
	eventInfo := buildEventInfo(role.Name, role.Namespace, roleKind, datadog.CreationEvent)
	r.recordEvent(dda, eventInfo)
	return reconcile.Result{Requeue: true}, r.client.Create(context.TODO(), role)
}

func (r *ReconcileDatadogAgent) createAgentClusterRole(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string) (reconcile.Result, error) {
	clusterRole := buildAgentClusterRole(dda, name, agentVersion)
	if err := SetOwnerReference(dda, clusterRole, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.V(1).Info("createAgentClusterRole", "clusterRole.name", clusterRole.Name)
	eventInfo := buildEventInfo(clusterRole.Name, clusterRole.Namespace, clusterRoleKind, datadog.CreationEvent)
	r.recordEvent(dda, eventInfo)
	return reconcile.Result{Requeue: true}, r.client.Create(context.TODO(), clusterRole)
}

func (r *ReconcileDatadogAgent) updateIfNeededClusterAgentClusterRole(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string, clusterRole *rbacv1.ClusterRole) (reconcile.Result, error) {
	newClusterRole := buildClusterAgentClusterRole(dda, name, agentVersion)
	if !apiequality.Semantic.DeepEqual(newClusterRole.Rules, clusterRole.Rules) {
		logger.V(1).Info("updateClusterAgentClusterRole", "clusterRole.name", clusterRole.Name)
		if err := r.client.Update(context.TODO(), newClusterRole); err != nil {
			return reconcile.Result{}, err
		}
		eventInfo := buildEventInfo(newClusterRole.Name, newClusterRole.Namespace, clusterRoleKind, datadog.UpdateEvent)
		r.recordEvent(dda, eventInfo)
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileDatadogAgent) updateIfNeededClusterAgentRole(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string, role *rbacv1.Role) (reconcile.Result, error) {
	newRole := buildClusterAgentRole(dda, name, agentVersion)
	if !apiequality.Semantic.DeepEqual(newRole.Rules, role.Rules) {
		logger.V(1).Info("updateClusterAgentRole", "role.name", newRole.Name)
		if err := r.client.Update(context.TODO(), newRole); err != nil {
			return reconcile.Result{}, err
		}
		eventInfo := buildEventInfo(newRole.Name, newRole.Namespace, roleKind, datadog.UpdateEvent)
		r.recordEvent(dda, eventInfo)
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileDatadogAgent) updateIfNeededAgentClusterRole(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string, clusterRole *rbacv1.ClusterRole) (reconcile.Result, error) {
	newClusterRole := buildAgentClusterRole(dda, name, agentVersion)
	if !apiequality.Semantic.DeepEqual(newClusterRole.Rules, clusterRole.Rules) {
		logger.V(1).Info("updateAgentClusterRole", "clusterRole.name", clusterRole.Name)
		if err := r.client.Update(context.TODO(), newClusterRole); err != nil {
			return reconcile.Result{}, err
		}
		eventInfo := buildEventInfo(newClusterRole.Name, newClusterRole.Namespace, clusterRoleKind, datadog.UpdateEvent)
		r.recordEvent(dda, eventInfo)
	}
	return reconcile.Result{}, nil
}

// cleanupClusterAgentRbacResources deletes ClusterRole, ClusterRoleBindings, and ServiceAccount of the Cluster Agent
func (r *ReconcileDatadogAgent) cleanupClusterAgentRbacResources(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	rbacResourcesName := getClusterAgentRbacResourcesName(dda)
	// Delete ClusterRole
	if result, err := r.cleanupClusterRole(logger, r.client, dda, rbacResourcesName); err != nil {
		return result, err
	}
	// Delete Cluster Role Binding
	if result, err := r.cleanupClusterRoleBinding(logger, r.client, dda, rbacResourcesName); err != nil {
		return result, err
	}
	// Delete HPA Cluster Role Binding
	hpaClusterRoleBindingName := getHPAClusterRoleBindingName(dda)
	if result, err := r.cleanupClusterRoleBinding(logger, r.client, dda, hpaClusterRoleBindingName); err != nil {
		return result, err
	}
	// Delete Service Account
	if result, err := r.cleanupServiceAccount(logger, r.client, dda, rbacResourcesName); err != nil {
		return result, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileDatadogAgent) createClusterAgentRoleBinding(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, info roleBindingInfo, agentVersion string) (reconcile.Result, error) {
	roleBinding := buildRoleBinding(dda, info, agentVersion)
	if err := controllerutil.SetControllerReference(dda, roleBinding, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.V(1).Info("createClusterAgentRoleBinding", "roleBinding.name", roleBinding.Name, "roleBinding.Namespace", roleBinding.Namespace)
	eventInfo := buildEventInfo(roleBinding.Name, roleBinding.Namespace, roleBindingKind, datadog.CreationEvent)
	r.recordEvent(dda, eventInfo)
	return reconcile.Result{}, r.client.Create(context.TODO(), roleBinding)
}

func (r *ReconcileDatadogAgent) updateIfNeededClusterAgentRoleBinding(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string, roleBinding *rbacv1.RoleBinding) (reconcile.Result, error) {
	info := roleBindingInfo{
		name:               getClusterAgentRbacResourcesName(dda),
		roleName:           getClusterAgentRbacResourcesName(dda),
		serviceAccountName: getClusterAgentServiceAccount(dda),
	}
	newRoleBinding := buildRoleBinding(dda, info, agentVersion)
	if !apiequality.Semantic.DeepEqual(newRoleBinding.RoleRef, roleBinding.RoleRef) || !apiequality.Semantic.DeepEqual(newRoleBinding.Subjects, roleBinding.Subjects) {
		logger.V(1).Info("updateAgentClusterRoleBinding", "roleBinding.name", newRoleBinding.Name, "roleBinding.namespace", newRoleBinding.Namespace)
		eventInfo := buildEventInfo(newRoleBinding.Name, newRoleBinding.Namespace, roleBindingKind, datadog.UpdateEvent)
		r.recordEvent(dda, eventInfo)
		if err := r.client.Update(context.TODO(), newRoleBinding); err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

// buildAgentClusterRole creates a ClusterRole object for the Agent based on its config
func buildAgentClusterRole(dda *datadoghqv1alpha1.DatadogAgent, name, version string) *rbacv1.ClusterRole {
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Labels: getDefaultLabels(dda, name, version),
			Name:   name,
		},
	}

	rbacRules := []rbacv1.PolicyRule{
		{
			// Get /metrics permissions
			NonResourceURLs: []string{datadoghqv1alpha1.MetricsURL},
			Verbs:           []string{datadoghqv1alpha1.GetVerb},
		},
		{
			// Kubelet connectivity
			APIGroups: []string{datadoghqv1alpha1.CoreAPIGroup},
			Resources: []string{
				datadoghqv1alpha1.NodeMetricsResource,
				datadoghqv1alpha1.NodeSpecResource,
				datadoghqv1alpha1.NodeProxyResource,
			},
			Verbs: []string{datadoghqv1alpha1.GetVerb},
		},
		{
			// Leader election check
			APIGroups: []string{datadoghqv1alpha1.CoreAPIGroup},
			Resources: []string{datadoghqv1alpha1.EndpointsResource},
			Verbs:     []string{datadoghqv1alpha1.GetVerb},
		},
	}

	if dda.Spec.ClusterAgent == nil {
		// Cluster Agent is disabled, the Agent needs extra permissions
		// to collect cluster level metrics and events
		rbacRules = append(rbacRules, getDefaultClusterAgentPolicyRules()...)

		if datadoghqv1alpha1.BoolValue(dda.Spec.Agent.Config.CollectEvents) {
			rbacRules = append(rbacRules, getEventCollectionPolicyRule())
		}

		if datadoghqv1alpha1.BoolValue(dda.Spec.Agent.Config.LeaderElection) {
			rbacRules = append(rbacRules, getLeaderElectionPolicyRule()...)
		}
	}

	clusterRole.Rules = rbacRules

	return clusterRole
}

// getDefaultClusterAgentPolicyRules returns the default policy rules for the Cluster Agent
// Can be used by the Agent if the Cluster Agent is disabled
func getDefaultClusterAgentPolicyRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups: []string{datadoghqv1alpha1.CoreAPIGroup},
			Resources: []string{
				datadoghqv1alpha1.ServicesResource,
				datadoghqv1alpha1.EventsResource,
				datadoghqv1alpha1.EndpointsResource,
				datadoghqv1alpha1.PodsResource,
				datadoghqv1alpha1.NodesResource,
				datadoghqv1alpha1.ComponentStatusesResource,
			},
			Verbs: []string{
				datadoghqv1alpha1.GetVerb,
				datadoghqv1alpha1.ListVerb,
				datadoghqv1alpha1.WatchVerb,
			},
		},
		{
			APIGroups: []string{datadoghqv1alpha1.OpenShiftQuotaAPIGroup},
			Resources: []string{datadoghqv1alpha1.ClusterResourceQuotasResource},
			Verbs:     []string{datadoghqv1alpha1.GetVerb, datadoghqv1alpha1.ListVerb},
		},
		{
			NonResourceURLs: []string{datadoghqv1alpha1.VersionURL, datadoghqv1alpha1.HealthzURL},
			Verbs:           []string{datadoghqv1alpha1.GetVerb},
		},
	}
}

// buildClusterRoleBinding creates a ClusterRoleBinding object
func buildClusterRoleBinding(dda *datadoghqv1alpha1.DatadogAgent, info roleBindingInfo, agentVersion string) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Labels: getDefaultLabels(dda, info.name, agentVersion),
			Name:   info.name,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: datadoghqv1alpha1.RbacAPIGroup,
			Kind:     datadoghqv1alpha1.ClusterRoleKind,
			Name:     info.roleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      datadoghqv1alpha1.ServiceAccountKind,
				Name:      info.serviceAccountName,
				Namespace: dda.Namespace,
			},
		},
	}
}

// buildClusterAgentClusterRole creates a ClusterRole object for the Cluster Agent based on its config
func buildClusterAgentClusterRole(dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string) *rbacv1.ClusterRole {
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Labels: getDefaultLabels(dda, name, agentVersion),
			Name:   name,
		},
	}

	rbacRules := getDefaultClusterAgentPolicyRules()

	rbacRules = append(rbacRules, rbacv1.PolicyRule{
		// Horizontal Pod Autoscaling
		APIGroups: []string{datadoghqv1alpha1.AutoscalingAPIGroup},
		Resources: []string{datadoghqv1alpha1.HorizontalPodAutoscalersRecource},
		Verbs:     []string{datadoghqv1alpha1.ListVerb, datadoghqv1alpha1.WatchVerb},
	})

	if datadoghqv1alpha1.BoolValue(dda.Spec.Agent.Config.CollectEvents) {
		rbacRules = append(rbacRules, getEventCollectionPolicyRule())
	}

	if datadoghqv1alpha1.BoolValue(dda.Spec.Agent.Config.LeaderElection) {
		rbacRules = append(rbacRules, getLeaderElectionPolicyRule()...)
	}

	if datadoghqv1alpha1.BoolValue(dda.Spec.ClusterAgent.Config.MetricsProviderEnabled) {
		rbacRules = append(rbacRules, rbacv1.PolicyRule{
			APIGroups: []string{datadoghqv1alpha1.CoreAPIGroup},
			Resources: []string{datadoghqv1alpha1.ConfigMapsResource},
			ResourceNames: []string{
				datadoghqv1alpha1.DatadogCustomMetricsResourceName,
				datadoghqv1alpha1.ExtensionApiServerAuthResourceName,
			},
			Verbs: []string{datadoghqv1alpha1.GetVerb, datadoghqv1alpha1.UpdateVerb},
		})
	}

	clusterRole.Rules = rbacRules

	return clusterRole
}

// buildClusterAgentRole creates a Role object for the Cluster Agent based on its config
func buildClusterAgentRole(dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string) *rbacv1.Role {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    getDefaultLabels(dda, name, agentVersion),
			Name:      name,
			Namespace: dda.Namespace,
		},
	}

	rbacRules := getLeaderElectionPolicyRule()

	if datadoghqv1alpha1.BoolValue(dda.Spec.ClusterAgent.Config.MetricsProviderEnabled) {
		rbacRules = append(rbacRules, rbacv1.PolicyRule{
			APIGroups: []string{datadoghqv1alpha1.CoreAPIGroup},
			Resources: []string{datadoghqv1alpha1.ConfigMapsResource},
			ResourceNames: []string{
				datadoghqv1alpha1.DatadogCustomMetricsResourceName,
				datadoghqv1alpha1.ExtensionApiServerAuthResourceName,
			},
			Verbs: []string{datadoghqv1alpha1.GetVerb, datadoghqv1alpha1.UpdateVerb},
		})
	}

	role.Rules = rbacRules

	return role
}
