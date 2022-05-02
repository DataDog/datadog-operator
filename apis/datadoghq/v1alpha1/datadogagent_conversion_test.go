// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/google/go-cmp/cmp"

	"github.com/stretchr/testify/assert"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestDatadogAgentConversion(t *testing.T) {
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	_ = AddToScheme(sch) // Local v1alpha1
	_ = v2alpha1.AddToScheme(sch)

	serializer := json.NewSerializerWithOptions(json.DefaultMetaFactory, sch, sch, json.SerializerOptions{
		Yaml: true,
	})

	testCases := []struct {
		desc             string
		inputFilename    string
		expectedFilename string
	}{
		{
			desc:             "Test full conversion",
			inputFilename:    "all.yaml",
			expectedFilename: "all.expected.yaml",
		},
		{
			desc:             "Test Features have priority over spec (logs)",
			inputFilename:    "featureOvr.yaml",
			expectedFilename: "featureOvr.expected.yaml",
		},
		{
			desc:             "Test Empty",
			inputFilename:    "empty.yaml",
			expectedFilename: "empty.expected.yaml",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			agentV1 := &DatadogAgent{}
			err := readKubernetesObject(serializer, tc.inputFilename, agentV1)
			assert.NoError(t, err)

			agentV2 := &v2alpha1.DatadogAgent{}
			assert.NoError(t, convertTo(agentV1, agentV2))

			if tc.expectedFilename != "" {
				expectedV2 := &v2alpha1.DatadogAgent{}
				err = readKubernetesObject(serializer, tc.expectedFilename, expectedV2)
				assert.NoError(t, err)

				assert.Empty(t, cmp.Diff(agentV2, expectedV2))
			} else {
				writeKubernetesObject(serializer, agentV2, "v2."+tc.inputFilename)
			}
		})
	}
}

func readKubernetesObject(decoder runtime.Decoder, filename string, object runtime.Object) error {
	data, err := ioutil.ReadFile(getTestFilePath(filename))
	if err != nil {
		return err
	}

	_, _, err = decoder.Decode(data, nil, object)
	return err
}

func writeKubernetesObject(encoder runtime.Encoder, output runtime.Object, filename string) error {
	f, err := os.OpenFile(getTestFilePath(filename), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder.Encode(output, f)
	return nil
}

func getTestFilePath(filename string) string {
	return filepath.Join("./testdata", filename)
}
