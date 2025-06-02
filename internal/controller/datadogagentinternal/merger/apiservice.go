// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"fmt"

	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// APIServiceManager is used to manage service resources.
type APIServiceManager interface {
	AddAPIService(name, namespace string, spec apiregistrationv1.APIServiceSpec) error
}

// NewAPIServiceManager returns a new APIServiceManager instance
func NewAPIServiceManager(store store.StoreClient) APIServiceManager {
	manager := &apiServiceManagerImpl{
		store: store,
	}
	return manager
}

// apiServiceManagerImpl is used to manage service resources.
type apiServiceManagerImpl struct {
	store store.StoreClient
}

// AddAPIService creates or updates service
func (m *apiServiceManagerImpl) AddAPIService(name, namespace string, spec apiregistrationv1.APIServiceSpec) error {
	obj, _ := m.store.GetOrCreate(kubernetes.APIServiceKind, "", name)
	apiService, ok := obj.(*apiregistrationv1.APIService)
	if !ok {
		return fmt.Errorf("unable to get from the store the APIService %s", name)
	}

	apiService.Spec = spec

	return m.store.AddOrUpdate(kubernetes.APIServiceKind, apiService)
}
