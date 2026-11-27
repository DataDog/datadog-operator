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
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/pkg/constants"
)

const maxPooledJSONBufCap = 64 * 1024

// jsonBufPool reuses encoding buffers across GenerateMD5ForSpec calls, which run
// for every rendered spec on every reconcile.
var jsonBufPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, 2048))
	},
}

func putJSONBuf(buf *bytes.Buffer) {
	// Don't keep unusually large buffers alive in the pool.
	if buf.Cap() <= maxPooledJSONBufCap {
		jsonBufPool.Put(buf)
	}
}

// IsSameSpecMD5Hash used to compare the DatadogAgent.spec md5 hash with the one set in annotations
func IsSameSpecMD5Hash(hash string, annotations map[string]string) bool {
	return IsSameMD5Hash(hash, annotations, constants.MD5AgentDeploymentAnnotationKey)
}

// IsSameMD5Hash used to compare a md5 hash with the one set in annotations
func IsSameMD5Hash(hash string, annotations map[string]string, annotationKey string) bool {
	if val, ok := annotations[annotationKey]; ok && val == hash {
		return true
	}

	return false
}

// GenerateMD5ForSpec used to generate MD5 hashes for the Agent and Cluster Agent specs
func GenerateMD5ForSpec(spec any) (string, error) {
	buf := jsonBufPool.Get().(*bytes.Buffer)
	buf.Reset()

	if err := json.NewEncoder(buf).Encode(spec); err != nil {
		putJSONBuf(buf)
		return "", err
	}
	// json.Encoder.Encode appends a trailing newline that json.Marshal doesn't.
	// Strip it so hashes stay identical to the previous json.Marshal-based ones.
	data := bytes.TrimSuffix(buf.Bytes(), []byte{'\n'})

	/* #nosec */
	sum := md5.Sum(data)
	putJSONBuf(buf)
	return hex.EncodeToString(sum[:]), nil
}

// SetMD5DatadogAgentGenerationAnnotation is used to set the md5 annotation key/value from spec
func SetMD5DatadogAgentGenerationAnnotation(obj *metav1.ObjectMeta, spec any) (string, error) {
	return SetMD5GenerationAnnotation(obj, spec, constants.MD5AgentDeploymentAnnotationKey)
}

// SetMD5GenerationAnnotation is used to set the md5 annotation key/value from spec
func SetMD5GenerationAnnotation(obj *metav1.ObjectMeta, spec any, annotationKey string) (string, error) {
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
