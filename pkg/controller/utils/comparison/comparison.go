// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package comparison

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/crds/datadoghq/common"
)

// IsSameSpecMD5Hash used to compare the DatadogAgent.spec md5 hash with the one set in annotations
func IsSameSpecMD5Hash(hash string, annotations map[string]string) bool {
	return IsSameMD5Hash(hash, annotations, common.MD5AgentDeploymentAnnotationKey)
}

// IsSameMD5Hash used to compare a md5 hash with the one set in annotations
func IsSameMD5Hash(hash string, annotations map[string]string, annotationKey string) bool {
	if val, ok := annotations[annotationKey]; ok && val == hash {
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

// SetMD5DatadogAgentGenerationAnnotation is used to set the md5 annotation key/value from spec
func SetMD5DatadogAgentGenerationAnnotation(obj *metav1.ObjectMeta, spec interface{}) (string, error) {
	return SetMD5GenerationAnnotation(obj, spec, common.MD5AgentDeploymentAnnotationKey)
}

// SetMD5GenerationAnnotation is used to set the md5 annotation key/value from spec
func SetMD5GenerationAnnotation(obj *metav1.ObjectMeta, spec interface{}, annotationKey string) (string, error) {
	hash, err := GenerateMD5ForSpec(spec)
	if err != nil {
		return "", fmt.Errorf("unable to generate the spec MD5, %w", err)
	}

	if obj.Annotations == nil {
		obj.SetAnnotations(map[string]string{})
	}
	obj.Annotations[annotationKey] = hash

	return hash, nil
}
