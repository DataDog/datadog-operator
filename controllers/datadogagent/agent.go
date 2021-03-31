// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"fmt"
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

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/version"
	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
)

func (r *Reconciler) reconcileAgent(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	result, err := r.manageAgentDependencies(logger, dda, newStatus)
	if shouldReturn(result, err) {
		return result, err
	}

	if newStatus.Agent != nil && newStatus.Agent.DaemonsetName != "" && newStatus.Agent.DaemonsetName != daemonsetName(dda) {
		return result, fmt.Errorf("the Datadog agent DaemonSet cannot be renamed once created")
	}

	nameNamespace := types.NamespacedName{
		Name:      daemonsetName(dda),
		Namespace: dda.ObjectMeta.Namespace,
	}
	// check if EDS or DS already exist
	eds := &edsdatadoghqv1alpha1.ExtendedDaemonSet{}
	if r.options.SupportExtendedDaemonset {
		if err2 := r.client.Get(context.TODO(), nameNamespace, eds); err2 != nil {
			if !errors.IsNotFound(err2) {
				return result, err2
			}
			eds = nil
		}
	} else {
		eds = nil
	}

	ds := &appsv1.DaemonSet{}
	if err2 := r.client.Get(context.TODO(), nameNamespace, ds); err2 != nil {
		if !errors.IsNotFound(err2) {
			return result, err2
		}
		ds = nil
	}

	if dda.Spec.Agent == nil {
		if ds != nil {
			if err = r.deleteDaemonSet(logger, dda, ds); err != nil {
				return result, err
			}
		}
		if eds != nil {
			if err = r.deleteExtendedDaemonSet(logger, dda, eds); err != nil {
				return result, err
			}
		}
		newStatus.Agent = nil
		return result, err
	}

	if r.options.SupportExtendedDaemonset && datadoghqv1alpha1.BoolValue(dda.Spec.Agent.UseExtendedDaemonset) {
		if ds != nil {
			// TODO manage properly the migration from DS to EDS
			err = r.deleteDaemonSet(logger, dda, ds)
			if err != nil {
				return result, err
			}
			result.RequeueAfter = 5 * time.Second
			return result, nil
		}
		if eds == nil {
			return r.createNewExtendedDaemonSet(logger, dda, newStatus)
		}

		return r.updateExtendedDaemonSet(logger, dda, eds, newStatus)
	}

	// Case when Daemonset is requested
	if eds != nil && r.options.SupportExtendedDaemonset {
		// if EDS exist delete before creating or updating the Daemonset
		err = r.deleteExtendedDaemonSet(logger, dda, eds)
		if err != nil {
			return result, err
		}
		result.RequeueAfter = 5 * time.Second
		return result, nil
	}
	if ds == nil {
		return r.createNewDaemonSet(logger, dda, newStatus)
	}

	return r.updateDaemonSet(logger, dda, ds, newStatus)
}

func (r *Reconciler) deleteDaemonSet(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, ds *appsv1.DaemonSet) error {
	err := r.client.Delete(context.TODO(), ds)
	if err != nil {
		return err
	}
	logger.Info("Delete DaemonSet", "daemonSet.Namespace", ds.Namespace, "daemonSet.Name", ds.Name)
	event := buildEventInfo(ds.Name, ds.Namespace, daemonSetKind, datadog.DeletionEvent)
	r.recordEvent(dda, event)
	return err
}

func (r *Reconciler) deleteExtendedDaemonSet(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, eds *edsdatadoghqv1alpha1.ExtendedDaemonSet) error {
	err := r.client.Delete(context.TODO(), eds)
	if err != nil {
		return err
	}
	logger.Info("Delete DaemonSet", "extendedDaemonSet.Namespace", eds.Namespace, "extendedDaemonSet.Name", eds.Name)
	event := buildEventInfo(eds.Name, eds.Namespace, extendedDaemonSetKind, datadog.DeletionEvent)
	r.recordEvent(dda, event)
	return err
}

