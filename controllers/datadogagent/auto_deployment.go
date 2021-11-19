// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"

	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	autoconfigGroupIdLabelKey = "operator.datadoghq.com/autoconfig-group"
)

type AutoDeployer struct {
	mgr   manager.Manager
	creds config.Creds
}

type DeployerGroup struct {
	dda     *v1alpha1.DatadogAgent
	groupId string
	nodes   []*corev1.Node
}

var errShouldNotRun = errors.New("autoconfig should not run")

func NewAutoDeployer(mgr manager.Manager, creds config.Creds) *AutoDeployer {
	return &AutoDeployer{mgr: mgr, creds: creds}
}

// Implementing Runnable and LeaderElectionRunnable interfaces
func (ad *AutoDeployer) Start(c context.Context) error {
	ticker := time.NewTicker(60 * time.Second)

	if err := ad.runAutoconfig(ad.mgr.GetLogger(), c); err != nil {
		return err
	}

	for {
		select {
		case <-c.Done():
			return nil

		case <-ticker.C:
			if err := ad.runAutoconfig(ad.mgr.GetLogger(), c); err != nil {
				return err
			}
		}
	}
}

func (ad *AutoDeployer) NeedLeaderElection() bool {
	return true
}

func (ad *AutoDeployer) runAutoconfig(logger logr.Logger, c context.Context) error {
	datadogAgents := v1alpha1.DatadogAgentList{}

	err := ad.mgr.GetClient().List(c, &datadogAgents, &client.ListOptions{Namespace: ""})
	if err != nil {
		logger.Error(err, "Unable to list DatadogAgents, not running autoconfig")
		return nil
	}

	// Maps already existing DatadogAgent to DeployerGroups
	deployerGroups := make(map[string]*DeployerGroup)

	// Do not run if at least one DatadogAgent is not handlded by autoconfig
	shouldRun := true
	for _, datadogAgent := range datadogAgents.Items {
		if groupId, found := datadogAgent.Labels[autoconfigGroupIdLabelKey]; found {
			deployerGroups[groupId] = &DeployerGroup{
				dda:     &datadogAgent,
				groupId: groupId,
			}
		} else {
			shouldRun = false
		}
	}

	if !shouldRun {
		return errShouldNotRun
	}

	// List nodes before reconciling DeployerGroups
	nodes := corev1.NodeList{}
	err = ad.mgr.GetClient().List(c, &nodes)
	if err != nil {
		logger.Error(err, "Unable to list nodes, not running autoconfig")
		return nil
	}

	// Dispatch nodes to group, currently we are quite dumb and only dispatch based on OS
	// Basically group ids are OS name ATM
	for _, node := range nodes.Items {
		groupId := node.Status.NodeInfo.OperatingSystem
		deployerGroup := deployerGroups[groupId]

		if deployerGroup == nil {
			deployerGroup = &DeployerGroup{
				groupId: groupId,
				nodes:   []*corev1.Node{&node},
			}
			deployerGroups[groupId] = deployerGroup
		} else {
			deployerGroup.nodes = append(deployerGroup.nodes, &node)
		}
	}

	for _, deployerGroup := range deployerGroups {
		if err := ad.reconcile(logger.WithName(deployerGroup.groupId), c, deployerGroup); err != nil {
			logger.Error(err, "unable to reconcile deployerGroup")
		}
	}

	return nil
}

func (ad *AutoDeployer) reconcile(logger logr.Logger, c context.Context, group *DeployerGroup) error {
	if group.dda == nil {
		group.dda = &v1alpha1.DatadogAgent{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dd-ad-" + group.groupId,
				Namespace: "default",
			},
		}
	} else {
		group.dda = group.dda.DeepCopy()
	}

	if ad.updateDatadogAgent(group.dda, group.nodes) {
		if group.dda.ResourceVersion == "" {
			return ad.mgr.GetClient().Create(c, group.dda)
		} else {
			return ad.mgr.GetClient().Update(c, group.dda)
		}
	}

	return nil
}

func (ad *AutoDeployer) updateDatadogAgent(dda *v1alpha1.DatadogAgent, nodes []*corev1.Node) bool {
	if len(nodes) == 0 {
		return false
	}

	// Update use case not supported ATM
	if dda.ResourceVersion != "" {
		return false
	}

	// Handle automatic registry selection
	switch {
	case strings.Contains(nodes[0].Spec.ProviderID, "aws"):
		dda.Spec.Registry = v1alpha1.NewStringPointer("public.ecr.aws/datadog")
	}

	dda.Labels = map[string]string{autoconfigGroupIdLabelKey: nodes[0].Status.NodeInfo.OperatingSystem}

	dda.Spec.Credentials = &v1alpha1.AgentCredentials{
		DatadogCredentials: v1alpha1.DatadogCredentials{
			APIKey: ad.creds.APIKey,
			AppKey: ad.creds.AppKey,
		},
	}

	dda.Spec.Agent = v1alpha1.DatadogAgentSpecAgentSpec{
		Config: &v1alpha1.NodeAgentConfig{
			Kubelet: &v1alpha1.KubeletConfig{
				TLSVerify: v1alpha1.NewBoolPointer(false),
			},
		},
		Enabled: v1alpha1.NewBoolPointer(true),
		Apm: &v1alpha1.APMSpec{
			Enabled: v1alpha1.NewBoolPointer(true),
		},
		Process: &v1alpha1.ProcessSpec{
			Enabled:                  v1alpha1.NewBoolPointer(true),
			ProcessCollectionEnabled: v1alpha1.NewBoolPointer(true),
		},
		Log: &v1alpha1.LogCollectionConfig{
			Enabled: v1alpha1.NewBoolPointer(true),
		},
		Affinity: &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{Key: "kubernetes.io/os", Operator: "In", Values: []string{nodes[0].Status.NodeInfo.OperatingSystem}},
							},
						},
					},
				},
			},
		},
	}

	dda.Spec.ClusterAgent = v1alpha1.DatadogAgentSpecClusterAgentSpec{
		Enabled: v1alpha1.NewBoolPointer(true),
		Config: &v1alpha1.ClusterAgentConfig{
			ClusterChecksEnabled: v1alpha1.NewBoolPointer(true),
			CollectEvents:        v1alpha1.NewBoolPointer(true),
		},
	}

	dda.Spec.ClusterChecksRunner = v1alpha1.DatadogAgentSpecClusterChecksRunnerSpec{
		Enabled: v1alpha1.NewBoolPointer(true),
	}

	return true
}
