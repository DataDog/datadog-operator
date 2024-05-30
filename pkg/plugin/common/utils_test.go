// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	validOpenmetricsInstance = `
	[{
	  "prometheus_url": "http://%%host%%:8383/metrics",
	  "namespace": "datadog.operator",
	  "metrics": ["*"]
	}]`
	invalidOpenmetricsInstance = `
	[{
	  "prometheus_url": "http://%%host%%:8383/metrics",
	  "namespace": "datadog.operator",
	  "metrics": ["*]
	}]`
)

func TestHasImagePattern(t *testing.T) {
	tests := []struct {
		name  string
		image string
		want  bool
	}{
		{
			name:  "nominal case",
			image: "gcr.io/datadoghq/agent:latest",
			want:  true,
		},
		{
			name:  "no tag 1",
			image: "datadog/agent",
			want:  false,
		},
		{
			name:  "no tag 2",
			image: "datadog/agent:",
			want:  false,
		},
		{
			name:  "no repo 1",
			image: "datadog/:latest",
			want:  false,
		},
		{
			name:  "no repo 2",
			image: "datadog:latest",
			want:  false,
		},
		{
			name:  "multiple tags",
			image: "datadog/agent:tag1:tag2",
			want:  true,
		},
		{
			name:  "no account",
			image: "/agent:latest",
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasImagePattern(tt.image); got != tt.want {
				t.Errorf("HasImagePattern() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsAnnotated(t *testing.T) {
	type args struct {
		annotations map[string]string
		prefix      string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "found",
			args: args{
				annotations: map[string]string{
					"foo": "bar",
					"baz": "foo",
				},
				prefix: "ba",
			},
			want: true,
		},
		{
			name: "not found",
			args: args{
				annotations: map[string]string{
					"foo": "bar",
					"baz": "foo",
				},
				prefix: "ab",
			},
			want: false,
		},
		{
			name: "empty prefix",
			args: args{
				annotations: map[string]string{
					"foo": "bar",
					"baz": "foo",
				},
				prefix: "",
			},
			want: false,
		},
		{
			name: "nil input",
			args: args{
				annotations: nil,
				prefix:      "",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAnnotated(tt.args.annotations, tt.args.prefix); got != tt.want {
				t.Errorf("IsAnnotated() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateAnnotationsContent(t *testing.T) {
	type args struct {
		annotations map[string]string
		identifier  string
	}
	tests := []struct {
		name  string
		args  args
		want  []string
		want1 bool
	}{
		{
			name: "valid",
			args: args{
				annotations: map[string]string{
					"ad.datadoghq.com/datadog-operator.check_names":  `["openmetrics"]`,
					"ad.datadoghq.com/datadog-operator.init_configs": "[{}]",
					"ad.datadoghq.com/datadog-operator.instances":    validOpenmetricsInstance,
				},
				identifier: "ad.datadoghq.com/datadog-operator",
			},
			want:  []string{},
			want1: true,
		},
		{
			name: "typos",
			args: args{
				annotations: map[string]string{
					"ad.datadoghq.com/datadog-operator.check_name":  `["openmetrics"]`,
					"ad.datadoghq.com/datadog-operator.int_configs": "[{}]",
					"ad.datadoghq.com/datadog-operator.instances":   validOpenmetricsInstance,
				},
				identifier: "ad.datadoghq.com/datadog-operator",
			},
			want: []string{
				"Annotation ad.datadoghq.com/datadog-operator.check_names is missing",
				"Annotation ad.datadoghq.com/datadog-operator.init_configs is missing",
			},
			want1: true,
		},
		{
			name: "invalid json",
			args: args{
				annotations: map[string]string{
					"ad.datadoghq.com/datadog-operator.check_names":  `["openmetrics"]`,
					"ad.datadoghq.com/datadog-operator.init_configs": "[{}]",
					"ad.datadoghq.com/datadog-operator.instances":    invalidOpenmetricsInstance,
				},
				identifier: "ad.datadoghq.com/datadog-operator",
			},
			want:  []string{fmt.Sprintf("Annotation ad.datadoghq.com/datadog-operator.instances with value %s is not a valid JSON: invalid character '\\n' in string literal", invalidOpenmetricsInstance)},
			want1: true,
		},
		{
			name: "missing init configs",
			args: args{
				annotations: map[string]string{
					"ad.datadoghq.com/datadog-operator.check_names": `["openmetrics"]`,
					"ad.datadoghq.com/datadog-operator.instances":   validOpenmetricsInstance,
				},
				identifier: "ad.datadoghq.com/datadog-operator",
			},
			want:  []string{"Annotation ad.datadoghq.com/datadog-operator.init_configs is missing"},
			want1: true,
		},
		{
			name: "valid metrics / invalid logs json",
			args: args{
				annotations: map[string]string{
					"ad.datadoghq.com/datadog-operator.check_names":  `["openmetrics"]`,
					"ad.datadoghq.com/datadog-operator.init_configs": "[{}]",
					"ad.datadoghq.com/datadog-operator.instances":    validOpenmetricsInstance,
					"ad.datadoghq.com/datadog-operator.logs":         `[{"source":"operator","service":"datadog}]`,
				},
				identifier: "ad.datadoghq.com/datadog-operator",
			},
			want:  []string{`Annotation ad.datadoghq.com/datadog-operator.logs with value [{"source":"operator","service":"datadog}] is not a valid JSON: unexpected end of JSON input`},
			want1: true,
		},
		{
			name: "invalid tags json",
			args: args{
				annotations: map[string]string{
					"ad.datadoghq.com/datadog-operator.tags": `[{service:datadog}]`,
				},
				identifier: "ad.datadoghq.com/datadog-operator",
			},
			want:  []string{`Annotation ad.datadoghq.com/datadog-operator.tags with value [{service:datadog}] is not a valid JSON: invalid character 's' looking for beginning of object key string`},
			want1: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := ValidateAnnotationsContent(tt.args.annotations, tt.args.identifier)
			assert.ElementsMatch(t, tt.want, got)
			assert.Equal(t, tt.want1, got1)
		})
	}
}