func (r *Reconciler) createNewExtendedDaemonSet(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	var err error
	// ExtendedDaemonSet up to date didn't exist yet, create a new one
	var newEDS *edsdatadoghqv1alpha1.ExtendedDaemonSet
	var hashEDS string
	if newEDS, hashEDS, err = newExtendedDaemonSetFromInstance(logger, dda, nil); err != nil {
		return reconcile.Result{}, err
	}

	// Set ExtendedDaemonSet instance as the owner and controller
	if err = controllerutil.SetControllerReference(dda, newEDS, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	logger.Info("Creating a new ExtendedDaemonSet", "extendedDaemonSet.Namespace", newEDS.Namespace, "extendedDaemonSet.Name", newEDS.Name, "agentdeployment.Status.Agent.CurrentHash", hashEDS)

	err = r.client.Create(context.TODO(), newEDS)
	if err != nil {
		return reconcile.Result{}, err
	}
	event := buildEventInfo(newEDS.Name, newEDS.Namespace, extendedDaemonSetKind, datadog.CreationEvent)
	r.recordEvent(dda, event)
	now := metav1.NewTime(time.Now())
	newStatus.Agent = updateExtendedDaemonSetStatus(newEDS, newStatus.Agent, &now)

	return reconcile.Result{}, nil
}

func (r *Reconciler) createNewDaemonSet(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	var err error
	// DaemonSet up to date didn't exist yet, create a new one
	var newDS *appsv1.DaemonSet
	var hashDS string
	if newDS, hashDS, err = newDaemonSetFromInstance(logger, dda, nil); err != nil {
		return reconcile.Result{}, err
	}

	// Set DaemonSet instance as the owner and controller
	if err = controllerutil.SetControllerReference(dda, newDS, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.Info("Creating a new DaemonSet", "daemonSet.Namespace", newDS.Namespace, "daemonSet.Name", newDS.Name, "agentdeployment.Status.Agent.CurrentHash", hashDS)
	err = r.client.Create(context.TODO(), newDS)
	if err != nil {
		return reconcile.Result{}, err
	}
	event := buildEventInfo(newDS.Name, newDS.Namespace, daemonSetKind, datadog.CreationEvent)
	r.recordEvent(dda, event)
	now := metav1.NewTime(time.Now())
	newStatus.Agent = updateDaemonSetStatus(newDS, newStatus.Agent, &now)

	return reconcile.Result{}, nil
}

func (r *Reconciler) updateExtendedDaemonSet(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, eds *edsdatadoghqv1alpha1.ExtendedDaemonSet, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	now := metav1.NewTime(time.Now())
	newEDS, newHashEDS, err := newExtendedDaemonSetFromInstance(logger, dda, eds.Spec.Selector)
	if err != nil {
		return reconcile.Result{}, err
	}

	if comparison.IsSameSpecMD5Hash(newHashEDS, eds.GetAnnotations()) {
		// no update needed so return, update the status and return
		newStatus.Agent = updateExtendedDaemonSetStatus(eds, newStatus.Agent, &now)
		return reconcile.Result{}, nil
	}

	// Set ExtendedDaemonSet instance as the owner and controller
	if err = controllerutil.SetControllerReference(dda, eds, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.Info("Updating an existing ExtendedDaemonSet", "extendedDaemonSet.Namespace", newEDS.Namespace, "extendedDaemonSet.Name", newEDS.Name)

	// Copy possibly changed fields
	updatedEds := eds.DeepCopy()
	updatedEds.Spec = *newEDS.Spec.DeepCopy()
	for k, v := range newEDS.Annotations {
		updatedEds.Annotations[k] = v
	}
	for k, v := range newEDS.Labels {
		updatedEds.Labels[k] = v
	}

	err = r.client.Update(context.TODO(), updatedEds)
	if err != nil {
		return reconcile.Result{}, err
	}
	event := buildEventInfo(updatedEds.Name, updatedEds.Namespace, extendedDaemonSetKind, datadog.UpdateEvent)
	r.recordEvent(dda, event)
	newStatus.Agent = updateExtendedDaemonSetStatus(updatedEds, newStatus.Agent, &now)
	return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
}

func getHashAnnotation(annotations map[string]string) string {
	return annotations[datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey]
}

func (r *Reconciler) updateDaemonSet(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, ds *appsv1.DaemonSet, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	// Update values from current DS in any case
	newStatus.Agent = updateDaemonSetStatus(ds, newStatus.Agent, nil)

	newDS, newHashDS, err := newDaemonSetFromInstance(logger, dda, ds.Spec.Selector)
	if err != nil {
		return reconcile.Result{}, err
	}
	now := metav1.NewTime(time.Now())
	if comparison.IsSameSpecMD5Hash(newHashDS, ds.GetAnnotations()) {
		// no update needed so update the status and return
		newStatus.Agent = updateDaemonSetStatus(ds, newStatus.Agent, &now)
		return reconcile.Result{}, nil
	}

	// Set DaemonSet instance as the owner and controller
	if err = controllerutil.SetControllerReference(dda, ds, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.Info("Updating an existing DaemonSet", "daemonSet.Namespace", newDS.Namespace, "daemonSet.Name", newDS.Name)

	// Copy possibly changed fields
	updatedDS := ds.DeepCopy()
	updatedDS.Spec = *newDS.Spec.DeepCopy()
	for k, v := range newDS.Annotations {
		updatedDS.Annotations[k] = v
	}
	for k, v := range newDS.Labels {
		updatedDS.Labels[k] = v
	}
	err = r.client.Update(context.TODO(), updatedDS)
	if err != nil {
		return reconcile.Result{}, err
	}
	event := buildEventInfo(updatedDS.Name, updatedDS.Namespace, daemonSetKind, datadog.UpdateEvent)
	r.recordEvent(dda, event)
	newStatus.Agent = updateDaemonSetStatus(updatedDS, newStatus.Agent, &now)
	return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *Reconciler) manageAgentDependencies(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	result, err := r.manageAgentSecret(logger, dda, newStatus)
	if shouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageAgentRBACs(logger, dda)
	if shouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageSystemProbeDependencies(logger, dda)
	if shouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageConfigMap(logger, dda, getAgentCustomConfigConfigMapName(dda), buildAgentConfigurationConfigMap)
	if shouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageConfigMap(logger, dda, getInstallInfoConfigMapName(dda), buildInstallInfoConfigMap)
	if shouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageAgentNetworkPolicy(logger, dda)
	if shouldReturn(result, err) {
		return result, err
	}

	return reconcile.Result{}, nil
}

func (r *Reconciler) manageAgentNetworkPolicy(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	policyName := fmt.Sprintf("%s-%s", dda.Name, datadoghqv1alpha1.DefaultAgentResourceSuffix)

	spec := dda.Spec.Agent
	if spec == nil || !datadoghqv1alpha1.BoolValue(spec.NetworkPolicy.Create) {
		return r.cleanupNetworkPolicy(logger, dda, policyName)
	}

	return r.ensureNetworkPolicy(logger, dda, policyName, buildAgentNetworkPolicy)
}

func buildAgentNetworkPolicy(dda *datadoghqv1alpha1.DatadogAgent, name string) *networkingv1.NetworkPolicy {
	egressRules := []networkingv1.NetworkPolicyEgressRule{
		// Egress to datadog intake and
		// kubeapi server
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

		// The agents are susceptible to connect to any pod that would
		// be annotated with auto-discovery annotations.
		//
		// When a user wants to add a check on one of its pod, he needs
		// to
		// * annotate its pod
		// * add an ingress policy from the agent on its own pod
		// In order to not ask end-users to inject NetworkPolicy on the
		// agent in the agent namespace, the agent must be allowed to
		// probe any pod.
		{},
	}

	protocolUDP := corev1.ProtocolUDP
	protocolTCP := corev1.ProtocolTCP
	ingressRules := []networkingv1.NetworkPolicyIngressRule{
		// Ingress for dogstatsd
		{
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Port: &intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: datadoghqv1alpha1.DefaultDogstatsdPort,
					},
					Protocol: &protocolUDP,
				},
			},
		},
	}

	if isAPMEnabled(&dda.Spec) {
		port := datadoghqv1alpha1.DefaultAPMAgentTCPPort
		if dda.Spec.Agent.Apm.HostPort != nil {
			port = *dda.Spec.Agent.Apm.HostPort
		}

		ingressRules = append(ingressRules, networkingv1.NetworkPolicyIngressRule{
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Port: &intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: port,
					},
					Protocol: &protocolTCP,
				},
			},
		})
	}

	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    getDefaultLabels(dda, name, getAgentVersion(dda)),
			Name:      name,
			Namespace: dda.Namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					kubernetes.AppKubernetesInstanceLabelKey: datadoghqv1alpha1.DefaultAgentResourceSuffix,
					kubernetes.AppKubernetesPartOfLabelKey:   dda.Name,
				},
			},
			Ingress: ingressRules,
			Egress:  egressRules,
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
		},
	}

	return policy
}

