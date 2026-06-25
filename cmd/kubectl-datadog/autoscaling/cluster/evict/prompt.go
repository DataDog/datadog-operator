package evict

import (
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clusterinfo"
)

func printPlan(streams genericclioptions.IOStreams, info *clusterinfo.ClusterInfo, targets []Target, scaleCA, ensurePDBs bool) {
	panic("TODO: printPlan — implemented in PR https://github.com/DataDog/datadog-operator/pull/3162")
}

func promptConfirmation(streams genericclioptions.IOStreams) bool {
	panic("TODO: promptConfirmation — implemented in PR https://github.com/DataDog/datadog-operator/pull/3162")
}
