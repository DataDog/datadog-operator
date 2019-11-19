// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagentdeployment

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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
)

func (r *ReconcileDatadogAgentDeployment) reconcileClusterAgent(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	result, err := r.manageClusterAgentDependencies(logger, dad, newStatus)
	if shouldReturn(result, err) {
		return result, err
	}
	if dad.Spec.ClusterAgent == nil {
		result, err := r.cleanupClusterAgent(logger, dad, newStatus)
		return result, err
	}

	// Generate a Token for clusterAgent-Agent communication if not provided
	if dad.Spec.Credentials.Token == "" {
		if newStatus.ClusterAgent == nil {
			newStatus.ClusterAgent = &datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStatus{}
		}
		if newStatus.ClusterAgent.GeneratedToken == "" {
			newStatus.ClusterAgent.GeneratedToken = generateRandomString(16)
			return reconcile.Result{}, nil
		}
	}

	nsName := types.NamespacedName{
		Name:      getClusterAgentName(dad),
		Namespace: dad.Namespace,
	}
	// ClusterAgentDeployment attached to this instance
	clusterAgentDeployment := &appsv1.Deployment{}
	if dad.Spec.ClusterAgent != nil {
		err := r.client.Get(context.TODO(), nsName, clusterAgentDeployment)
		if err != nil {
			if errors.IsNotFound(err) {
				logger.Info("ClusterAgent deployment not found", "name", nsName.Name, "namespace", nsName.Namespace)
				// Create and attach a ClusterAgentDeployment
				var result reconcile.Result
				result, err = r.createNewClusterAgentDeployment(logger, dad, newStatus)
				return r.updateStatusIfNeeded(logger, dad, newStatus, result, err)
			}
			return r.updateStatusIfNeeded(logger, dad, newStatus, reconcile.Result{}, err)
		}

		result, err := r.updateClusterAgentDeployment(logger, dad, clusterAgentDeployment, newStatus)
		return r.updateStatusIfNeeded(logger, dad, newStatus, result, err)
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileDatadogAgentDeployment) createNewClusterAgentDeployment(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	newDCA, hash, err := newClusterAgentDeploymentFromInstance(logger, agentdeployment, newStatus)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Set ClusterAgent Deployment instance as the owner and controller
	if err = controllerutil.SetControllerReference(agentdeployment, newDCA, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.Info("Creating a new Cluster Agent Deployment", "deployment.Namespace", newDCA.Namespace, "deployment.Name", newDCA.Name, "agentdeployment.Status.ClusterAgent.CurrentHash", hash)
	newStatus.ClusterAgent = &datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStatus{}
	err = r.client.Create(context.TODO(), newDCA)
	if err != nil {
		newStatus.ClusterAgent.State = datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStateFailed
		return reconcile.Result{}, err
	}
	now := metav1.NewTime(time.Now())
	newStatus.ClusterAgent.State = datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStateStarted
	updateStatusWithClusterAgent(newDCA, newStatus, &now)
	r.recorder.Event(agentdeployment, corev1.EventTypeNormal, "Create Cluster Agent Deployment", fmt.Sprintf("%s/%s", newDCA.Namespace, newDCA.Name))
	return reconcile.Result{}, nil
}

func updateStatusWithClusterAgent(dca *appsv1.Deployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus, updateTime *metav1.Time) {
	newStatus.ClusterAgent = updateDeploymentStatus(dca, newStatus.ClusterAgent, updateTime)
}

func (r *ReconcileDatadogAgentDeployment) updateClusterAgentDeployment(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgentDeployment, dca *appsv1.Deployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	newDCA, hash, err := newClusterAgentDeploymentFromInstance(logger, agentdeployment, newStatus)
	if err != nil {
		return reconcile.Result{}, err
	}

	var needUpdate bool
	if !comparison.CompareSpecMD5Hash(hash, dca.GetAnnotations()) {
		needUpdate = true
	}

	updateStatusWithClusterAgent(dca, newStatus, nil)

	if !needUpdate {
		return reconcile.Result{}, nil
	}
	logger.Info("update ClusterAgent deployment", "name", dca.Name, "namespace", dca.Namespace)
	// Set ClusterAgent Deployment instance as the owner and controller
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
	r.recorder.Event(agentdeployment, corev1.EventTypeNormal, "Update Cluster Agent Deployment", fmt.Sprintf("%s/%s", updateDca.Namespace, updateDca.Name))
	updateStatusWithClusterAgent(updateDca, newStatus, &now)
	return reconcile.Result{}, nil
}

// newClusterAgentDeploymentFromInstance creates a Cluster Agent Deployment from a given DatadogAgentDeployment
func newClusterAgentDeploymentFromInstance(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (*appsv1.Deployment, string, error) {
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
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					datadoghqv1alpha1.AgentDeploymentNameLabelKey:      agentdeployment.Name,
					datadoghqv1alpha1.AgentDeploymentComponentLabelKey: "cluster-agent",
				},
			},
		},
	}
	hash, err := comparison.SetMD5GenerationAnnotation(&dca.ObjectMeta, agentdeployment.Spec.ClusterAgent)
	return dca, hash, err
}

