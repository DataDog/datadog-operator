package evict

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clusterinfo"
)

func TestPrintPlan(t *testing.T) {
	caPresent := clusterinfo.Autoscaling{
		ClusterAutoscaler: clusterinfo.ClusterAutoscaler{Present: true, Namespace: "kube-system", Name: "cluster-autoscaler"},
	}
	caAbsent := clusterinfo.Autoscaling{ClusterAutoscaler: clusterinfo.ClusterAutoscaler{Present: false}}
	fullTargets := []Target{
		{Manager: clusterinfo.NodeManagerASG, Entity: "legacy-asg", Nodes: []string{"ip-1", "ip-2"}},
		{Manager: clusterinfo.NodeManagerKarpenter, Entity: "user-np", Nodes: []string{"ip-3"}},
		{Manager: clusterinfo.NodeManagerStandalone, Entity: "", Nodes: []string{"ip-4"}},
	}

	for _, tc := range []struct {
		name string
		// info, when non-nil, becomes the input. Use a sentinel "nil" via
		// nilInfo=true to test the nil-tolerant path.
		info       *clusterinfo.ClusterInfo
		nilInfo    bool
		targets    []Target
		scaleCA    bool
		ensurePDBs bool

		wantContains    []string
		wantNotContains []string
	}{
		{
			name:       "all sections rendered",
			info:       &clusterinfo.ClusterInfo{Autoscaling: caPresent},
			targets:    fullTargets,
			scaleCA:    true,
			ensurePDBs: true,
			wantContains: []string{
				"cluster-autoscaler Deployment kube-system/cluster-autoscaler",
				"Create temporary PodDisruptionBudgets",
				"Remove the temporary PodDisruptionBudgets",
				"Evict asg entities",
				"legacy-asg (2 node(s))",
				"Evict karpenter entities",
				"user-np (1 node(s))",
				"Evict standalone entities",
				// Standalone entry with empty name renders as "(all)".
				"(all) (1 node(s))",
			},
			// No EKS managed node group in this plan ⇒ no deferred-cleanup note.
			wantNotContains: []string{"deferred to a later rerun"},
		},
		{
			// An EKS managed node group target surfaces the deferred-cleanup
			// caveat next to the PDB removal bullet.
			name:       "EKS managed node group adds deferred-cleanup note",
			info:       &clusterinfo.ClusterInfo{Autoscaling: caAbsent},
			targets:    []Target{{Manager: clusterinfo.NodeManagerEKSManagedNodeGroup, Entity: "mng-1", Nodes: []string{"ip-5"}}},
			scaleCA:    false,
			ensurePDBs: true,
			wantContains: []string{
				"Remove the temporary PodDisruptionBudgets",
				"deferred to a later rerun if an EKS managed node group drain times out",
			},
		},
		{
			// scaleCA=false hides the CA bullet even when CA is Present;
			// ensurePDBs=false hides the PDB bullets.
			name:            "scaleCA off and ensurePDBs off hide their bullets",
			info:            &clusterinfo.ClusterInfo{Autoscaling: caPresent},
			targets:         []Target{{Manager: clusterinfo.NodeManagerASG, Entity: "x"}},
			scaleCA:         false,
			ensurePDBs:      false,
			wantNotContains: []string{"Scale cluster-autoscaler", "PodDisruptionBudgets"},
		},
		{
			// CA not Present ⇒ no bullet regardless of scaleCA.
			name:            "CA absent hides the bullet",
			info:            &clusterinfo.ClusterInfo{Autoscaling: caAbsent},
			targets:         nil,
			scaleCA:         true,
			ensurePDBs:      false,
			wantNotContains: []string{"Scale cluster-autoscaler"},
		},
		{
			// Nil info ⇒ no CA bullet but the PDB bullets and warning
			// footer still appear.
			name:            "nil info tolerated",
			nilInfo:         true,
			targets:         nil,
			scaleCA:         true,
			ensurePDBs:      true,
			wantContains:    []string{"Create temporary PodDisruptionBudgets"},
			wantNotContains: []string{"Scale cluster-autoscaler"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			streams, _, outBuf, _ := genericclioptions.NewTestIOStreams()
			info := tc.info
			if tc.nilInfo {
				info = nil
			}
			printPlan(streams, info, tc.targets, tc.scaleCA, tc.ensurePDBs)
			out := outBuf.String()
			for _, s := range tc.wantContains {
				assert.Contains(t, out, s)
			}
			for _, s := range tc.wantNotContains {
				assert.NotContains(t, out, s)
			}
		})
	}
}

func TestPromptConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name              string
		input             string
		wantOK            bool
		wantCancelMessage bool
	}{
		{name: "y", input: "y", wantOK: true},
		{name: "Y", input: "Y", wantOK: true},
		{name: "yes", input: "yes", wantOK: true},
		{name: "YES", input: "YES", wantOK: true},
		{name: "Yes-with-newline", input: "Yes\n", wantOK: true},

		{name: "n", input: "n", wantOK: false, wantCancelMessage: true},
		{name: "N", input: "N", wantOK: false, wantCancelMessage: true},
		{name: "no", input: "no", wantOK: false, wantCancelMessage: true},
		{name: "empty", input: "", wantOK: false, wantCancelMessage: true},
		{name: "garbage", input: "blah", wantOK: false, wantCancelMessage: true},
		{name: "digit", input: "1", wantOK: false, wantCancelMessage: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			streams, stdin, outBuf, _ := genericclioptions.NewTestIOStreams()
			stdin.WriteString(tc.input + "\n")
			assert.Equal(t, tc.wantOK, promptConfirmation(streams))
			if tc.wantCancelMessage {
				out := outBuf.String()
				assert.True(t, strings.Contains(out, "cancelled") || strings.Contains(out, "Cancelled"))
			}
		})
	}
}
