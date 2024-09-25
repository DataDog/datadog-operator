// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helmcheck

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/equality"
)

func Test_buildHelmCheckConfigMap(t *testing.T) {
	owner := &metav1.ObjectMeta{
		Name:      "test",
		Namespace: "foo",
	}

	type fields struct {
		enable                   bool
		runInClusterChecksRunner bool
		rbacSuffix               string
		serviceAccountName       string
		owner                    metav1.Object
		configMapName            string

		collectEvents bool
		valuesAsTags  map[string]string
	}
	tests := []struct {
		name    string
		fields  fields
		want    *corev1.ConfigMap
		wantErr bool
	}{
		{
			name: "default helm check",
			fields: fields{
				owner:         owner,
				enable:        true,
				configMapName: v2alpha1.DefaultHelmCheckConf,
			},
			want: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      v2alpha1.DefaultHelmCheckConf,
					Namespace: owner.GetNamespace(),
				},
				Data: map[string]string{
					helmCheckConfFileName: `---
cluster_check: false
init_config:
instances:
  - collect_events: false
`,
				},
			},
		},
		{
			name: "no cluster check runners",
			fields: fields{
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: false,
				configMapName:            v2alpha1.DefaultHelmCheckConf,
			},
			want: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      v2alpha1.DefaultHelmCheckConf,
					Namespace: owner.GetNamespace(),
				},
				Data: map[string]string{
					helmCheckConfFileName: `---
cluster_check: false
init_config:
instances:
  - collect_events: false
`,
				},
			},
		},
		{
			name: "collect events",
			fields: fields{
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: true,
				configMapName:            v2alpha1.DefaultHelmCheckConf,
				collectEvents:            true,
			},
			want: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      v2alpha1.DefaultHelmCheckConf,
					Namespace: owner.GetNamespace(),
				},
				Data: map[string]string{
					helmCheckConfFileName: `---
cluster_check: true
init_config:
instances:
  - collect_events: true
`,
				},
			},
		},
		{
			name: "collect events, no cluster check runners",
			fields: fields{
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: false,
				configMapName:            v2alpha1.DefaultHelmCheckConf,
				collectEvents:            true,
			},
			want: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      v2alpha1.DefaultHelmCheckConf,
					Namespace: owner.GetNamespace(),
				},
				Data: map[string]string{
					helmCheckConfFileName: `---
cluster_check: false
init_config:
instances:
  - collect_events: true
`,
				},
			},
		},
		{
			name: "values as tags",
			fields: fields{
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: true,
				configMapName:            v2alpha1.DefaultHelmCheckConf,
				valuesAsTags:             map[string]string{"zip": "zap", "foo": "bar"}, // tags map should get sorted alphabetically by key
			},
			want: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      v2alpha1.DefaultHelmCheckConf,
					Namespace: owner.GetNamespace(),
				},
				Data: map[string]string{
					helmCheckConfFileName: `---
cluster_check: true
init_config:
instances:
  - collect_events: false
    helm_values_as_tags:
      foo: bar
      zip: zap
`,
				},
			},
		},
		{
			name: "values as tags, no cluster check runners",
			fields: fields{
				owner:                    owner,
				enable:                   true,
				runInClusterChecksRunner: false,
				configMapName:            v2alpha1.DefaultHelmCheckConf,
				valuesAsTags:             map[string]string{"foo": "bar", "zip": "zap"},
			},
			want: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      v2alpha1.DefaultHelmCheckConf,
					Namespace: owner.GetNamespace(),
				},
				Data: map[string]string{
					helmCheckConfFileName: `---
cluster_check: false
init_config:
instances:
  - collect_events: false
    helm_values_as_tags:
      foo: bar
      zip: zap
`,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &helmCheckFeature{
				runInClusterChecksRunner: tt.fields.runInClusterChecksRunner,
				rbacSuffix:               tt.fields.rbacSuffix,
				serviceAccountName:       tt.fields.serviceAccountName,
				owner:                    tt.fields.owner,
				configMapName:            tt.fields.configMapName,
				collectEvents:            tt.fields.collectEvents,
				valuesAsTags:             tt.fields.valuesAsTags,
			}
			got, err := f.buildHelmCheckConfigMap()

			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !equality.IsEqualConfigMap(got, tt.want) {
				t.Errorf("got = %#v,\nwant %#v", got, tt.want)
			}
		})
	}
}
