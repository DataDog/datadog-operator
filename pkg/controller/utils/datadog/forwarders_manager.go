// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadog

import (
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ForwardersManager is a collection of metricsForwarder per DatadogAgentDeployment
// ForwardersManager implements the controller-runtime Runnable interface
type ForwardersManager struct {
	forwarders map[string]*metricsForwarder
	k8sClient  client.Client
	wg         sync.WaitGroup
	sync.Mutex
}

// NewForwardersManager builds a new ForwardersManager
// ForwardersManager implements the controller-runtime Runnable interface
func NewForwardersManager(k8sClient client.Client) *ForwardersManager {
	return &ForwardersManager{
		k8sClient:  k8sClient,
		forwarders: make(map[string]*metricsForwarder),
		wg:         sync.WaitGroup{},
	}
}

// Start must be handled by the controller-runtime manager
func (f *ForwardersManager) Start(stop <-chan struct{}) error {
	<-stop
	f.stopAllForwarders()
	return nil
}

// Register starts a new metricsForwarder if a new DatadogAgentDeployment is detected
func (f *ForwardersManager) Register(namespacedName types.NamespacedName) {
	f.Lock()
	defer f.Unlock()
	id := namespacedName.String()
	if _, found := f.forwarders[id]; !found {
		log.Info("New Datadog metrics forwarder registred", "ID", id)
		f.forwarders[id] = newMetricsForwarder(f.k8sClient, namespacedName)
		f.wg.Add(1)
		go f.forwarders[id].start(&f.wg)
	}
}

// Unregister stops a metricsForwarder when its corresponding DatadogAgentDeployment is deleted
func (f *ForwardersManager) Unregister(namespacedName types.NamespacedName) {
	id := namespacedName.String()
	log.Info("Unregistering metrics forwarder", "ID", id)
	if err := f.unregisterForwarder(id); err != nil {
		log.Error(err, "cannot unregister metrics forwarder", "ID", id)
		return
	}
}

// ProcessError dispatches reconcile errors to their corresponding metric forwarders
// metric forwarders generates reconcile loop metrics based on the errors
func (f *ForwardersManager) ProcessError(namespacedName types.NamespacedName, err error) {
	f.Lock()
	defer f.Unlock()
	id := namespacedName.String()
	forwarder, found := f.forwarders[id]
	if !found {
		log.Error(fmt.Errorf("%s not found", id), "cannot process error")
		return
	}
	if forwarder.isErrChanFull() {
		// Discard sending the error to avoid blocking this method
		log.Error(fmt.Errorf("metrics forwarder %s: blocked error forwarding", id), "cannot process error")
		return
	}
	forwarder.errorChan <- err
}

// ProcessEvent dispatches recorded events to their corresponding metric forwarders
func (f *ForwardersManager) ProcessEvent(namespacedName types.NamespacedName, event Event) {
	f.Lock()
	defer f.Unlock()
	id := namespacedName.String()
	forwarder, found := f.forwarders[id]
	if !found {
		log.Error(fmt.Errorf("%s not found", id), "cannot process event")
		return
	}
	if forwarder.isEventChanFull() {
		// Discard sending the event to avoid blocking this method
		log.Error(fmt.Errorf("metrics forwarder %s: blocked event forwarding", id), "cannot process event")
		return
	}
	forwarder.eventChan <- event
}

// stopAllForwarders stops the running metricsForwarder goroutines
func (f *ForwardersManager) stopAllForwarders() {
	f.Lock()
	defer f.Unlock()
	for id, forwarder := range f.forwarders {
		log.Info("Stopping metrics forwarder", "ID", id)
		forwarder.stop()
	}
	f.wg.Wait()
}

// unregisterForwarder deletes a given metricsForwarder
func (f *ForwardersManager) unregisterForwarder(id string) error {
	f.Lock()
	defer f.Unlock()
	if _, found := f.forwarders[id]; !found {
		return fmt.Errorf("%s not found", id)
	}
	f.forwarders[id].stop()
	delete(f.forwarders, id)
	return nil
}