func (r *ReconcileDatadogAgentDeployment) manageClusterAgentDependencies(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	result, err := r.manageClusterAgentSecret(logger, dad, newStatus)
	if shouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageClusterAgentService(logger, dad, newStatus)
	if shouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageMetricsServerService(logger, dad, newStatus)
	if shouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageClusterAgentRBACs(logger, dad)
	if shouldReturn(result, err) {
		return result, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileDatadogAgentDeployment) cleanupClusterAgent(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	nsName := types.NamespacedName{
		Name:      getClusterAgentName(dad),
		Namespace: dad.Namespace,
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
	r.recorder.Event(dad, corev1.EventTypeNormal, "Delete Cluster Agent Deployment", fmt.Sprintf("%s/%s", clusterAgentDeployment.Namespace, clusterAgentDeployment.Name))
	if err := r.client.Delete(context.TODO(), clusterAgentDeployment); err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{Requeue: true}, nil
}

// newClusterAgentPodTemplate generates a PodTemplate from a DatadogClusterAgentDeployment spec
func newClusterAgentPodTemplate(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgentDeployment, labels, annotations map[string]string) corev1.PodTemplateSpec {
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
						{
							ContainerPort: 443,
							Name:          "metricsapi",
							Protocol:      "TCP",
						},
					},
					Env: getEnvVarsForClusterAgent(logger, agentdeployment),
				},
			},
			Affinity:    getPodAffinity(clusterAgentSpec.Affinity, getClusterAgentName(agentdeployment)),
			Tolerations: clusterAgentSpec.Tolerations,
		},
	}

	if clusterAgentSpec.Config.Resources != nil {
		newPodTemplate.Spec.Containers[0].Resources = *clusterAgentSpec.Config.Resources
	}

	return newPodTemplate
}

