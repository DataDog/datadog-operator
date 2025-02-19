// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package upgrade

import (
	"context"
	"fmt"
	"testing"

	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/pkg/plugin/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_options_upgradeV2(t *testing.T) {
	s := scheme.Scheme
	if err := datadoghqv2alpha1.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add DatadogAgent scheme: %v", err)
	}

	s.AddKnownTypes(datadoghqv2alpha1.GroupVersion, &datadoghqv2alpha1.DatadogAgent{})

	type fields struct {
		IOStreams            genericclioptions.IOStreams
		Options              common.Options
		args                 []string
		userDatadogAgentName string
	}

	tests := []struct {
		name     string
		fields   fields
		loadFunc func(c client.Client) *datadoghqv2alpha1.DatadogAgent
		image    string
		wantErr  bool
		wantFunc func(c client.Client, image string) error
	}{
		{
			name: "agent upgrade",
			loadFunc: func(c client.Client) *datadoghqv2alpha1.DatadogAgent {
				dd := buildV2DatadogAgent("")
				_ = c.Create(context.TODO(), dd)
				return dd
			},
			image:   "datadog/agent:7.17.1",
			wantErr: false,
			wantFunc: func(c client.Client, image string) error {
				dd := &datadoghqv2alpha1.DatadogAgent{}
				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: "datadog-agent", Name: "dd"}, dd); err != nil {
					return err
				}
				if dd.Spec.Override == nil || dd.Spec.Override["nodeAgent"] == nil {
					return fmt.Errorf("nodeAgent override is not present, spec: %#v", dd.Spec)
				}
				name, tag := common.SplitImageString(image)
				if dd.Spec.Override["nodeAgent"].Image.Name != name || dd.Spec.Override["nodeAgent"].Image.Tag != tag {
					return fmt.Errorf("current image: %s:%s, wanted: %s", dd.Spec.Override["nodeAgent"].Image.Name, dd.Spec.Override["nodeAgent"].Image.Tag, image)
				}
				return nil
			},
		},
		{
			name: "agent is disabled",
			loadFunc: func(c client.Client) *datadoghqv2alpha1.DatadogAgent {
				dd := buildV2DatadogAgent("")
				dd.Spec.Override = map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
					"nodeAgent": {
						Disabled: apiutils.NewPointer(true),
					},
				}
				_ = c.Create(context.TODO(), dd)
				return dd
			},
			image:   "datadog/agent:7.17.1",
			wantErr: false,
			wantFunc: func(c client.Client, image string) error {
				dd := &datadoghqv2alpha1.DatadogAgent{}
				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: "datadog-agent", Name: "dd"}, dd); err != nil {
					return err
				}
				if dd.Spec.Override == nil || dd.Spec.Override["nodeAgent"] == nil {
					return fmt.Errorf("nodeAgent override is not present, spec: %#v", dd.Spec)
				}
				if dd.Spec.Override["nodeAgent"].Image != nil {
					return fmt.Errorf("image override should be nil, current: %#v", dd.Spec.Override["nodeAgent"].Image)
				}

				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &options{
				IOStreams:            tt.fields.IOStreams,
				Options:              tt.fields.Options,
				args:                 tt.fields.args,
				userDatadogAgentName: tt.fields.userDatadogAgentName,
			}
			o.Client = fake.NewClientBuilder().WithScheme(s).Build()
			if err := o.upgradeV2(tt.loadFunc(o.Client), tt.image); (err != nil) != tt.wantErr {
				t.Errorf("options.upgrade() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err := tt.wantFunc(o.Client, tt.image); err != nil {
				t.Errorf("wantFunc returned an error: %v", err)
			}
		})
	}
}

func buildV2DatadogAgent(image string) *datadoghqv2alpha1.DatadogAgent {
	return &datadoghqv2alpha1.DatadogAgent{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogAgent",
			APIVersion: fmt.Sprintf("%s/%s", datadoghqv2alpha1.GroupVersion.Group, datadoghqv2alpha1.GroupVersion.Version),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "datadog-agent",
			Name:      "dd",
		},
		Spec: datadoghqv2alpha1.DatadogAgentSpec{},
	}
}
