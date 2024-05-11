package utils

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestConvertLabelSelector(t *testing.T) {
	logf.SetLogger(zap.New())
	log := logf.Log.WithName("TestConvertLabelSelector")

	foobarReq, _ := labels.NewRequirement("foo", selection.In, []string{"bar"})
	bazquxReq, _ := labels.NewRequirement("baz", selection.In, []string{"qux"})

	tests := []struct {
		name    string
		input   metav1.LabelSelector
		want    labels.Selector
		wantErr bool
	}{
		{
			name: "match labels",
			input: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
					"baz": "qux",
				},
			},
			want:    labels.NewSelector().Add(*foobarReq).Add(*bazquxReq),
			wantErr: false,
		},
		{
			name: "match expressions",
			input: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "foo",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"bar"},
					},
					{
						Key:      "baz",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"qux"},
					},
				},
			},
			want:    labels.NewSelector().Add(*foobarReq).Add(*bazquxReq),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqLogger := log.WithValues("test:", tt.name)
			got, err := ConvertLabelSelector(reqLogger, &tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConvertLabelSelector() error = %v, wantErr %v", err, tt.wantErr)

				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ConvertLabelSelector() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
