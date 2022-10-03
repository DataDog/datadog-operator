// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"fmt"
	"strings"
)

func getDefaultConfigMapName(ddaName, fileName string) string {
	return fmt.Sprintf("%s-%s-yaml", ddaName, strings.Split(fileName, ".")[0])
}
