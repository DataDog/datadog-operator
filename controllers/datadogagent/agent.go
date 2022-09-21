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

	"github.com/DataDog/datadog-operator/apis/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	cilium "github.com/DataDog/datadog-operator/pkg/cilium/v1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/version"

	"github.com/DataDog/datadog-operator/controllers/datadogagent/component"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object"

	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
)

func (r *Reconciler) reconcileAgent(logger logr.Logger, features []feature.Feature, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	result, err := r.manageAgentDependencies(logger, dda)
	if utils.ShouldReturn(result, err) {
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

	if !apiutils.BoolValue(dda.Spec.Agent.Enabled) {
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

	if r.options.SupportExtendedDaemonset && apiutils.BoolValue(dda.Spec.Agent.UseExtendedDaemonset) {
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
	updatedEds.Annotations = mergeAnnotationsLabels(logger, eds.GetAnnotations(), newEDS.GetAnnotations(), dda.Spec.Agent.KeepAnnotations)
	updatedEds.Labels = mergeAnnotationsLabels(logger, eds.GetLabels(), newEDS.GetLabels(), dda.Spec.Agent.KeepLabels)

	err = kubernetes.UpdateFromObject(context.TODO(), r.client, updatedEds, eds.ObjectMeta)
	if err != nil {
		return reconcile.Result{}, err
	}
	event := buildEventInfo(updatedEds.Name, updatedEds.Namespace, extendedDaemonSetKind, datadog.UpdateEvent)
	r.recordEvent(dda, event)
	newStatus.Agent = updateExtendedDaemonSetStatus(updatedEds, newStatus.Agent, &now)
	return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
}

func getHashAnnotation(annotations map[string]string) string {
	return annotations[common.MD5AgentDeploymentAnnotationKey]
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
	updatedDS.Annotations = mergeAnnotationsLabels(logger, ds.GetAnnotations(), newDS.GetAnnotations(), dda.Spec.Agent.KeepAnnotations)
	updatedDS.Labels = mergeAnnotationsLabels(logger, ds.GetLabels(), newDS.GetLabels(), dda.Spec.Agent.KeepLabels)

	err = kubernetes.UpdateFromObject(context.TODO(), r.client, updatedDS, ds.ObjectMeta)
	if err != nil {
		return reconcile.Result{}, err
	}
	event := buildEventInfo(updatedDS.Name, updatedDS.Namespace, daemonSetKind, datadog.UpdateEvent)
	r.recordEvent(dda, event)
	newStatus.Agent = updateDaemonSetStatus(updatedDS, newStatus.Agent, &now)
	return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *Reconciler) manageAgentDependencies(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	result, err := r.manageAgentSecret(logger, dda)
	if utils.ShouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageAgentRBACs(logger, dda)
	if utils.ShouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageSystemProbeDependencies(logger, dda)
	if utils.ShouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageConfigMap(logger, dda, getAgentCustomConfigConfigMapName(dda), buildAgentConfigurationConfigMap)
	if utils.ShouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageConfigMap(logger, dda, component.GetInstallInfoConfigMapName(dda), buildInstallInfoConfigMap)
	if utils.ShouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageAgentNetworkPolicy(logger, dda)
	if utils.ShouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageAgentService(logger, dda)
	if utils.ShouldReturn(result, err) {
		return result, err
	}

	return reconcile.Result{}, nil
}

func (r *Reconciler) manageAgentNetworkPolicy(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	spec := dda.Spec.Agent
	builder := agentNetworkPolicyBuilder{dda, spec.NetworkPolicy}
	if !apiutils.BoolValue(spec.Enabled) || spec.NetworkPolicy == nil || !apiutils.BoolValue(spec.NetworkPolicy.Create) {
		return r.cleanupNetworkPolicy(logger, dda, builder.Name())
	}

	return r.ensureNetworkPolicy(logger, dda, builder)
}

type agentNetworkPolicyBuilder struct {
	dda *datadoghqv1alpha1.DatadogAgent
	np  *datadoghqv1alpha1.NetworkPolicySpec
}

func (b agentNetworkPolicyBuilder) Name() string {
	return fmt.Sprintf("%s-%s", b.dda.Name, common.DefaultAgentResourceSuffix)
}

func (b agentNetworkPolicyBuilder) NetworkPolicySpec() *datadoghqv1alpha1.NetworkPolicySpec {
	return b.np
}

func (b agentNetworkPolicyBuilder) BuildKubernetesPolicy() *networkingv1.NetworkPolicy {
	dda := b.dda
	name := b.Name()

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
						IntVal: common.DefaultDogstatsdPort,
					},
					Protocol: &protocolUDP,
				},
			},
		},
	}

	if isAPMEnabled(&dda.Spec) {
		ingressRules = append(ingressRules, networkingv1.NetworkPolicyIngressRule{
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Port: &intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: *dda.Spec.Agent.Apm.HostPort,
					},
					Protocol: &protocolTCP,
				},
			},
		})
	}

	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    object.GetDefaultLabels(dda, name, getAgentVersion(dda)),
			Name:      name,
			Namespace: dda.Namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: b.PodSelector(),
			Ingress:     ingressRules,
			Egress:      egressRules,
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
		},
	}

	return policy
}

