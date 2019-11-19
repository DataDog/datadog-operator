package v1alpha1

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	defaultCanaryReplica             = 1
	defaultCanaryDuration            = 10
	defaultSlowStartIntervalDuration = 1
	defaultMaxParallelPodCreation    = 250
	defaultReconcileFrequency        = 10 * time.Second
)

// IsDefaultedExtendedDaemonSet used to know if a ExtendedDaemonSet is already defaulted
// returns true if yes, else no
func IsDefaultedExtendedDaemonSet(dd *ExtendedDaemonSet) bool {
	if !IsDefaultedExtendedDaemonSetSpecStrategyRollingUpdate(&dd.Spec.Strategy.RollingUpdate) {
		return false
	}

	if dd.Spec.Strategy.Canary != nil {
		if defauled := IsDefaultedExtendedDaemonSetSpecStrategyCanary(dd.Spec.Strategy.Canary); !defauled {
			return false
		}
	}

	if dd.Spec.Strategy.ReconcileFrequency == nil {
		return false
	}

	if dd.Spec.Template.Name != "" {
		// this field needs to be cleaned up as we can't deploy multiple
		// pods with the same name
		return false
	}

	return true
}

// IsDefaultedExtendedDaemonSetSpecStrategyRollingUpdate used to know if a ExtendedDaemonSetSpecStrategyRollingUpdate is already defaulted
// returns true if yes, else no
func IsDefaultedExtendedDaemonSetSpecStrategyRollingUpdate(rollingupdate *ExtendedDaemonSetSpecStrategyRollingUpdate) bool {
	if rollingupdate.MaxUnavailable == nil {
		return false
	}

	if rollingupdate.MaxParallelPodCreation == nil {
		return false
	}

	if rollingupdate.MaxPodSchedulerFailure == nil {
		return false
	}

	if rollingupdate.SlowStartIntervalDuration == nil {
		return false
	}

	if rollingupdate.SlowStartAdditiveIncrease == nil {
		return false
	}

	return true
}

// IsDefaultedExtendedDaemonSetSpecStrategyCanary used to know if a ExtendedDaemonSetSpecStrategyCanary is already defaulted
// returns true if yes, else no
func IsDefaultedExtendedDaemonSetSpecStrategyCanary(canary *ExtendedDaemonSetSpecStrategyCanary) bool {

	if canary.Replicas == nil {
		return false
	}
	if canary.Duration == nil {
		return false
	}
	return true
}

// DefaultExtendedDaemonSet used to default an ExtendedDaemonSet
// return a list of errors in case of unvalid fields.
func DefaultExtendedDaemonSet(dd *ExtendedDaemonSet) *ExtendedDaemonSet {
	defaultedDD := dd.DeepCopy()
	DefaultExtendedDaemonSetSpec(&defaultedDD.Spec)
	return defaultedDD
}

// DefaultExtendedDaemonSetSpec used to default an ExtendedDaemonSetSpec
func DefaultExtendedDaemonSetSpec(spec *ExtendedDaemonSetSpec) *ExtendedDaemonSetSpec {
	// reset template name
	spec.Template.Name = ""

	DefaultExtendedDaemonSetSpecStrategyRollingUpdate(&spec.Strategy.RollingUpdate)

	if spec.Strategy.Canary != nil {
		DefaultExtendedDaemonSetSpecStrategyCanary(spec.Strategy.Canary)
	}

	if spec.Strategy.ReconcileFrequency == nil {
		spec.Strategy.ReconcileFrequency = &metav1.Duration{Duration: defaultReconcileFrequency}
	}

	return spec
}

// DefaultExtendedDaemonSetSpecStrategyCanary used to default an ExtendedDaemonSetSpecStrategyCanary
func DefaultExtendedDaemonSetSpecStrategyCanary(c *ExtendedDaemonSetSpecStrategyCanary) *ExtendedDaemonSetSpecStrategyCanary {
	if c.Duration == nil {
		c.Duration = &metav1.Duration{
			Duration: defaultCanaryDuration * time.Minute,
		}
	}
	if c.Replicas == nil {
		replicas := intstr.FromInt(defaultCanaryReplica)
		c.Replicas = &replicas
	}
	return c
}

// DefaultExtendedDaemonSetSpecStrategyRollingUpdate used to default an ExtendedDaemonSetSpecStrategyRollingUpdate
func DefaultExtendedDaemonSetSpecStrategyRollingUpdate(rollingupdate *ExtendedDaemonSetSpecStrategyRollingUpdate) *ExtendedDaemonSetSpecStrategyRollingUpdate {
	rollingupdate.MaxUnavailable = intstr.ValueOrDefault(rollingupdate.MaxUnavailable, intstr.FromInt(1))

	if rollingupdate.MaxParallelPodCreation == nil {
		rollingupdate.MaxParallelPodCreation = NewInt32(defaultMaxParallelPodCreation)
	}

	rollingupdate.MaxPodSchedulerFailure = intstr.ValueOrDefault(rollingupdate.MaxPodSchedulerFailure, intstr.FromInt(0))

	if rollingupdate.SlowStartIntervalDuration == nil {
		rollingupdate.SlowStartIntervalDuration = &metav1.Duration{
			Duration: defaultSlowStartIntervalDuration * time.Minute,
		}
	}

	rollingupdate.SlowStartAdditiveIncrease = intstr.ValueOrDefault(rollingupdate.SlowStartAdditiveIncrease, intstr.FromInt(1))
	return rollingupdate
}