// newExtendedDaemonSetFromInstance creates an ExtendedDaemonSet from a given DatadogAgent
func newExtendedDaemonSetFromInstance(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, selector *metav1.LabelSelector) (*edsdatadoghqv1alpha1.ExtendedDaemonSet, string, error) {
	template, err := newAgentPodTemplate(logger, dda, selector)
	if err != nil {
		return nil, "", err
	}
	eds := &edsdatadoghqv1alpha1.ExtendedDaemonSet{
		ObjectMeta: newDaemonsetObjectMetaData(dda),
		Spec: edsdatadoghqv1alpha1.ExtendedDaemonSetSpec{
			Selector: selector,
			Template: *template,
			Strategy: edsdatadoghqv1alpha1.ExtendedDaemonSetSpecStrategy{
				Canary:             dda.Spec.Agent.DeploymentStrategy.Canary.DeepCopy(),
				ReconcileFrequency: dda.Spec.Agent.DeploymentStrategy.ReconcileFrequency.DeepCopy(),
				RollingUpdate: edsdatadoghqv1alpha1.ExtendedDaemonSetSpecStrategyRollingUpdate{
					MaxUnavailable:            dda.Spec.Agent.DeploymentStrategy.RollingUpdate.MaxUnavailable,
					MaxPodSchedulerFailure:    dda.Spec.Agent.DeploymentStrategy.RollingUpdate.MaxPodSchedulerFailure,
					MaxParallelPodCreation:    dda.Spec.Agent.DeploymentStrategy.RollingUpdate.MaxParallelPodCreation,
					SlowStartIntervalDuration: dda.Spec.Agent.DeploymentStrategy.RollingUpdate.SlowStartIntervalDuration,
					SlowStartAdditiveIncrease: dda.Spec.Agent.DeploymentStrategy.RollingUpdate.SlowStartAdditiveIncrease,
				},
			},
		},
	}
	hash, err := comparison.SetMD5DatadogAgentGenerationAnnotation(&eds.ObjectMeta, eds.Spec)
	if err != nil {
		return nil, "", err
	}

	return eds, hash, nil
}

