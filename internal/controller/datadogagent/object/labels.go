// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package object

import (
	"fmt"
	"maps"
	"strings"

	"github.com/go-logr/logr"
	"github.com/gobwas/glob"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// GetDefaultLabels return default labels attached to a DatadogAgent resource.
func GetDefaultLabels(dda metav1.Object, instanceName, version string) map[string]string {
	labels := make(map[string]string)
	labels[kubernetes.AppKubernetesNameLabelKey] = "datadog-agent-deployment"
	labels[kubernetes.AppKubernetesInstanceLabelKey] = instanceName
	labels[kubernetes.AppKubernetesPartOfLabelKey] = NewPartOfLabelValue(dda).String()
	labels[kubernetes.AppKubernetesVersionLabelKey] = version
	labels[kubernetes.AppKubernetesManageByLabelKey] = "datadog-operator"

	// Copy Datadog labels from DDA Labels
	for k, v := range dda.GetLabels() {
		if strings.HasPrefix(k, DatadogTagPrefix) {
			labels[k] = v
		}
	}

	return labels
}

// GetDefaultAnnotations return default annotations attached to a DatadogAgent resource.
func GetDefaultAnnotations(dda metav1.Object) map[string]string {
	// Currently we don't have any annotation to set by default
	return map[string]string{}
}

// MergeAnnotationsLabels used to merge Annotations and Labels
func MergeAnnotationsLabels(logger logr.Logger, previousVal map[string]string, newVal map[string]string, filter string) map[string]string {
	var globFilter glob.Glob
	var err error
	if filter != "" {
		globFilter, err = glob.Compile(filter)
		if err != nil {
			logger.Error(err, "Unable to parse glob filter for metadata/annotations - discarding everything", "filter", filter)
		}
	}

	mergedMap := make(map[string]string, len(newVal))
	maps.Copy(mergedMap, newVal)

	// Copy from previous if not in new match and matches globfilter
	for k, v := range previousVal {
		if _, found := newVal[k]; !found {
			if (globFilter != nil && globFilter.Match(k)) || strings.Contains(k, "datadoghq.com") {
				mergedMap[k] = v
			}
		}
	}

	return mergedMap
}

func GetChecksumAnnotationKey(keyName string) string {
	if keyName == "" {
		return ""
	}

	return fmt.Sprintf(constants.MD5ChecksumAnnotationKey, keyName)
}
