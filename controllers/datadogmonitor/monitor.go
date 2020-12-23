// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package datadogmonitor

import (
	"fmt"
	"net/url"
	"sort"

	datadogapiclientv1 "github.com/DataDog/datadog-api-client-go/api/v1/datadog"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
)

func buildMonitor(dm *datadoghqv1alpha1.DatadogMonitor) (*datadogapiclientv1.Monitor, *datadogapiclientv1.MonitorUpdateRequest) {
	monitorType := datadogapiclientv1.MonitorType(string(dm.Spec.Type))
	query := dm.Spec.Query
	name := dm.Spec.Name
	msg := dm.Spec.Message

	// TODO
	o := datadogapiclientv1.MonitorOptions{}

	m := datadogapiclientv1.NewMonitor()
	m.SetType(monitorType)
	m.SetQuery(query)
	m.SetName(name)
	m.SetMessage(msg)
	m.SetOptions(o)

	u := datadogapiclientv1.NewMonitorUpdateRequest()
	u.SetType(monitorType)
	u.SetQuery(query)
	u.SetName(name)
	u.SetMessage(msg)
	u.SetOptions(o)

	tags := dm.Spec.Tags
	sort.Strings(tags)
	m.SetTags(tags)
	u.SetTags(tags)

	return m, u
}

func (r *Reconciler) getMonitor(monitorID int) (datadogapiclientv1.Monitor, error) {
	auth := r.datadogAuth

	m, _, err := r.datadogClient.MonitorsApi.GetMonitor(auth, int64(monitorID)).GroupStates("all").Execute()
	if err != nil {
		return datadogapiclientv1.Monitor{}, translateClientError(err, "error getting monitor")
	}

	return m, nil
}

func (r *Reconciler) validateMonitor(dm *datadoghqv1alpha1.DatadogMonitor) error {
	m, _ := buildMonitor(dm)
	if _, _, err := r.datadogClient.MonitorsApi.ValidateMonitor(r.datadogAuth).Body(*m).Execute(); err != nil {
		return translateClientError(err, "error validating monitor")
	}
	return nil
}

func (r *Reconciler) createMonitor(dm *datadoghqv1alpha1.DatadogMonitor) (datadogapiclientv1.Monitor, error) {
	m, _ := buildMonitor(dm)
	mCreated, _, err := r.datadogClient.MonitorsApi.CreateMonitor(r.datadogAuth).Body(*m).Execute()
	if err != nil {
		return datadogapiclientv1.Monitor{}, translateClientError(err, "error creating monitor")
	}

	return mCreated, nil
}

func (r *Reconciler) deleteMonitor(monitorID int) error {
	_, _, err := r.datadogClient.MonitorsApi.DeleteMonitor(r.datadogAuth, int64(monitorID)).Execute()

	if err != nil {
		return translateClientError(err, "error deleting monitor")
	}

	return nil
}

func (r *Reconciler) updateMonitor(dm *datadoghqv1alpha1.DatadogMonitor) (datadogapiclientv1.Monitor, error) {
	_, u := buildMonitor(dm)

	mUpdated, _, err := r.datadogClient.MonitorsApi.UpdateMonitor(r.datadogAuth, int64(dm.Status.ID)).Body(*u).Execute()
	if err != nil {
		return datadogapiclientv1.Monitor{}, translateClientError(err, "error updating monitor")
	}

	// TODO additional logic to handle downtimes (and silenced param if needed)

	return mUpdated, nil
}

// inspired by https://github.com/DataDog/terraform-provider-datadog/blob/faa0d3c32145b288e53de5e38396ff393c47ba82/datadog/provider.go#L262
func translateClientError(err error, msg string) error {
	if msg == "" {
		msg = "an error occurred"
	}

	if apiErr, ok := err.(datadogapiclientv1.GenericOpenAPIError); ok {
		return fmt.Errorf(msg+": %v: %s", err, apiErr.Body())
	}

	if errURL, ok := err.(*url.Error); ok {
		return fmt.Errorf(msg+" (url.Error): %s", errURL)
	}

	return fmt.Errorf(msg+": %s", err.Error())
}
