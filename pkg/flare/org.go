// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/pkg/constants"
)

// Unknown is the value of org information the flare couldn't find.
const Unknown = "unknown"

// OrgValue is one piece of org information in the manifest: its value, or
// "unknown", with how it was obtained or why it's unknown.
type OrgValue struct {
	Value string `json:"value"`
	// Source says how the value was obtained.
	Source string `json:"source,omitempty"`
	// Reason says why the value is unknown.
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
// it. It's written to the manifest. It never holds keys: the org is
// identified by its site, the org UUID that Remote Configuration fetches
// from the org data endpoint, and the org ID of Remote Configuration paths.
type OrgInfo struct {
	Site    OrgValue `json:"site"`
	OrgUUID OrgValue `json:"org_uuid"`
	OrgID   OrgValue `json:"org_id"`
	// RemoteConfigOrgEnabled and RemoteConfigAPIKeyAuthorized are the last
	// org status the Remote Configuration service read, as the Agent status
	// shows it ("true" or "false"), if it read one.
	RemoteConfigOrgEnabled       string `json:"remote_config_org_enabled,omitempty"`
	RemoteConfigAPIKeyAuthorized string `json:"remote_config_api_key_authorized,omitempty"`
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

// PodOrgInfo returns the org information the Kubernetes API shows of an
// Operator pod: the site from its container environment. The org UUID and
// org ID are only known to the Operator's Remote Configuration client, so
// they're unknown.
func PodOrgInfo(pod *corev1.Pod) *OrgInfo {
	const notVisible = "only known to the Operator's Remote Configuration client: it's in flares requested from Fleet Automation, which the Operator collects itself, not in flares the kubectl plugin collects through the Kubernetes API"
	info := &OrgInfo{
		OrgUUID: UnknownOrgValue("the org UUID is " + notVisible),
		OrgID:   UnknownOrgValue("the org ID is " + notVisible),
	}
	container := OperatorContainer(pod)
	if container == nil {
		info.Site = UnknownOrgValue("the Operator pod is unknown")
		return info
	}
	from := fmt.Sprintf("the pod spec of %s, container %s", pod.Name, container.Name)
	values := map[string]string{}
	for _, env := range container.Env {
		switch env.Name {
		case constants.DDSite, constants.DDddURL, constants.DDURL:
		default:
			continue
		}
		if env.ValueFrom != nil {
			// The value is in a Secret or ConfigMap, which flares don't
			// read for the Operator container.
			ref := podConfigEnv(env).From
			info.Site = UnknownOrgValue(fmt.Sprintf("%s is set from %s in %s, which the flare doesn't read", env.Name, ref, from))
			return info
		}
		values[env.Name] = env.Value
	}
	if len(container.EnvFrom) > 0 && len(values) == 0 {
		info.Site = UnknownOrgValue(fmt.Sprintf("%s is not set in %s, which takes variables from Secrets or ConfigMaps (envFrom) the flare doesn't read", constants.DDSite, from))
		return info
	}
	ddURL := values[constants.DDddURL]
	if ddURL == "" {
		ddURL = values[constants.DDURL]
	}
	info.Site = SiteFromSettings(values[constants.DDSite], ddURL, from)
	return info
}
