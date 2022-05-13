// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"fmt"

	"github.com/DataDog/datadog-operator/controllers/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	securityv1 "github.com/openshift/api/security/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
)

// PodSecurityManager use to manage Security resources.
type PodSecurityManager interface {
	// GetSecurityContextConstraints gets a SecurityContextConstraints
	GetSecurityContextConstraints(namespace string, sccName string) (*securityv1.SecurityContextConstraints, error)
	// UpdateSecurityContextConstraints updates a SecurityContextConstraints
	UpdateSecurityContextConstraints(*securityv1.SecurityContextConstraints)
	// GetPodSecurityPolicy gets a PodSecurityPolicy
	GetPodSecurityPolicy(namespace string, pspName string) (*policyv1beta1.PodSecurityPolicy, error)
	// UpdatePodSecurityPolicy updates a PodSecurityPolicy
	UpdatePodSecurityPolicy(*policyv1beta1.PodSecurityPolicy)
}

// NewPodSecurityManager return new PodSecurityManager instance
func NewPodSecurityManager(store dependencies.StoreClient) PodSecurityManager {
	manager := &podSecurityManagerImpl{
		store: store,
	}
	return manager
}

// podSecurityManagerImpl is used to manage pod security resources.
type podSecurityManagerImpl struct {
	store dependencies.StoreClient
}

func (m *podSecurityManagerImpl) GetSecurityContextConstraints(namespace string, sccName string) (*securityv1.SecurityContextConstraints, error) {
	obj, _ := m.store.GetOrCreate(kubernetes.SecurityContextConstraintsKind, namespace, sccName)
	scc, ok := obj.(*securityv1.SecurityContextConstraints)
	if !ok {
		return nil, fmt.Errorf("unable to get from the store the SecurityContextConstraints %s", sccName)
	}
	return scc, nil
}

func (m *podSecurityManagerImpl) UpdateSecurityContextConstraints(scc *securityv1.SecurityContextConstraints) {
	m.store.AddOrUpdate(kubernetes.SecurityContextConstraintsKind, scc)
}

func (m *podSecurityManagerImpl) GetPodSecurityPolicy(namespace string, pspName string) (*policyv1beta1.PodSecurityPolicy, error) {
	obj, _ := m.store.GetOrCreate(kubernetes.PodSecurityPoliciesKind, namespace, pspName)
	psp, ok := obj.(*policyv1beta1.PodSecurityPolicy)
	if !ok {
		return nil, fmt.Errorf("unable to get from the store the PodSecurityPolicy %s", pspName)
	}
	return psp, nil
}

func (m *podSecurityManagerImpl) UpdatePodSecurityPolicy(psp *policyv1beta1.PodSecurityPolicy) {
	m.store.AddOrUpdate(kubernetes.PodSecurityPoliciesKind, psp)
}
