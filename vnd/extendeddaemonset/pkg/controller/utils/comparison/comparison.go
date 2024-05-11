// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// Package comparison contains object comparison functions.
package comparison

import (
	"bytes"

	// #nosec
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
)

// IsReplicaSetUpToDate returns true if the ExtendedDaemonSetReplicaSet is up to date with the ExtendedDaemonSet pod template.
func IsReplicaSetUpToDate(rs *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet, daemonset *datadoghqv1alpha1.ExtendedDaemonSet) bool {
	hash, err := GenerateMD5PodTemplateSpec(&daemonset.Spec.Template)
	if err != nil {
		return false
	}

	return ComparePodTemplateSpecMD5Hash(hash, rs)
}

// ComparePodTemplateSpecMD5Hash used to compare a md5 hash with the one setted in Deployment annotation.
func ComparePodTemplateSpecMD5Hash(hash string, rs *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet) bool {
	if val, ok := rs.Annotations[string(datadoghqv1alpha1.MD5ExtendedDaemonSetAnnotationKey)]; ok && val == hash {
		return true
	}

	return false
}

// GenerateMD5PodTemplateSpec used to generate the DeploymentSpec MD5 hash.
func GenerateMD5PodTemplateSpec(tpl *corev1.PodTemplateSpec) (string, error) {
	b, err := json.Marshal(tpl)
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

// SetMD5PodTemplateSpecAnnotation used to set the md5 annotation key/value from the ExtendedDaemonSetReplicaSet.Spec.Template.
func SetMD5PodTemplateSpecAnnotation(rs *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet, daemonset *datadoghqv1alpha1.ExtendedDaemonSet) (string, error) {
	md5Spec, err := GenerateMD5PodTemplateSpec(&daemonset.Spec.Template)
	if err != nil {
		return "", fmt.Errorf("unable to generates the JobSpec MD5, %w", err)
	}
	if rs.Annotations == nil {
		rs.SetAnnotations(map[string]string{})
	}
	rs.Annotations[string(datadoghqv1alpha1.MD5ExtendedDaemonSetAnnotationKey)] = md5Spec

	return md5Spec, nil
}

// StringsContains contains tells whether a contains x.
func StringsContains(a []string, x string) bool {
	for _, n := range a {
		if x == n {
			return true
		}
	}

	return false
}

// GenerateHashFromEDSResourceNodeAnnotation is used to generate the MD5 hash from EDS Node annotations that allow a user
// to overwrites the containers resources specification for a specific Node.
func GenerateHashFromEDSResourceNodeAnnotation(edsNamespace, edsName string, nodeAnnotations map[string]string) string {
	// build prefix for this specific eds
	prefixKey := fmt.Sprintf(datadoghqv1alpha1.ExtendedDaemonSetRessourceNodeAnnotationKey, edsNamespace, edsName, "")

	resourcesAnnotations := []string{}
	for key, value := range nodeAnnotations {
		if strings.HasPrefix(key, prefixKey) {
			resourcesAnnotations = append(resourcesAnnotations, fmt.Sprintf("%s=%s", key, value))
		}
	}
	if len(resourcesAnnotations) == 0 {
		// no annotation == no hash
		return ""
	}
	sort.Strings(resourcesAnnotations)
	/* #nosec */
	hash := md5.New()
	for _, val := range resourcesAnnotations {
		_, _ = hash.Write([]byte(val))
	}

	return hex.EncodeToString(hash.Sum(nil))
}
