// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSiteFromSettings(t *testing.T) {
	assert.Equal(t, OrgValue{Value: "datadoghq.eu", Source: "DD_SITE from env"}, SiteFromSettings(" datadoghq.eu ", "https://app.datadoghq.com", "env"))
	assert.Equal(t, OrgValue{Value: "https://app.datad0g.com", Source: "DD_DD_URL or DD_URL from env (DD_SITE is unset)"}, SiteFromSettings("", "https://app.datad0g.com", "env"))
	assert.Equal(t, OrgValue{Value: DefaultSite, Source: "default: DD_SITE, DD_DD_URL and DD_URL are unset in env"}, SiteFromSettings("", "", "env"))
}
