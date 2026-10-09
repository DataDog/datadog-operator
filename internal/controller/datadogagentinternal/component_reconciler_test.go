package datadogagentinternal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

func Test_isComponentEnabled(t *testing.T) {
	required := feature.RequiredComponents{
		ClusterAgent:        feature.RequiredComponent{IsRequired: ptr.To(true)},
		ClusterChecksRunner: feature.RequiredComponent{IsRequired: ptr.To(true)},
		OtelAgentGateway:    feature.RequiredComponent{IsRequired: ptr.To(true)},
	}
	components := []ComponentReconciler{
		NewClusterAgentComponent(nil),
		NewClusterChecksRunnerComponent(nil),
		NewOtelAgentGatewayComponent(nil),
	}

	tests := []struct {
		name         string
		labels       map[string]string
		override     map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride
		wantEnabled  bool
		wantConflict bool
	}{
		{
			name:        "default DDAI deploys required components",
			wantEnabled: true,
		},
		{
			name: "default DDAI respects the user's disabled override",
			override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.ClusterAgentComponentName:        {Disabled: ptr.To(true)},
				v2alpha1.ClusterChecksRunnerComponentName: {Disabled: ptr.To(true)},
				v2alpha1.OtelAgentGatewayComponentName:    {Disabled: ptr.To(true)},
			},
			wantEnabled:  false,
			wantConflict: true,
		},
		{
			name:         "profile DDAI never deploys cluster-wide components, without a disabled override",
			labels:       map[string]string{constants.ProfileLabelKey: "foo-profile"},
			wantEnabled:  false,
			wantConflict: false,
		},
		{
			name:   "profile DDAI never deploys cluster-wide components, with an inherited override",
			labels: map[string]string{constants.ProfileLabelKey: "foo-profile"},
			override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.ClusterAgentComponentName: {Replicas: ptr.To(int32(2))},
			},
			wantEnabled:  false,
			wantConflict: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := &ReconcileComponentParams{
				DDAI: &datadoghqv1alpha1.DatadogAgentInternal{
					ObjectMeta: metav1.ObjectMeta{Name: "foo", Labels: tt.labels},
					Spec:       v2alpha1.DatadogAgentSpec{Override: tt.override},
				},
				RequiredComponents: required,
			}
			for _, comp := range components {
				enabled, conflict := isComponentEnabled(comp, params)
				assert.Equal(t, tt.wantEnabled, enabled, "enabled for %s", comp.Name())
				assert.Equal(t, tt.wantConflict, conflict, "conflict for %s", comp.Name())
			}
		})
	}
}
