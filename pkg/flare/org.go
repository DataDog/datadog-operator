// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-operator/pkg/constants"
)

// Unknown is the value of org information the flare couldn't find.
const Unknown = "unknown"

// OrgValue is one piece of org information in the manifest: its value, or
// "unknown", with how it was obtained or why it's unknown.
type OrgValue struct {
	Value  string `json:"value"`
	Source string `json:"source,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// Known returns a known org value.
func Known(value, source string) OrgValue {
	return OrgValue{Value: value, Source: source}
}

// UnknownOrgValue returns an org value that the flare couldn't find, and why.
func UnknownOrgValue(reason string) OrgValue {
	return OrgValue{Value: Unknown, Reason: reason}
}

// OrgInfo is the Datadog org a flare came from, as far as the Operator knows
// it. It never holds keys. The RemoteConfig fields are the last Remote
// Configuration org status ("true" or "false"), if one was read.
type OrgInfo struct {
	Site                         OrgValue `json:"site"`
	OrgUUID                      OrgValue `json:"org_uuid"`
	OrgID                        OrgValue `json:"org_id"`
	RemoteConfigOrgEnabled       string   `json:"remote_config_org_enabled,omitempty"`
	RemoteConfigAPIKeyAuthorized string   `json:"remote_config_api_key_authorized,omitempty"`
}

// SiteFromSettings returns the site the Operator sends data to, from its
// DD_SITE, DD_DD_URL and DD_URL settings. from describes where the settings
// were read, such as "the Operator environment".
func SiteFromSettings(site, ddURL, from string) OrgValue {
	if site = strings.TrimSpace(site); site != "" {
		return Known(site, fmt.Sprintf("%s from %s", constants.DDSite, from))
	}
	if ddURL = strings.TrimSpace(ddURL); ddURL != "" {
		return Known(ddURL, fmt.Sprintf("%s or %s from %s (%s is unset)", constants.DDddURL, constants.DDURL, from, constants.DDSite))
	}
	return Known(DefaultSite, fmt.Sprintf("default: %s, %s and %s are unset in %s", constants.DDSite, constants.DDddURL, constants.DDURL, from))
}
