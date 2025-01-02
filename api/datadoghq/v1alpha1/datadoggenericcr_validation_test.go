// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_IsValidDatadogGenericCR(t *testing.T) {
	tests := []struct {
		name    string
		spec    *DatadogGenericCRSpec
		wantErr string
	}{
		{
			name: "supported resource type and non empty json spec",
			spec: &DatadogGenericCRSpec{
				Type: SyntheticsBrowserTest,
				// N.B. This is a valid JSON string but not valid for the API (not a model payload).
				// This is just for testing purposes.
				JsonSpec: "{\"foo\": \"bar\"}",
			},
			wantErr: "",
		},
		{
			name: "unsupported resource type",
			spec: &DatadogGenericCRSpec{
				Type:     "foo",
				JsonSpec: "{\"foo\": \"bar\"}",
			},
			wantErr: "spec.Type must be a supported resource type",
		},
		{
			name: "empty json spec",
			spec: &DatadogGenericCRSpec{
				Type:     Notebook,
				JsonSpec: "",
			},
			wantErr: "spec.JsonSpec must be defined",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := IsValidDatadogGenericCR(test.spec)
			if test.wantErr != "" {
				assert.EqualError(t, err, test.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
