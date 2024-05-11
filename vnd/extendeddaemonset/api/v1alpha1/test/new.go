// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// Package test contains unit-test helper functions.
package test

import (
	"fmt"
	"time"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
)

// apiVersion datadoghqv1alpha1 api version.
var apiVersion = fmt.Sprintf("%s/%s", datadoghqv1alpha1.GroupVersion.Group, datadoghqv1alpha1.GroupVersion.Version)

// NewExtendedDaemonSetOptions set of option for the ExtendedDaemonset creation.
type NewExtendedDaemonSetOptions struct {
	CreationTime    *time.Time
	Annotations     map[string]string
	Labels          map[string]string
	RollingUpdate   *datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyRollingUpdate
	Canary          *datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary
	Status          *datadoghqv1alpha1.ExtendedDaemonSetStatus
	PodTemplateSpec *corev1.PodTemplateSpec
}

// NewExtendedDaemonSet return new ExtendedDDaemonset instance for test purpose.
func NewExtendedDaemonSet(ns, name string, options *NewExtendedDaemonSetOptions) *datadoghqv1alpha1.ExtendedDaemonSet {
	dd := &datadoghqv1alpha1.ExtendedDaemonSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ExtendedDaemonSet",
			APIVersion: apiVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   ns,
			Name:        name,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
	}
	if options != nil {
		if options.CreationTime != nil {
			dd.CreationTimestamp = metav1.NewTime(*options.CreationTime)
		}
		if options.Annotations != nil {
			for key, value := range options.Annotations {
				dd.Annotations[key] = value
			}
		}
		if options.Labels != nil {
			for key, value := range options.Labels {
				dd.Labels[key] = value
			}
		}
		if options.RollingUpdate != nil {
			dd.Spec.Strategy.RollingUpdate = *options.RollingUpdate
		}
		if options.Canary != nil {
			dd.Spec.Strategy.Canary = options.Canary
		}
		if options.Status != nil {
			dd.Status = *options.Status
		}
		if options.PodTemplateSpec != nil {
			dd.Spec.Template = *options.PodTemplateSpec
		}
	}

	return dd
}

// NewExtendedDaemonSetReplicaSetOptions set of option for the ExtendedDaemonsetReplicaSet creation.
type NewExtendedDaemonSetReplicaSetOptions struct {
	CreationTime *time.Time
	Annotations  map[string]string
	Labels       map[string]string
	GenerateName string
	OwnerRefName string
	Status       *datadoghqv1alpha1.ExtendedDaemonSetReplicaSetStatus
}

// NewExtendedDaemonSetReplicaSet returns new ExtendedDaemonSetReplicaSet instance for testing purpose.
func NewExtendedDaemonSetReplicaSet(ns, name string, options *NewExtendedDaemonSetReplicaSetOptions) *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet {
	dd := &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ExtendedDaemonSetReplicaSet",
			APIVersion: apiVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   ns,
			Name:        name,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
	}
	if options != nil {
		if options.GenerateName != "" {
			dd.GenerateName = options.GenerateName
		}
		if options.CreationTime != nil {
			dd.CreationTimestamp = metav1.NewTime(*options.CreationTime)
		}
		if options.Annotations != nil {
			for key, value := range options.Annotations {
				dd.Annotations[key] = value
			}
		}
		if options.Labels != nil {
			for key, value := range options.Labels {
				dd.Labels[key] = value
			}
		}
		if options.OwnerRefName != "" {
			dd.OwnerReferences = []metav1.OwnerReference{
				{
					Name: options.OwnerRefName,
					Kind: "ExtendedDaemonSet",
				},
			}
		}
		if options.Status != nil {
			dd.Status = *options.Status
		}
	}

	return dd
}

// NewExtendedDaemonsetSettingOptions used to provide creation options to the NewExtendedDaemonsetSetting function.
type NewExtendedDaemonsetSettingOptions struct {
	CreationTime        time.Time
	Selector            map[string]string
	Resources           map[string]corev1.ResourceRequirements
	SelectorRequirement []metav1.LabelSelectorRequirement
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
		edsNode.ObjectMeta.CreationTimestamp = metav1.Time{Time: options.CreationTime}
		if options.Selector != nil || len(options.SelectorRequirement) > 0 {
			edsNode.Spec.NodeSelector = metav1.LabelSelector{}
			if options.Selector != nil {
				edsNode.Spec.NodeSelector.MatchLabels = options.Selector
			}
			if len(options.SelectorRequirement) > 0 {
				edsNode.Spec.NodeSelector.MatchExpressions = options.SelectorRequirement
			}
		}

		for key, val := range options.Resources {
			edsNode.Spec.Containers = append(edsNode.Spec.Containers, datadoghqv1alpha1.ExtendedDaemonsetSettingContainerSpec{Name: key, Resources: val})
		}
	}

	return edsNode
}
