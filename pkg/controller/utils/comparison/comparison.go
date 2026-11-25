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
	"slices"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/pkg/constants"
)

const maxPooledJSONBufCap = 64 * 1024

var jsonBufPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, 2048))
	},
}

// isPlainJSONASCII reports whether s contains only printable ASCII characters
// that encoding/json writes verbatim without backslash or HTML escaping.
func isPlainJSONASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c >= 0x7f || c == '"' || c == '\\' || c == '<' || c == '>' || c == '&' {
			return false
		}
	}
	return true
}

// writeStringMapJSON writes m in exact encoding/json format into buf if all keys
// and values are plain ASCII. Returns false if any string requires escaping so
// the caller can fall back to json.NewEncoder.
func writeStringMapJSON(buf *bytes.Buffer, m map[string]string) bool {
	if m == nil {
		buf.WriteString("null")
		return true
	}
	if len(m) == 0 {
		buf.WriteString("{}")
		return true
	}
	var stackKeys [16]string
	var keys []string
	if len(m) <= len(stackKeys) {
		keys = stackKeys[:0]
	} else {
		keys = make([]string, 0, len(m))
	}
	for k, v := range m {
		if !isPlainJSONASCII(k) || !isPlainJSONASCII(v) {
			return false
		}
		keys = append(keys, k)
	}
	slices.Sort(keys)
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteByte('"')
		buf.WriteString(k)
		buf.WriteString(`":"`)
		buf.WriteString(m[k])
		buf.WriteByte('"')
	}
	buf.WriteByte('}')
	return true
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

	encoded := false
	if m, ok := spec.(map[string]string); ok {
		if writeStringMapJSON(buf, m) {
			encoded = true
		} else {
			buf.Reset()
		}
	}

	if !encoded {
		enc := json.NewEncoder(buf)
		if err := enc.Encode(spec); err != nil {
			if buf.Cap() <= maxPooledJSONBufCap {
				jsonBufPool.Put(buf)
			}
			return "", err
		}
		// json.Encoder.Encode appends a single trailing newline compared to json.Marshal.
		if data := buf.Bytes(); len(data) > 0 && data[len(data)-1] == '\n' {
			buf.Truncate(len(data) - 1)
		}
	}

	/* #nosec */
	sum := md5.Sum(buf.Bytes())
	if buf.Cap() <= maxPooledJSONBufCap {
		jsonBufPool.Put(buf)
	}

	var hexBuf [md5.Size * 2]byte
	hex.Encode(hexBuf[:], sum[:])
	return string(hexBuf[:]), nil
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
