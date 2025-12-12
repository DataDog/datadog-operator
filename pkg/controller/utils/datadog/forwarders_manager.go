// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadog

import (
	"context"
	"fmt"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/secrets"
)

// MetricsForwardersManager defines interface for metrics forwarding
type MetricsForwardersManager interface {
	Register(client.Object)
	Unregister(client.Object)
	ProcessError(client.Object, error)
	ProcessEvent(client.Object, Event)
	MetricsForwarderStatusForObj(obj client.Object) *ConditionCommon
	SetEnabledFeatures(obj client.Object, features []string)
}

// ForwardersManager is a collection of metricsForwarder per DatadogAgent
// ForwardersManager implements the controller-runtime Runnable interface
type ForwardersManager struct {
	k8sClient                   client.Client
	platformInfo                *kubernetes.PlatformInfo
	metricsForwarders           map[string]*metricsForwarder
	datadogAgentInternalEnabled bool
	// TODO expand this to include a metadataForwarder
	decryptor    secrets.Decryptor
	credsManager *config.CredentialManager
	wg           sync.WaitGroup
	sync.Mutex
}

// NewForwardersManager builds a new ForwardersManager object
// ForwardersManager implements the controller-runtime Runnable interface
func NewForwardersManager(k8sClient client.Client, platformInfo *kubernetes.PlatformInfo, datadogAgentInternalEnabled bool, credsManager *config.CredentialManager) *ForwardersManager {
	return &ForwardersManager{
		k8sClient:                   k8sClient,
		platformInfo:                platformInfo,
		metricsForwarders:           make(map[string]*metricsForwarder),
		datadogAgentInternalEnabled: datadogAgentInternalEnabled,
		decryptor:                   secrets.NewSecretBackend(),
		wg:                          sync.WaitGroup{},
		credsManager:                credsManager,
	}
}

// Start must be handled by the controller-runtime manager
func (f *ForwardersManager) Start(stop <-chan struct{}) error {
	<-stop
	f.stopAllForwarders()

	return nil
}

// Register starts a new metricsForwarder if a new MonitoredObject is detected
func (f *ForwardersManager) Register(obj client.Object) {
	f.Lock()
	defer f.Unlock()
	id := getObjID(obj) // nolint: ifshort
	if _, found := f.metricsForwarders[id]; !found {
		log.Info("New Datadog metrics forwarder registered", "ID", id)
		f.metricsForwarders[id] = newMetricsForwarder(f.k8sClient, f.decryptor, obj, f.platformInfo, f.datadogAgentInternalEnabled, f.credsManager)
		f.wg.Add(1)
		go f.metricsForwarders[id].start(&f.wg)
	}
}

// Unregister stops a metricsForwarder when its corresponding MonitoredObject is deleted
func (f *ForwardersManager) Unregister(obj client.Object) {
	id := getObjID(obj)
	log.Info("Unregistering metrics forwarder", "ID", id)
	if err := f.unregisterForwarder(id); err != nil {
		log.Error(err, "cannot unregister metrics forwarder", "ID", id)

		return
	}
}

// ProcessError dispatches reconcile errors to their corresponding metric forwarders
// metric forwarders generates reconcile loop metrics based on the errors
func (f *ForwardersManager) ProcessError(obj client.Object, reconcileErr error) {
	id := getObjID(obj)
	forwarder, err := f.getForwarder(id)
	if err != nil {
		// Only auto-register if the object still exists and isn't being deleted
		if f.shouldAutoRegister(obj) {
			log.Info("Forwarder not found for error processing, attempting to register", "ID", id)
			f.Register(obj)

			// Try again after registration
			forwarder, err = f.getForwarder(id)
			if err != nil {
				log.Error(err, "cannot process error even after registration", "ID", id)
				return
			}
		} else {
			log.Info("Skipping auto-registration for deleted/non-existent object during error processing", "ID", id)
			return
		}
	}
	if forwarder.isErrChanFull() {
		// Discard sending the error to avoid blocking this method
		log.Error(fmt.Errorf("metrics forwarder %s: blocked error forwarding", id), "cannot process error")

		return
	}
	forwarder.errorChan <- reconcileErr
}