// getEnvVarsForClusterAgent converts Cluster Agent Config into container env vars
func getEnvVarsForClusterAgent(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment) []corev1.EnvVar {
	spec := &dad.Spec
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
			Value: getClusterAgentServiceName(dad),
		},
		{
			Name:      datadoghqv1alpha1.DDClusterAgentAuthToken,
			ValueFrom: getClusterAgentAuthToken(dad),
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

	if needClusterAgentSecret(dad) {
		if spec.Credentials.APIKeyExistingSecret != "" {
			envVars = append(envVars, corev1.EnvVar{
				Name:      datadoghqv1alpha1.DDAPIKey,
				ValueFrom: getAPIKeyFromSecret(dad),
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
			if spec.Credentials.APIKeyExistingSecret != "" {
				envVars = append(envVars, corev1.EnvVar{
					Name:      datadoghqv1alpha1.DDAppKey,
					ValueFrom: getAppKeyFromSecret(dad),
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

func getClusterAgentName(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	return fmt.Sprintf("%s-%s", dad.Name, "cluster-agent")
}

// manageClusterAgentRBACs creates deletes and updates the RBACs for the Cluster Agent
func (r *ReconcileDatadogAgentDeployment) manageClusterAgentRBACs(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment) (reconcile.Result, error) {
	if dad.Spec.ClusterAgent == nil {
		return r.cleanupClusterAgentRbacResources(logger, dad)
	}

	if !isCreateRBACEnabled(dad.Spec.ClusterAgent.Rbac) {
		return reconcile.Result{}, nil
	}

	rbacResourcesName := getClusterAgentRbacResourcesName(dad)
	clusterAgentVersion := getClusterAgentVersion(dad)
	// Create or update ClusterRole
	clusterRole := &rbacv1.ClusterRole{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName}, clusterRole); err != nil {
		if errors.IsNotFound(err) {
			return r.createClusterAgentClusterRole(logger, dad, rbacResourcesName, clusterAgentVersion)
		}
		return reconcile.Result{}, err
	}
	if result, err := r.updateIfNeededClusterAgentClusterRole(logger, dad, rbacResourcesName, clusterAgentVersion, clusterRole); err != nil {
		return result, err
	}

	// Create ClusterRoleBindig
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName}, clusterRoleBinding); err != nil {
		if errors.IsNotFound(err) {
			return r.createClusterRoleBinding(logger, dad, roleBindingInfo{
				name:               rbacResourcesName,
				roleName:           rbacResourcesName,
				serviceAccountName: getClusterAgentServiceAccount(dad),
			}, clusterAgentVersion)
		}
		return reconcile.Result{}, err
	}

	// Create or delete HPA ClusterRoleBindig
	hpaClusterRoleBindingName := getHPAClusterRoleBindingName(dad)
	hpaClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	if datadoghqv1alpha1.BoolValue(dad.Spec.ClusterAgent.Config.MetricsProviderEnabled) {
		if err := r.client.Get(context.TODO(), types.NamespacedName{Name: hpaClusterRoleBindingName}, hpaClusterRoleBinding); err != nil {
			if errors.IsNotFound(err) {
				return r.createHPAClusterRoleBinding(logger, dad, hpaClusterRoleBindingName, clusterAgentVersion)
			}
			return reconcile.Result{}, err
		}
	} else {
		if result, err := r.deleteIfNeededHpaClusterRoleBinding(logger, dad, hpaClusterRoleBindingName, clusterAgentVersion, hpaClusterRoleBinding); err != nil {
			return result, err
		}
	}

	// Create ServiceAccount
	serviceAccount := &corev1.ServiceAccount{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName, Namespace: dad.Namespace}, serviceAccount); err != nil {
		if errors.IsNotFound(err) {
			return r.createServiceAccount(logger, dad, rbacResourcesName, clusterAgentVersion)
		}
		return reconcile.Result{}, err
	}

	// Create or update Role
	role := &rbacv1.Role{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName, Namespace: dad.Namespace}, role); err != nil {
		if errors.IsNotFound(err) {
			return r.createClusterAgentRole(logger, dad, rbacResourcesName, clusterAgentVersion)
		}
		return reconcile.Result{}, err
	}
	if result, err := r.updateIfNeededClusterAgentRole(logger, dad, rbacResourcesName, clusterAgentVersion, role); err != nil {
		return result, err
	}
	// Create or update RoleBinding
	roleBinding := &rbacv1.RoleBinding{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName, Namespace: dad.Namespace}, roleBinding); err != nil {
		if errors.IsNotFound(err) {
			info := roleBindingInfo{
				name:               rbacResourcesName,
				roleName:           rbacResourcesName,
				serviceAccountName: getClusterAgentServiceAccount(dad),
			}
			return r.createClusterAgentRoleBinding(logger, dad, info, clusterAgentVersion)
		}
		return reconcile.Result{}, err
	}
	if result, err := r.updateIfNeededClusterAgentRoleBinding(logger, dad, rbacResourcesName, clusterAgentVersion, roleBinding); err != nil {
		return result, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileDatadogAgentDeployment) createClusterAgentClusterRole(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, name, agentVersion string) (reconcile.Result, error) {
	clusterRole := buildClusterAgentClusterRole(dad, name, agentVersion)
	if err := controllerutil.SetControllerReference(dad, clusterRole, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.V(1).Info("createClusterAgentClusterRole", "clusterRole.name", clusterRole.Name)
	return reconcile.Result{}, r.client.Create(context.TODO(), clusterRole)
}

func (r *ReconcileDatadogAgentDeployment) createClusterAgentRole(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, name, agentVersion string) (reconcile.Result, error) {
	clusterRole := buildClusterAgentRole(dad, name, agentVersion)
	if err := controllerutil.SetControllerReference(dad, clusterRole, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.V(1).Info("createClusterAgentRole", "clusterRole.name", clusterRole.Name)
	return reconcile.Result{}, r.client.Create(context.TODO(), clusterRole)
}

func (r *ReconcileDatadogAgentDeployment) createAgentClusterRole(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, name, agentVersion string) (reconcile.Result, error) {
	clusterRole := buildAgentClusterRole(dad, name, agentVersion)
	if err := controllerutil.SetControllerReference(dad, clusterRole, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.V(1).Info("createAgentClusterRole", "clusterRole.name", clusterRole.Name)
	return reconcile.Result{}, r.client.Create(context.TODO(), clusterRole)
}

func (r *ReconcileDatadogAgentDeployment) updateIfNeededClusterAgentClusterRole(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, name, agentVersion string, clusterRole *rbacv1.ClusterRole) (reconcile.Result, error) {
	newClusterRole := buildClusterAgentClusterRole(dad, name, agentVersion)
	if !apiequality.Semantic.DeepEqual(newClusterRole.Rules, clusterRole.Rules) {
		logger.V(1).Info("updateClusterAgentClusterRole", "clusterRole.name", clusterRole.Name)
		if err := r.client.Update(context.TODO(), newClusterRole); err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileDatadogAgentDeployment) updateIfNeededClusterAgentRole(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, name, agentVersion string, role *rbacv1.Role) (reconcile.Result, error) {
	newRole := buildClusterAgentRole(dad, name, agentVersion)
	if !apiequality.Semantic.DeepEqual(newRole.Rules, role.Rules) {
		logger.V(1).Info("updateClusterAgentRole", "role.name", newRole.Name)
		if err := r.client.Update(context.TODO(), newRole); err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileDatadogAgentDeployment) updateIfNeededAgentClusterRole(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, name, agentVersion string, clusterRole *rbacv1.ClusterRole) (reconcile.Result, error) {
	newClusterRole := buildAgentClusterRole(dad, name, agentVersion)
	if !apiequality.Semantic.DeepEqual(newClusterRole.Rules, clusterRole.Rules) {
		logger.V(1).Info("updateAgentClusterRole", "clusterRole.name", clusterRole.Name)
		if err := r.client.Update(context.TODO(), newClusterRole); err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

// cleanupClusterAgentRbacResources deletes ClusterRole, ClusterRoleBindings, and ServiceAccount of the Cluster Agent
func (r *ReconcileDatadogAgentDeployment) cleanupClusterAgentRbacResources(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment) (reconcile.Result, error) {
	rbacResourcesName := getClusterAgentRbacResourcesName(dad)
	// Delete ClusterRole
	if result, err := cleanupClusterRole(r.client, rbacResourcesName); err != nil {
		return result, err
	}
	// Delete Cluster Role Binding
	if result, err := cleanupClusterRoleBinding(r.client, rbacResourcesName); err != nil {
		return result, err
	}
	// Delete HPA Cluster Role Binding
	hpaClusterRoleBindingName := getHPAClusterRoleBindingName(dad)
	if result, err := cleanupClusterRoleBinding(r.client, hpaClusterRoleBindingName); err != nil {
		return result, err
	}
	// Delete Service Account
	if result, err := cleanupServiceAccount(r.client, rbacResourcesName); err != nil {
		return result, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileDatadogAgentDeployment) createClusterAgentRoleBinding(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, info roleBindingInfo, agentVersion string) (reconcile.Result, error) {
	roleBinding := buildRoleBinding(dad, info, agentVersion)
	if err := controllerutil.SetControllerReference(dad, roleBinding, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.V(1).Info("createClusterAgentRoleBinding", "roleBinding.name", roleBinding.Name, "roleBinding.Namespace", roleBinding.Namespace)
	return reconcile.Result{}, r.client.Create(context.TODO(), roleBinding)
}

func (r *ReconcileDatadogAgentDeployment) updateIfNeededClusterAgentRoleBinding(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, name, agentVersion string, roleBinding *rbacv1.RoleBinding) (reconcile.Result, error) {
	info := roleBindingInfo{
		name:               getClusterAgentRbacResourcesName(dad),
		roleName:           getClusterAgentRbacResourcesName(dad),
		serviceAccountName: getClusterAgentServiceAccount(dad),
	}
	newRoleBinding := buildRoleBinding(dad, info, agentVersion)
	if !apiequality.Semantic.DeepEqual(newRoleBinding.RoleRef, roleBinding.RoleRef) || !apiequality.Semantic.DeepEqual(newRoleBinding.Subjects, roleBinding.Subjects) {
		logger.V(1).Info("updateAgentClusterRoleBinding", "roleBinding.name", newRoleBinding.Name, "roleBinding.namespace", newRoleBinding.Namespace)
		if err := r.client.Update(context.TODO(), newRoleBinding); err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

// buildAgentClusterRole creates a ClusterRole object for the Agent based on its config
func buildAgentClusterRole(dad *datadoghqv1alpha1.DatadogAgentDeployment, name, version string) *rbacv1.ClusterRole {
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Labels: getDefaultLabels(dad, name, version),
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

	if dad.Spec.ClusterAgent == nil {
		// Cluster Agent is disabled, the Agent needs extra permissions
		// to collect cluster level metrics and events
		rbacRules = append(rbacRules, getDefaultClusterAgentPolicyRules()...)

		if datadoghqv1alpha1.BoolValue(dad.Spec.Agent.Config.CollectEvents) {
			rbacRules = append(rbacRules, getEventCollectionPolicyRule())
		}

		if datadoghqv1alpha1.BoolValue(dad.Spec.Agent.Config.LeaderElection) {
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
func buildClusterRoleBinding(dad *datadoghqv1alpha1.DatadogAgentDeployment, info roleBindingInfo, agentVersion string) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Labels: getDefaultLabels(dad, info.name, agentVersion),
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
				Namespace: dad.Namespace,
			},
		},
	}
}

// buildClusterAgentClusterRole creates a ClusterRole object for the Cluster Agent based on its config
func buildClusterAgentClusterRole(dad *datadoghqv1alpha1.DatadogAgentDeployment, name, agentVersion string) *rbacv1.ClusterRole {
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Labels: getDefaultLabels(dad, name, agentVersion),
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

	if datadoghqv1alpha1.BoolValue(dad.Spec.Agent.Config.CollectEvents) {
		rbacRules = append(rbacRules, getEventCollectionPolicyRule())
	}

	if datadoghqv1alpha1.BoolValue(dad.Spec.Agent.Config.LeaderElection) {
		rbacRules = append(rbacRules, getLeaderElectionPolicyRule()...)
	}

	if datadoghqv1alpha1.BoolValue(dad.Spec.ClusterAgent.Config.MetricsProviderEnabled) {
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
func buildClusterAgentRole(dad *datadoghqv1alpha1.DatadogAgentDeployment, name, agentVersion string) *rbacv1.Role {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    getDefaultLabels(dad, name, agentVersion),
			Name:      name,
			Namespace: dad.Namespace,
		},
	}

	rbacRules := getLeaderElectionPolicyRule()

	if datadoghqv1alpha1.BoolValue(dad.Spec.ClusterAgent.Config.MetricsProviderEnabled) {
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