// newDaemonSetFromInstance creates a DaemonSet from a given DatadogAgent
func newDaemonSetFromInstance(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, selector *metav1.LabelSelector) (*appsv1.DaemonSet, string, error) {
	template, err := newAgentPodTemplate(logger, dda, selector)
	if err != nil {
		return nil, "", err
	}

	if selector == nil {
		selector = &metav1.LabelSelector{
			MatchLabels: template.Labels,
		}
	}
	ds := &appsv1.DaemonSet{
		ObjectMeta: newDaemonsetObjectMetaData(dda),
		Spec: appsv1.DaemonSetSpec{
			Selector: selector,
			Template: *template,
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: *dda.Spec.Agent.DeploymentStrategy.UpdateStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDaemonSet{
					MaxUnavailable: dda.Spec.Agent.DeploymentStrategy.RollingUpdate.MaxUnavailable,
				},
			},
		},
	}
	hashDS, err := comparison.SetMD5DatadogAgentGenerationAnnotation(&ds.ObjectMeta, dda.Spec)
	if err != nil {
		return nil, "", err
	}

	return ds, hashDS, nil
}

func daemonsetName(dda *datadoghqv1alpha1.DatadogAgent) string {
	if dda.Spec.Agent != nil && dda.Spec.Agent.DaemonsetName != "" {
		return dda.Spec.Agent.DaemonsetName
	}
	return fmt.Sprintf("%s-%s", dda.Name, "agent")
}

func newDaemonsetObjectMetaData(dda *datadoghqv1alpha1.DatadogAgent) metav1.ObjectMeta {
	labels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultAgentResourceSuffix, getAgentVersion(dda))
	labels[datadoghqv1alpha1.AgentDeploymentNameLabelKey] = dda.Name
	labels[datadoghqv1alpha1.AgentDeploymentComponentLabelKey] = datadoghqv1alpha1.DefaultAgentResourceSuffix
	for key, val := range dda.Labels {
		labels[key] = val
	}
	annotations := map[string]string{}
	for key, val := range dda.Annotations {
		annotations[key] = val
	}

	return metav1.ObjectMeta{
		Name:        daemonsetName(dda),
		Namespace:   dda.Namespace,
		Labels:      labels,
		Annotations: annotations,
	}
}

func getAgentCustomConfigConfigMapName(dda *datadoghqv1alpha1.DatadogAgent) string {
	return fmt.Sprintf("%s-datadog-yaml", dda.Name)
}

func buildAgentConfigurationConfigMap(dda *datadoghqv1alpha1.DatadogAgent) (*corev1.ConfigMap, error) {
	if dda.Spec.Agent == nil {
		return nil, nil
	}
	return buildConfigurationConfigMap(dda, dda.Spec.Agent.CustomConfig, getAgentCustomConfigConfigMapName(dda), datadoghqv1alpha1.AgentCustomConfigVolumeSubPath)
}

func getInstallInfoConfigMapName(dda *datadoghqv1alpha1.DatadogAgent) string {
	return fmt.Sprintf("%s-install-info", dda.Name)
}

const installInfoDataTmpl = `---
install_method:
  tool: datadog-operator
  tool_version: datadog-operator
  installer_version: %s
`

func buildInstallInfoConfigMap(dda *datadoghqv1alpha1.DatadogAgent) (*corev1.ConfigMap, error) {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        getInstallInfoConfigMapName(dda),
			Namespace:   dda.Namespace,
			Labels:      getDefaultLabels(dda, dda.Name, getAgentVersion(dda)),
			Annotations: getDefaultAnnotations(dda),
		},
		Data: map[string]string{
			"install_info": fmt.Sprintf(installInfoDataTmpl, version.Version),
		},
	}

	return configMap, nil
}
