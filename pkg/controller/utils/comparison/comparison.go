// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package comparison

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	edsdatadoghqv1alpha1 "github.com/datadog/extendeddaemonset/pkg/apis/datadoghq/v1alpha1"
)

// IsExtendedDaemonSetUpToDate returns true if the ExtendedDaemonSet is up to date with the DatadogAgentDeployment agent config spec.
func IsExtendedDaemonSetUpToDate(eds *edsdatadoghqv1alpha1.ExtendedDaemonSet, agentdeployment *datadoghqv1alpha1.DatadogAgentDeployment) bool {
	hash, err := GenerateMD5ForSpec(agentdeployment.Spec.Agent)
	if err != nil {
		return false
	}

	return CompareSpecMD5Hash(hash, eds.Annotations)
}

// CompareSpecMD5Hash used to compare a md5 hash with the one set in annotations
func CompareSpecMD5Hash(hash string, annotations map[string]string) bool {
	if val, ok := annotations[string(datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey)]; ok && val == hash {
		return true
	}
	return false
}

// GenerateMD5ForSpec used to generate MD5 hashes for the Agent and Cluster Agent specs
func GenerateMD5ForSpec(spec interface{}) (string, error) {
	b, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	/* #nosec */
	hash := md5.New()
	_, err = io.Copy(hash, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// SetMD5AgentSpecAnnotation used to set the md5 annotation key/value from the DatadogAgentDeployment.Spec.Agent
func SetMD5AgentSpecAnnotation(meta *metav1.ObjectMeta, agentdeployment *datadoghqv1alpha1.DatadogAgentDeployment) (string, error) {
	hash, err := GenerateMD5ForSpec(agentdeployment.Spec)
	if err != nil {
		return "", fmt.Errorf("unable to generate the Agent spec MD5, %v", err)
	}

	if meta.Annotations == nil {
		meta.SetAnnotations(map[string]string{})
	}
	meta.Annotations[string(datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey)] = hash
	return hash, nil
}

// SetMD5GenerationAnnotation is used to set the md5 annotation key/value from spec
func SetMD5GenerationAnnotation(obj *metav1.ObjectMeta, spec interface{}) (string, error) {
	hash, err := GenerateMD5ForSpec(spec)
	if err != nil {
		return "", fmt.Errorf("unable to generate the spec MD5, %v", err)
	}

	if obj.Annotations == nil {
		obj.SetAnnotations(map[string]string{})
	}
	obj.Annotations[string(datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey)] = hash
	return hash, nil
}
