// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package clusterchecksrunner

import (
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
)

const (
	pdbMaxUnavailableInstances = 1

	// initCopyCheckAssetsContainerName is the name of the init container that
	// seeds the conf.d overlay with packaged check assets from the agent image.
	initCopyCheckAssetsContainerName = "init-copy-check-assets"

	// copyCheckAssetsCmd copies the packaged conf.d assets from the agent image
	// into the remove-corechecks overlay, then drops the default check
	// configurations ("*.yaml.default") so default core checks still do not run
	// on the cluster checks runner. This preserves packaged assets that live
	// under "conf.d/<check>.d/" subdirectories (for example SNMP profiles under
	// "snmp.d/default_profiles"), mirroring the Helm chart behaviour and letting
	// cluster checks such as the SNMP check autodetect profiles.
	copyCheckAssetsCmd = `set -euo pipefail
if [ -d /etc/datadog-agent/conf.d ]; then
  cp -a /etc/datadog-agent/conf.d/. ` + common.RmCorechecksConfdInitPath + `/
  find ` + common.RmCorechecksConfdInitPath + ` -type f -name '*.yaml.default' -delete
fi`
)