// ProcessEvent dispatches recorded events to their corresponding metric forwarders
func (f *ForwardersManager) ProcessEvent(obj client.Object, event Event) {
	id := getObjID(obj)
	forwarder, err := f.getForwarder(id)
	if err != nil {
		log.Error(err, "cannot process event")

		return
	}
	if forwarder.isEventChanFull() {
		// Discard sending the event to avoid blocking this method
		log.Error(fmt.Errorf("metrics forwarder %s: blocked event forwarding", id), "cannot process event")

		return
	}
	forwarder.eventChan <- event
}

// MetricsForwarderStatusForObj returns the status of the metrics forwarder for a given object
func (f *ForwardersManager) MetricsForwarderStatusForObj(obj client.Object) *ConditionCommon {
	id := getObjID(obj)
	forwarder, err := f.getForwarder(id)
	if err != nil {
		// forwarder not present yet
		return nil
	}

	return forwarder.getStatus()
}

// stopAllForwarders stops the running metricsForwarder goroutines
func (f *ForwardersManager) stopAllForwarders() {
	f.Lock()
	defer f.Unlock()
	for id, forwarder := range f.metricsForwarders {
		log.Info("Stopping metrics forwarder", "ID", id)
		forwarder.stop()
	}
	f.wg.Wait()
}

// unregisterForwarder deletes a given metricsForwarder
func (f *ForwardersManager) unregisterForwarder(id string) error {
	f.Lock()
	defer f.Unlock()
	if _, found := f.metricsForwarders[id]; !found {
		return fmt.Errorf("%s not found", id)
	}
	f.metricsForwarders[id].stop()
	delete(f.metricsForwarders, id)

	return nil
}

// getForwarder returns a metrics forwarder by ID
func (f *ForwardersManager) getForwarder(id string) (*metricsForwarder, error) {
	f.Lock()
	defer f.Unlock()
	forwarder, found := f.metricsForwarders[id]
	if !found {
		return nil, fmt.Errorf("%s not found", id)
	}

	return forwarder, nil
}

// SetEnabledFeatures sets the enabled features for a given object
func (f *ForwardersManager) SetEnabledFeatures(dda client.Object, features []string) {
	id := getObjID(dda)
	forwarder, err := f.getForwarder(id)
	if err != nil {
		// Only auto-register if the object still exists and isn't being deleted
		if f.shouldAutoRegister(dda) {
			log.Info("Forwarder not found, attempting to register", "ID", id)
			f.Register(dda)

			// Try again after registration
			forwarder, err = f.getForwarder(id)
			if err != nil {
				log.Error(err, "cannot set enabled features for object even after registration", "ID", id)
				return
			}
		} else {
			log.Info("Skipping auto-registration for deleted/non-existent object", "ID", id)
			return
		}
	}
	forwarder.setEnabledFeatures(features)
}

// shouldAutoRegister checks if we should auto-register a forwarder for the given object
// Returns false if the object is being deleted or doesn't exist
func (f *ForwardersManager) shouldAutoRegister(obj client.Object) bool {
	// Don't auto-register for objects that are being deleted
	if obj.GetDeletionTimestamp() != nil {
		return false
	}

	// Verify the object still exists in the cluster
	namespacedName := GetNamespacedName(obj)
	switch obj.(type) {
	case *v2alpha1.DatadogAgent:
		existingObj := &v2alpha1.DatadogAgent{}
		if err := f.k8sClient.Get(context.TODO(), namespacedName, existingObj); err != nil {
			return false
		}
	case *v1alpha1.DatadogAgentInternal:
		existingObj := &v1alpha1.DatadogAgentInternal{}
		if err := f.k8sClient.Get(context.TODO(), namespacedName, existingObj); err != nil {
			return false
		}
	default:
		// For unknown types, attempt auto-registration (safe default)
		// This should never happen as we only register features for DatadogAgent and DatadogAgentInternal
		return true
	}

	return true
}
