// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"fmt"

	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	securityv1 "github.com/openshift/api/security/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
)

// PodSecurityManager use to manage Security resources.
type PodSecurityManager interface {
	// AddSecurityContextConstraints updates a SecurityContextConstraints
	AddSecurityContextConstraints(name, namespace string, sccUpdates *securityv1.SecurityContextConstraints) error
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

func (m *podSecurityManagerImpl) AddSecurityContextConstraints(name, namespace string, sccUpdates *securityv1.SecurityContextConstraints) error {
	if sccUpdates == nil {
		return nil
	}

	obj, _ := m.store.GetOrCreate(kubernetes.SecurityContextConstraintsKind, namespace, name)
	scc, ok := obj.(*securityv1.SecurityContextConstraints)
	if !ok {
		return fmt.Errorf("unable to get from the store the SecurityContextConstraints %s/%s", namespace, name)
	}

	if sccUpdates.Priority != nil {
		scc.Priority = sccUpdates.Priority
	}
	scc.AllowPrivilegedContainer = apiutils.BoolValue(&sccUpdates.AllowPrivilegedContainer)
	if len(sccUpdates.DefaultAddCapabilities) > 0 {
		scc.DefaultAddCapabilities = append(scc.DefaultAddCapabilities, sccUpdates.DefaultAddCapabilities...)
	}
	if len(sccUpdates.RequiredDropCapabilities) > 0 {
		scc.RequiredDropCapabilities = append(scc.RequiredDropCapabilities, sccUpdates.RequiredDropCapabilities...)
	}
	if len(sccUpdates.AllowedCapabilities) > 0 {
		scc.AllowedCapabilities = append(scc.AllowedCapabilities, sccUpdates.AllowedCapabilities...)
	}
	scc.AllowHostDirVolumePlugin = apiutils.BoolValue(&sccUpdates.AllowHostDirVolumePlugin)
	if len(sccUpdates.Volumes) > 0 {
		scc.Volumes = append(scc.Volumes, sccUpdates.Volumes...)
	}
	if len(sccUpdates.AllowedFlexVolumes) > 0 {
		scc.AllowedFlexVolumes = append(scc.AllowedFlexVolumes, sccUpdates.AllowedFlexVolumes...)
	}
	scc.AllowHostNetwork = apiutils.BoolValue(&sccUpdates.AllowHostNetwork)
	scc.AllowHostPorts = apiutils.BoolValue(&sccUpdates.AllowHostPorts)
	scc.AllowHostPID = apiutils.BoolValue(&sccUpdates.AllowHostPID)
	scc.AllowHostIPC = apiutils.BoolValue(&sccUpdates.AllowHostIPC)
	if sccUpdates.SELinuxContext.Type != "" {
		scc.SELinuxContext.Type = sccUpdates.SELinuxContext.Type
	}
	if sccUpdates.SELinuxContext.SELinuxOptions != nil {
		scc.SELinuxContext.SELinuxOptions = sccUpdates.SELinuxContext.SELinuxOptions
	}
	if sccUpdates.RunAsUser.Type != "" {
		scc.RunAsUser.Type = sccUpdates.RunAsUser.Type
	}
	if sccUpdates.RunAsUser.UID != nil {
		scc.RunAsUser.UID = sccUpdates.RunAsUser.UID
	}
	if sccUpdates.RunAsUser.UIDRangeMin != nil {
		scc.RunAsUser.UIDRangeMin = sccUpdates.RunAsUser.UIDRangeMin
	}
	if sccUpdates.RunAsUser.UIDRangeMax != nil {
		scc.RunAsUser.UIDRangeMax = sccUpdates.RunAsUser.UIDRangeMax
	}
	if sccUpdates.SupplementalGroups.Type != "" {
		scc.SupplementalGroups.Type = sccUpdates.SupplementalGroups.Type
	}
	if len(sccUpdates.SupplementalGroups.Ranges) > 0 {
		scc.SupplementalGroups.Ranges = append(scc.SupplementalGroups.Ranges, sccUpdates.SupplementalGroups.Ranges...)
	}
	if sccUpdates.FSGroup.Type != "" {
		scc.FSGroup.Type = sccUpdates.FSGroup.Type
	}
	if len(sccUpdates.FSGroup.Ranges) > 0 {
		scc.FSGroup.Ranges = append(scc.FSGroup.Ranges, sccUpdates.FSGroup.Ranges...)
	}
	scc.ReadOnlyRootFilesystem = apiutils.BoolValue(&sccUpdates.ReadOnlyRootFilesystem)
	if len(sccUpdates.Users) > 0 {
		scc.Users = append(scc.Users, sccUpdates.Users...)
	}
	if len(sccUpdates.Groups) > 0 {
		scc.Groups = append(scc.Groups, sccUpdates.Groups...)
	}
	if len(sccUpdates.SeccompProfiles) > 0 {
		scc.SeccompProfiles = append(scc.SeccompProfiles, sccUpdates.SeccompProfiles...)
	}

	return m.store.AddOrUpdate(kubernetes.SecurityContextConstraintsKind, scc)
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
