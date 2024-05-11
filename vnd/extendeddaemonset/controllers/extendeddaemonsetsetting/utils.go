package extendeddaemonsetsetting

import (
	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
)

type edsNodeByCreationTimestampAndPhase []*datadoghqv1alpha1.ExtendedDaemonsetSetting

func (o edsNodeByCreationTimestampAndPhase) Len() int      { return len(o) }
func (o edsNodeByCreationTimestampAndPhase) Swap(i, j int) { o[i], o[j] = o[j], o[i] }

func (o edsNodeByCreationTimestampAndPhase) Less(i, j int) bool {
	if o[i].CreationTimestamp.Equal(&o[j].CreationTimestamp) {
		return o[i].Name > o[j].Name
	}

	return o[j].CreationTimestamp.Before(&o[i].CreationTimestamp)
}
