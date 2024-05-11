package utils

import (
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

// ConvertLabelSelector converts a "k8s.io/apimachinery/pkg/apis/meta/v1".LabelSelector as found in manifests spec section into a "k8s.io/apimachinery/pkg/labels".Selector to be used to filter list operations.
func ConvertLabelSelector(logger logr.Logger, inSelector *metav1.LabelSelector) (labels.Selector, error) {
	outSelector := labels.NewSelector()
	if inSelector != nil {
		for key, value := range inSelector.MatchLabels {
			req, err := labels.NewRequirement(key, selection.In, []string{value})
			if err != nil {
				logger.Error(err, "NewRequirement")

				return outSelector, err
			}
			outSelector = outSelector.Add(*req)
		}

		for _, expr := range inSelector.MatchExpressions {
			var op selection.Operator
			switch expr.Operator {
			case metav1.LabelSelectorOpIn:
				op = selection.In
			case metav1.LabelSelectorOpNotIn:
				op = selection.NotIn
			case metav1.LabelSelectorOpExists:
				op = selection.Exists
			case metav1.LabelSelectorOpDoesNotExist:
				op = selection.DoesNotExist
			default:
				logger.Info("Invalid Operator:", expr.Operator)

				continue
			}
			req, err := labels.NewRequirement(expr.Key, op, expr.Values)
			if err != nil {
				logger.Error(err, "NewRequirement")

				return outSelector, err
			}
			outSelector = outSelector.Add(*req)
		}
	}

	return outSelector, nil
}