func (b agentNetworkPolicyBuilder) PodSelector() metav1.LabelSelector {
	return metav1.LabelSelector{
		MatchLabels: map[string]string{
			kubernetes.AppKubernetesInstanceLabelKey: common.DefaultAgentResourceSuffix,
			kubernetes.AppKubernetesPartOfLabelKey:   object.NewPartOfLabelValue(b.dda).String(),
		},
	}
}

func (b agentNetworkPolicyBuilder) ddFQDNs() []cilium.FQDNSelector {
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
		{
			MatchName: fmt.Sprintf("api.%s", site),
		},
		{
			MatchName: fmt.Sprintf("agent-intake.logs.%s", site),
		},
		{
			MatchName: fmt.Sprintf("agent-http-intake.logs.%s", site),
		},
		{
			MatchName: fmt.Sprintf("process.%s", site),
		},
		{
			MatchName: fmt.Sprintf("orchestrator.%s", site),
		},
	}...)

	return selectors
}

func (b agentNetworkPolicyBuilder) BuildCiliumPolicy() *cilium.NetworkPolicy {
	specs := []cilium.NetworkPolicySpec{
		{
			Description:      "Egress to ECS agent port 51678",
			EndpointSelector: b.PodSelector(),
			Egress: []cilium.EgressRule{
				{
					ToEntities: []cilium.Entity{cilium.EntityHost},
					ToPorts: []cilium.PortRule{
						{
							Ports: []cilium.PortProtocol{
								{
									Port:     "51678",
									Protocol: cilium.ProtocolTCP,
								},
							},
						},
					},
				},
				{
					ToCIDR: []string{"169.254.0.0/16"},
					ToPorts: []cilium.PortRule{
						{
							Ports: []cilium.PortProtocol{
								{
									Port:     "51678",
									Protocol: cilium.ProtocolTCP,
								},
							},
						},
					},
				},
			},
		},
		{
			Description:      "Egress to ntp",
			EndpointSelector: b.PodSelector(),
			Egress: []cilium.EgressRule{
				{
					ToPorts: []cilium.PortRule{
						{
							Ports: []cilium.PortProtocol{
								{
									Port:     "123",
									Protocol: cilium.ProtocolUDP,
								},
							},
						},
					},
					ToFQDNs: []cilium.FQDNSelector{
						{
							MatchPattern: "*.datadog.pool.ntp.org",
						},
					},
				},
			},
		},
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
								{
									Port:     "10516",
									Protocol: cilium.ProtocolTCP,
								},
							},
						},
					},
				},
			},
		},
		{
			Description:      "Egress to kubelet",
			EndpointSelector: b.PodSelector(),
			Egress: []cilium.EgressRule{
				{
					ToEntities: []cilium.Entity{
						cilium.EntityHost,
					},
					ToPorts: []cilium.PortRule{
						{
							Ports: []cilium.PortProtocol{
								{
									Port:     "10250",
									Protocol: cilium.ProtocolTCP,
								},
							},
						},
					},
				},
			},
		},
		{
			Description:      "Ingress for dogstatsd",
			EndpointSelector: b.PodSelector(),
			Ingress: []cilium.IngressRule{
				{
					FromEndpoints: []metav1.LabelSelector{
						{},
					},
					ToPorts: []cilium.PortRule{
						{
							Ports: []cilium.PortProtocol{
								{
									Port:     strconv.Itoa(common.DefaultDogstatsdPort),
									Protocol: cilium.ProtocolUDP,
								},
							},
						},
					},
				},
			},
		},
		ciliumEgressChecks(b),
	}

	if isAPMEnabled(&b.dda.Spec) {
		specs = append(specs, cilium.NetworkPolicySpec{
			Description:      "Ingress for APM trace",
			EndpointSelector: b.PodSelector(),
			Ingress: []cilium.IngressRule{
				{
					FromEndpoints: []metav1.LabelSelector{
						{},
					},
					ToPorts: []cilium.PortRule{
						{
							Ports: []cilium.PortProtocol{
								{
									Port:     strconv.Itoa(int(*b.dda.Spec.Agent.Apm.HostPort)),
									Protocol: cilium.ProtocolUDP,
								},
							},
						},
					},
				},
			},
		})
	}

	return &cilium.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    object.GetDefaultLabels(b.dda, b.Name(), getAgentVersion(b.dda)),
			Name:      b.Name(),
			Namespace: b.dda.Namespace,
		},
		Specs: specs,
	}
}

