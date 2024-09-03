// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"github.com/DataDog/datadog-operator/controllers/datadogagent/store"

	policyv1beta1 "k8s.io/api/policy/v1beta1"
)

// PodSecurityManager use to manage Security resources.
type PodSecurityManager interface {
	// GetPodSecurityPolicy gets a PodSecurityPolicy
	GetPodSecurityPolicy(namespace string, pspName string) (*policyv1beta1.PodSecurityPolicy, error)
	// UpdatePodSecurityPolicy updates a PodSecurityPolicy
	UpdatePodSecurityPolicy(*policyv1beta1.PodSecurityPolicy)
}

// NewPodSecurityManager return new PodSecurityManager instance
func NewPodSecurityManager(store store.StoreClient) PodSecurityManager {
	manager := &podSecurityManagerImpl{
		store: store,
	}
	return manager
}

// podSecurityManagerImpl is used to manage pod security resources.
type podSecurityManagerImpl struct {
	store store.StoreClient
}

func (m *podSecurityManagerImpl) GetPodSecurityPolicy(namespace string, pspName string) (psp *policyv1beta1.PodSecurityPolicy, err error) {
	// TODO
	// obj, _ := m.store.GetOrCreate(kubernetes.PodSecurityPoliciesKind, namespace, pspName)
	// psp, ok := obj.(*policyv1beta1.PodSecurityPolicy)
	// if !ok {
	// 	return nil, fmt.Errorf("unable to get from the store the PodSecurityPolicy %s", pspName)
	// }
	return psp, err
}

func (m *podSecurityManagerImpl) UpdatePodSecurityPolicy(psp *policyv1beta1.PodSecurityPolicy) {
	// TODO
	// m.store.AddOrUpdate(kubernetes.PodSecurityPoliciesKind, psp)
}
