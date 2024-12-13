// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadog

import (
	"fmt"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/secrets"
)

// MetricForwardersManager defines interface for metrics forwarding
type MetricForwardersManager interface {
	Register(client.Object)
	Unregister(MonitoredObject)
	ProcessError(MonitoredObject, error)
	ProcessEvent(MonitoredObject, Event)
	MetricsForwarderStatusForObj(obj MonitoredObject) *ConditionCommon
	SetEnabledFeatures(obj MonitoredObject, features []feature.Feature)
}

// ForwardersManager is a collection of metricsForwarder per DatadogAgent
// ForwardersManager implements the controller-runtime Runnable interface
type ForwardersManager struct {
	k8sClient    client.Client
	platformInfo *kubernetes.PlatformInfo
	forwarders   map[string]*metricsForwarder
	// TODO expand this to include a metadataForwarder
	decryptor secrets.Decryptor
	wg        sync.WaitGroup
	sync.Mutex
}

// NewForwardersManager builds a new ForwardersManager object
// ForwardersManager implements the controller-runtime Runnable interface
func NewForwardersManager(k8sClient client.Client, platformInfo *kubernetes.PlatformInfo) *ForwardersManager {
	return &ForwardersManager{
		k8sClient:    k8sClient,
		platformInfo: platformInfo,
		forwarders:   make(map[string]*metricsForwarder),
		decryptor:    secrets.NewSecretBackend(),
		wg:           sync.WaitGroup{},
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
	if _, found := f.forwarders[id]; !found {
		log.Info("New Datadog metrics forwarder registered", "ID", id)
		f.forwarders[id] = newMetricsForwarder(f.k8sClient, f.decryptor, obj, obj.GetObjectKind(), f.platformInfo)
		f.wg.Add(1)
		go f.forwarders[id].start(&f.wg)
	}
}

// Unregister stops a metricsForwarder when its corresponding MonitoredObject is deleted
func (f *ForwardersManager) Unregister(obj MonitoredObject) {
	id := getObjID(obj)
	log.Info("Unregistering metrics forwarder", "ID", id)
	if err := f.unregisterForwarder(id); err != nil {
		log.Error(err, "cannot unregister metrics forwarder", "ID", id)

		return
	}
}

// ProcessError dispatches reconcile errors to their corresponding metric forwarders
// metric forwarders generates reconcile loop metrics based on the errors
func (f *ForwardersManager) ProcessError(obj MonitoredObject, reconcileErr error) {
	id := getObjID(obj)
	forwarder, err := f.getForwarder(id)
	if err != nil {
		log.Error(err, "cannot process error")

		return
	}
	if forwarder.isErrChanFull() {
		// Discard sending the error to avoid blocking this method
		log.Error(fmt.Errorf("metrics forwarder %s: blocked error forwarding", id), "cannot process error")

		return
	}
	forwarder.errorChan <- reconcileErr
}

// ProcessEvent dispatches recorded events to their corresponding metric forwarders
func (f *ForwardersManager) ProcessEvent(obj MonitoredObject, event Event) {
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

// MetricsForwarderStatusForObj used to retrieve the Metrics forwarder status for a given object
func (f *ForwardersManager) MetricsForwarderStatusForObj(obj MonitoredObject) *ConditionCommon {
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

// getForwarder returns a metrics forwarder by ID
func (f *ForwardersManager) getForwarder(id string) (*metricsForwarder, error) {
	f.Lock()
	defer f.Unlock()
	forwarder, found := f.forwarders[id]
	if !found {
		return nil, fmt.Errorf("%s not found", id)
	}

	return forwarder, nil
}

// SetEnabledFeatures updates the list of enabled features for a namespaced object
func (f *ForwardersManager) SetEnabledFeatures(dda MonitoredObject, features []feature.Feature) {
	id := getObjID(dda)
	mf, err := f.getForwarder(id)
	if err != nil {
		log.Error(err, "cannot process error")
	}

	mf.Lock()
	defer mf.Unlock()
	var featureList []string
	for _, feature := range features {
		featureList = append(featureList, string(feature.ID()))
	}
	if len(mf.EnabledFeatures) == 0 {
		mf.EnabledFeatures = make(map[string][]string)
	}
	mf.EnabledFeatures[id] = featureList
}