func TestValidateAnnotationsMatching(t *testing.T) {
	type args struct {
		annotations map[string]string
		validIDs    map[string]bool
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "match",
			args: args{
				annotations: map[string]string{
					"ad.datadoghq.com/datadog-operator.check_names":  `["openmetrics"]`,
					"ad.datadoghq.com/datadog-operator.init_configs": "[{}]",
					"ad.datadoghq.com/datadog-operator.instances":    validOpenmetricsInstance,
				},
				validIDs: map[string]bool{
					"datadog-operator":  true,
					"another-container": true,
				},
			},
			want: []string{},
		},
		{
			name: "no match",
			args: args{
				annotations: map[string]string{
					"ad.datadoghq.com/datadog-operator.check_names":  `["openmetrics"]`,
					"ad.datadoghq.com/datadog-operator.init_configs": "[{}]",
					"ad.datadoghq.com/datadog-operator.instances":    validOpenmetricsInstance,
				},
				validIDs: map[string]bool{
					"another-container": true,
				},
			},
			want: []string{
				"Annotation ad.datadoghq.com/datadog-operator.check_names is invalid: datadog-operator doesn't match a container name",
				"Annotation ad.datadoghq.com/datadog-operator.init_configs is invalid: datadog-operator doesn't match a container name",
				"Annotation ad.datadoghq.com/datadog-operator.instances is invalid: datadog-operator doesn't match a container name",
			},
		},
		{
			name: "no errors for pod tags",
			args: args{
				annotations: map[string]string{
					"ad.datadoghq.com/tags": `[{"service":"datadog"}]`,
				},
				validIDs: map[string]bool{
					"another-container": true,
				},
			},
			want: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateAnnotationsMatching(tt.args.annotations, tt.args.validIDs)
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}

func TestRegExEscape(t *testing.T) {
	matched, _ := regexp.MatchString(ADPrefixRegex, "adXdatadoghqXcom/")
	assert.False(t, matched)

	matched, _ = regexp.MatchString("ad.datadoghq.com/", "adXdatadoghqXcom/")
	assert.True(t, matched)

}
