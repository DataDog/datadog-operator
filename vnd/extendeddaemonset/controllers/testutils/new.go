// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package testutils

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
)

// NewExtendedDaemonsetOptions used to provide creation options to the NewExtendedDaemonset function.
type NewExtendedDaemonsetOptions struct {
	ExtraLabels        map[string]string
	ExtraAnnotations   map[string]string
	CanaryStrategy     *datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary
	RollingUpdate      *datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyRollingUpdate
	ReconcileFrequency *metav1.Duration
}

// NewExtendedDaemonset returns new ExtendedDaemonSet instance.
func NewExtendedDaemonset(ns, name, image string, options *NewExtendedDaemonsetOptions) *datadoghqv1alpha1.ExtendedDaemonSet {
	newDaemonset := &datadoghqv1alpha1.ExtendedDaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: datadoghqv1alpha1.ExtendedDaemonSetSpec{
			Strategy: datadoghqv1alpha1.ExtendedDaemonSetSpecStrategy{
				ReconcileFrequency: &metav1.Duration{
					Duration: 2 * time.Second,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "name"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image: image,
						Name:  "main",
						Ports: []corev1.ContainerPort{{
							ContainerPort: 80,
							Protocol:      "TCP",
							Name:          "http",
						}},
					}},
					Tolerations: []corev1.Toleration{
						{
							Operator: corev1.TolerationOpExists,
						},
					},
				},
			},
		},
	}
	if options != nil {
		newDaemonset.Spec.Strategy.Canary = options.CanaryStrategy
		if options.RollingUpdate != nil {
			newDaemonset.Spec.Strategy.RollingUpdate = *options.RollingUpdate
		}

		if options.ExtraLabels != nil {
			if newDaemonset.ObjectMeta.Labels == nil {
				newDaemonset.ObjectMeta.Labels = map[string]string{}
			}
			for key, val := range options.ExtraLabels {
				newDaemonset.ObjectMeta.Labels[key] = val
			}
		}

		if options.ExtraAnnotations != nil {
			if newDaemonset.ObjectMeta.Annotations == nil {
				newDaemonset.ObjectMeta.Annotations = map[string]string{}
			}
			for key, val := range options.ExtraAnnotations {
				newDaemonset.ObjectMeta.Annotations[key] = val
			}
		}

		if options.ReconcileFrequency != nil {
			newDaemonset.Spec.Strategy.ReconcileFrequency = options.ReconcileFrequency
		}
	}

	return newDaemonset
}

// NewExtendedDaemonsetSettingOptions used to provide creation options to the NewExtendedDaemonsetSetting function.
type NewExtendedDaemonsetSettingOptions struct {
	Selector  map[string]string
	Resources map[string]corev1.ResourceRequirements
}

// NewExtendedDaemonsetSetting returns new ExtendedDaemonsetSetting instance.
func NewExtendedDaemonsetSetting(ns, name, reference string, options *NewExtendedDaemonsetSettingOptions) *datadoghqv1alpha1.ExtendedDaemonsetSetting {
	edsNode := &datadoghqv1alpha1.ExtendedDaemonsetSetting{}
	edsNode.Name = name
	edsNode.Namespace = ns
	edsNode.Spec.Reference = &autoscalingv1.CrossVersionObjectReference{
		Name: reference,
		Kind: "ExtendedDaemonset",
	}
	if options != nil {
		if options.Selector != nil {
			edsNode.Spec.NodeSelector = metav1.LabelSelector{
				MatchLabels: options.Selector,
			}
		}

		for key, val := range options.Resources {
			edsNode.Spec.Containers = append(edsNode.Spec.Containers, datadoghqv1alpha1.ExtendedDaemonsetSettingContainerSpec{Name: key, Resources: val})
		}
	}

	return edsNode
}

// NewDaemonsetOptions used to provide creation options to the NewdDaemonset function.
type NewDaemonsetOptions struct {
	ExtraLabels map[string]string
}

// NewDaemonset returns new ExtendedDaemonSet instance.
func NewDaemonset(ns, name, image string, options *NewDaemonsetOptions) *appsv1.DaemonSet {
	newDaemonset := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name, "daemonset": "true"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": name, "daemonset": "true"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image: image,
						Name:  "main",
						Ports: []corev1.ContainerPort{{
							ContainerPort: 80,
							Protocol:      "TCP",
							Name:          "http",
						}},
					}},
					Tolerations: []corev1.Toleration{
						{
							Operator: corev1.TolerationOpExists,
						},
					},
				},
			},
		},
	}
	if options != nil {
		if options.ExtraLabels != nil {
			if newDaemonset.ObjectMeta.Labels == nil {
				newDaemonset.ObjectMeta.Labels = map[string]string{}
			}
			for key, val := range options.ExtraLabels {
				newDaemonset.ObjectMeta.Labels[key] = val
			}
		}
	}

	return newDaemonset
}
