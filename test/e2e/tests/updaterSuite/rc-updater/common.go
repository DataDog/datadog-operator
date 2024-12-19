// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package api provides test helpers to interact with the Datadog API
package updater

import (
	"fmt"
	api "github.com/DataDog/datadog-operator/test/e2e/tests/updaterSuite/rc-updater/api"

	"github.com/stretchr/testify/assert"
)

type testSuite interface {
	Client() *api.Client
}

func CheckFeaturesState(u testSuite, c assert.TestingT, clusterName string, state bool) {
	query := fmt.Sprintf("SELECT DISTINCT cluster_name, feature_cws_enabled, feature_cspm_enabled,feature_csm_vm_containers_enabled,feature_csm_vm_hosts_enabled,feature_usm_enabled FROM datadog_agent LEFT JOIN host USING(datadog_agent_key) WHERE cluster_name='%s'", clusterName)
	resp, err := u.Client().TableQuery(query)
	if !assert.NoErrorf(c, err, "ddsql query failed") {
		return
	}
	if !assert.Len(c, resp.Data, 1, "ddsql query didn't returned a single row") {
		return
	}
	if !assert.Len(c, resp.Data[0].Attributes.Columns, 6, "ddsql query didn't returned six columns") {
		return
	}
	for _, column := range resp.Data[0].Attributes.Columns[1:] {
		if !assert.True(c, len(column.Values) != 0, "Feature should be set", column.Name) {
			return
		}
		if !assert.Equal(c, column.Values[0].(bool), state, "Feature", column.Name, "should be", state) {
			return
		}
	}
}

func TestConfigsContent(t assert.TestingT, content api.ResponseContent) {
	if !assert.Truef(t, content.SystemProbe.RuntimeSecurityConfig.Enabled, "CWS should be enabled") {
		return
	}
	if !assert.Truef(t, content.SecurityAgent.ComplianceConfig.Enabled, "CSPM should be enabled") {
		return
	}
	if !assert.Truef(t, content.Config.SBOM.Host.Enabled, "Vuln Host should be enabled") {
		return
	}
	if !assert.Truef(t, content.Config.SBOM.Host.Enabled, "Vuln container should be enabled") {
		return
	}
	if !assert.Truef(t, content.SystemProbe.ServiceMonitoringConfig.Enabled, "USM should be enabled") {
		return
	}
}
