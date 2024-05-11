// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package podtemplate

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/DataDog/extendeddaemonset/pkg/controller/utils/comparison"
)

const (
	// podTemplateDaemonSetLabelKey declares to the cluster-autoscaler that the pod template corresponds to an extra daemonset.
	podTemplateDaemonSetLabelKey = "cluster-autoscaler.kubernetes.io/daemonset-pod"
	// podTemplateDaemonSetLabelValueTrue used as podTemplateDaemonSetLabelKey label value.
	podTemplateDaemonSetLabelValueTrue = "true"
)

// Reconciler is the internal reconciler for PodTemplate.
type Reconciler struct {
	options  ReconcilerOptions
	client   client.Client
	scheme   *runtime.Scheme
	log      logr.Logger
	recorder record.EventRecorder
}

// ReconcilerOptions provides options read from command line.
type ReconcilerOptions struct{}

// NewReconciler returns a reconciler for PodTemplate.
func NewReconciler(options ReconcilerOptions, client client.Client, scheme *runtime.Scheme, log logr.Logger, recorder record.EventRecorder) (*Reconciler, error) {
	return &Reconciler{
		options:  options,
		client:   client,
		scheme:   scheme,
		log:      log,
		recorder: recorder,
	}, nil
}

// Reconcile creates and updates PodTemplate objects corresponding to ExtendedDaemonSets.
func (r *Reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := r.log.WithValues("Req.NS", request.Namespace, "Req.Name", request.Name)
	reqLogger.Info("Reconciling PodTemplate")

	eds := &datadoghqv1alpha1.ExtendedDaemonSet{}
	err := r.client.Get(context.TODO(), request.NamespacedName, eds)
	if err != nil {
		return reconcile.Result{}, err
	}

	podTpl := &corev1.PodTemplate{}
	err = r.client.Get(context.TODO(), request.NamespacedName, podTpl)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.createPodTemplate(reqLogger, eds)
		}

		return reconcile.Result{}, err
	}

	return r.updatePodTemplateIfNeeded(reqLogger, eds, podTpl)
}

// createPodTemplate creates a new PodTemplate object corresponding to a given ExtendedDaemonSet.
func (r *Reconciler) createPodTemplate(logger logr.Logger, eds *datadoghqv1alpha1.ExtendedDaemonSet) (reconcile.Result, error) {
	podTpl, err := r.newPodTemplate(eds)
	if err != nil {
		return reconcile.Result{}, err
	}

	logger.Info("Creating a new PodTemplate", "podTemplate.Namespace", podTpl.Namespace, "podTemplate.Name", podTpl.Name)

	err = r.client.Create(context.TODO(), podTpl)
	if err != nil {
		return reconcile.Result{}, err
	}

	r.recorder.Event(eds, corev1.EventTypeNormal, "Create PodTemplate", fmt.Sprintf("%s/%s", podTpl.Namespace, podTpl.Name))

	return reconcile.Result{}, nil
}

// updatePodTemplateIfNeeded updates the PodTemplate object accordingly to the given ExtendedDaemonSet.
// It does nothing if the PodTemplate is up-to-date.
func (r *Reconciler) updatePodTemplateIfNeeded(logger logr.Logger, eds *datadoghqv1alpha1.ExtendedDaemonSet, podTpl *corev1.PodTemplate) (reconcile.Result, error) {
	specHash, err := comparison.GenerateMD5PodTemplateSpec(&eds.Spec.Template)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("cannot generate pod template hash: %w", err)
	}

	if podTpl.Annotations != nil && podTpl.Annotations[datadoghqv1alpha1.MD5ExtendedDaemonSetAnnotationKey] == specHash {
		// Already up-to-date
		return reconcile.Result{}, nil
	}

	logger.Info("Updating PodTemplate", "podTemplate.Namespace", podTpl.Namespace, "podTemplate.Name", podTpl.Name)

	newPodTpl, err := r.newPodTemplate(eds)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = r.client.Update(context.TODO(), newPodTpl)
	if err != nil {
		return reconcile.Result{}, err
	}

	r.recorder.Event(eds, corev1.EventTypeNormal, "Update PodTemplate", fmt.Sprintf("%s/%s", podTpl.Namespace, podTpl.Name))

	return reconcile.Result{}, nil
}

// newPodTemplate generates a PodTemplate object based on ExtendedDaemonSet.
func (r *Reconciler) newPodTemplate(eds *datadoghqv1alpha1.ExtendedDaemonSet) (*corev1.PodTemplate, error) {
	podTpl := &corev1.PodTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:        eds.Name,
			Namespace:   eds.Namespace,
			Labels:      eds.Labels,
			Annotations: eds.Annotations,
		},
		Template: *eds.Spec.Template.DeepCopy(),
	}

	specHash, err := comparison.GenerateMD5PodTemplateSpec(&eds.Spec.Template)
	if err != nil {
		return nil, fmt.Errorf("cannot generate pod template hash: %w", err)
	}

	if podTpl.Annotations == nil {
		podTpl.Annotations = make(map[string]string)
	}

	podTpl.Annotations[datadoghqv1alpha1.MD5ExtendedDaemonSetAnnotationKey] = specHash

	if podTpl.Labels == nil {
		podTpl.Labels = make(map[string]string)
	}

	podTpl.Labels[podTemplateDaemonSetLabelKey] = podTemplateDaemonSetLabelValueTrue
	err = controllerutil.SetControllerReference(eds, podTpl, r.scheme)

	return podTpl, err
}
