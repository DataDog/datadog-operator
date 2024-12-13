// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package upgrade

// TODO add test for v2alpha1

// import (
// 	"context"
// 	"fmt"
// 	"testing"

// 	commonv1 "github.com/DataDog/datadog-operator/api/datadoghq/common/v1"
// 	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
// 	apiutils "github.com/DataDog/datadog-operator/api/utils"
// 	"github.com/DataDog/datadog-operator/pkg/defaulting"

// 	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
// 	"k8s.io/apimachinery/pkg/types"
// 	"k8s.io/client-go/kubernetes/scheme"
// 	"sigs.k8s.io/controller-runtime/pkg/client"
// 	"sigs.k8s.io/controller-runtime/pkg/client/fake"
// )

// func Test_options_upgrade(t *testing.T) {
// 	s := scheme.Scheme
// 	if err := datadoghqv1alpha1.AddToScheme(s); err != nil {
// 		t.Fatalf("Unable to add DatadogAgent scheme: %v", err)
// 	}

// 	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogAgent{})

// 	tests := []struct {
// 		name     string
// 		loadFunc func(c client.Client) *datadoghqv1alpha1.DatadogAgent
// 		image    string
// 		wantErr  bool
// 		wantFunc func(c client.Client, image string) error
// 	}{
// 		{
// 			name: "cluster agent upgrade",
// 			loadFunc: func(c client.Client) *datadoghqv1alpha1.DatadogAgent {
// 				dd := buildDatadogAgent("datadog/cluster-agent:1.5.2")
// 				_ = c.Create(context.TODO(), dd)
// 				return dd
// 			},
// 			image:   defaulting.GetLatestClusterAgentImage(),
// 			wantErr: false,
// 			wantFunc: func(c client.Client, image string) error {
// 				dd := &datadoghqv1alpha1.DatadogAgent{}
// 				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: "datadog-agent", Name: "dd"}, dd); err != nil {
// 					return err
// 				}
// 				if dd.Spec.ClusterAgent.Image.Name != image {
// 					return fmt.Errorf("current image: %s, wanted: %s", dd.Spec.ClusterAgent.Image.Name, image)
// 				}
// 				return nil
// 			},
// 		},
// 		{
// 			name: "same image, no upgrade",
// 			loadFunc: func(c client.Client) *datadoghqv1alpha1.DatadogAgent {
// 				dd := buildDatadogAgent("datadog/cluster-agent:1.5.2")
// 				_ = c.Create(context.TODO(), dd)
// 				return dd
// 			},
// 			image:   "datadog/cluster-agent:1.5.2",
// 			wantErr: true,
// 			wantFunc: func(c client.Client, image string) error {
// 				dd := &datadoghqv1alpha1.DatadogAgent{}
// 				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: "datadog-agent", Name: "dd"}, dd); err != nil {
// 					return err
// 				}
// 				if dd.Spec.ClusterAgent.Image.Name != image {
// 					return fmt.Errorf("current image: %s, wanted: %s", dd.Spec.ClusterAgent.Image.Name, image)
// 				}
// 				return nil
// 			},
// 		},
// 		{
// 			name: "same tag, different repo",
// 			loadFunc: func(c client.Client) *datadoghqv1alpha1.DatadogAgent {
// 				dd := buildDatadogAgent("datadog/cluster-agent:1.5.2")
// 				_ = c.Create(context.TODO(), dd)
// 				return dd
// 			},
// 			image:   "datadog/dca-custom:1.5.2",
// 			wantErr: false,
// 			wantFunc: func(c client.Client, image string) error {
// 				dd := &datadoghqv1alpha1.DatadogAgent{}
// 				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: "datadog-agent", Name: "dd"}, dd); err != nil {
// 					return err
// 				}
// 				if dd.Spec.ClusterAgent.Image.Name != image {
// 					return fmt.Errorf("current image: %s, wanted: %s", dd.Spec.ClusterAgent.Image.Name, image)
// 				}
// 				return nil
// 			},
// 		},
// 		{
// 			name: "no image in the spec",
// 			loadFunc: func(c client.Client) *datadoghqv1alpha1.DatadogAgent {
// 				dd := buildDatadogAgent("")
// 				dd.Spec.ClusterAgent.Enabled = apiutils.NewBoolPointer(true)
// 				dd.Spec.ClusterAgent.Image = nil
// 				_ = c.Create(context.TODO(), dd)
// 				return dd
// 			},
// 			image:   "datadog/dca-custom:1.5.2",
// 			wantErr: false,
// 			wantFunc: func(c client.Client, image string) error {
// 				dd := &datadoghqv1alpha1.DatadogAgent{}
// 				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: "datadog-agent", Name: "dd"}, dd); err != nil {
// 					return err
// 				}
// 				if dd.Spec.ClusterAgent.Image == nil {
// 					return fmt.Errorf("image has not been upgraded")
// 				}
// 				if dd.Spec.ClusterAgent.Image.Name != image {
// 					return fmt.Errorf("current image: %s, wanted: %s", dd.Spec.ClusterAgent.Image.Name, image)
// 				}
// 				return nil
// 			},
// 		},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			o := &options{}
// 			o.Client = fake.NewClientBuilder().WithScheme(s).Build()
// 			if err := o.upgradeV1(tt.loadFunc(o.Client), tt.image); (err != nil) != tt.wantErr {
// 				t.Errorf("options.upgrade() error = %v, wantErr %v", err, tt.wantErr)
// 			}
// 			if err := tt.wantFunc(o.Client, tt.image); err != nil {
// 				t.Errorf("wantFunc returned an error: %v", err)
// 			}
// 		})
// 	}
// }

// func buildDatadogAgent(image string) *datadoghqv1alpha1.DatadogAgent {
// 	return &datadoghqv1alpha1.DatadogAgent{
// 		TypeMeta: metav1.TypeMeta{
// 			Kind:       "DatadogAgent",
// 			APIVersion: fmt.Sprintf("%s/%s", datadoghqv1alpha1.GroupVersion.Group, datadoghqv1alpha1.GroupVersion.Version),
// 		},
// 		ObjectMeta: metav1.ObjectMeta{
// 			Namespace: "datadog-agent",
// 			Name:      "dd",
// 		},
// 		Spec: datadoghqv1alpha1.DatadogAgentSpec{
// 			ClusterAgent: datadoghqv1alpha1.DatadogAgentSpecClusterAgentSpec{
// 				Image: &commonv1.AgentImageConfig{
// 					Name: image,
// 				},
// 			},
// 		},
// 	}
// }