// newExtendedDaemonSetFromInstance creates an ExtendedDaemonSet from a given DatadogAgent
func newExtendedDaemonSetFromInstance(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, selector *metav1.LabelSelector) (*edsdatadoghqv1alpha1.ExtendedDaemonSet, string, error) {
	template, err := newAgentPodTemplate(logger, dda, selector)
	if err != nil {
		return nil, "", fmt.Errorf("unable to get agent pod template when creating new EDS instance, err: %w", err)
	}
	strategy, err := getAgentDeploymentStrategy(dda)
	if err != nil {
		return nil, "", fmt.Errorf("unable to get Deployment strategy when creating new EDS instance, err: %w", err)
	}
	eds := &edsdatadoghqv1alpha1.ExtendedDaemonSet{
		ObjectMeta: newDaemonsetObjectMetaData(dda),
		Spec: edsdatadoghqv1alpha1.ExtendedDaemonSetSpec{
			Selector: selector,
			Template: *template,
			Strategy: edsdatadoghqv1alpha1.ExtendedDaemonSetSpecStrategy{
				Canary:             strategy.Canary.DeepCopy(),
				ReconcileFrequency: strategy.ReconcileFrequency.DeepCopy(),
				RollingUpdate: edsdatadoghqv1alpha1.ExtendedDaemonSetSpecStrategyRollingUpdate{
					MaxUnavailable:            strategy.RollingUpdate.MaxUnavailable,
					MaxPodSchedulerFailure:    strategy.RollingUpdate.MaxPodSchedulerFailure,
					MaxParallelPodCreation:    strategy.RollingUpdate.MaxParallelPodCreation,
					SlowStartIntervalDuration: strategy.RollingUpdate.SlowStartIntervalDuration,
					SlowStartAdditiveIncrease: strategy.RollingUpdate.SlowStartAdditiveIncrease,
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
	strategy, err := getAgentDeploymentStrategy(dda)
	if err != nil {
		return nil, "", err
	}
	ds := &appsv1.DaemonSet{
		ObjectMeta: newDaemonsetObjectMetaData(dda),
		Spec: appsv1.DaemonSetSpec{
			Selector: selector,
			Template: *template,
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: *strategy.UpdateStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDaemonSet{
					MaxUnavailable: strategy.RollingUpdate.MaxUnavailable,
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
	if apiutils.BoolValue(dda.Spec.Agent.Enabled) && dda.Spec.Agent.DaemonsetName != "" {
		return dda.Spec.Agent.DaemonsetName
	}
	return fmt.Sprintf("%s-%s", dda.Name, "agent")
}

func newDaemonsetObjectMetaData(dda *datadoghqv1alpha1.DatadogAgent) metav1.ObjectMeta {
	labels := object.GetDefaultLabels(dda, common.DefaultAgentResourceSuffix, getAgentVersion(dda))
	labels[common.AgentDeploymentNameLabelKey] = dda.Name
	labels[common.AgentDeploymentComponentLabelKey] = common.DefaultAgentResourceSuffix

	annotations := object.GetDefaultAnnotations(dda)

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
	if !apiutils.BoolValue(dda.Spec.Agent.Enabled) {
		return nil, nil
	}
	return buildConfigurationConfigMap(dda, datadoghqv1alpha1.ConvertCustomConfig(dda.Spec.Agent.CustomConfig), getAgentCustomConfigConfigMapName(dda), common.AgentCustomConfigVolumeSubPath)
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
			Name:        component.GetInstallInfoConfigMapName(dda),
			Namespace:   dda.Namespace,
			Labels:      object.GetDefaultLabels(dda, dda.Name, getAgentVersion(dda)),
			Annotations: object.GetDefaultAnnotations(dda),
		},
		Data: map[string]string{
			"install_info": fmt.Sprintf(installInfoDataTmpl, version.Version),
		},
	}

	return configMap, nil
}
