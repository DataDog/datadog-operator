package evict

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"k8s.io/client-go/kubernetes"
)

// AutoscalingAPI is the subset of *autoscaling.Client used by evictASG.
// Defined as an interface so unit tests can stub the AWS SDK out cheaply.
type AutoscalingAPI interface {
	UpdateAutoScalingGroup(ctx context.Context, in *autoscaling.UpdateAutoScalingGroupInput, opts ...func(*autoscaling.Options)) (*autoscaling.UpdateAutoScalingGroupOutput, error)
	SuspendProcesses(ctx context.Context, in *autoscaling.SuspendProcessesInput, opts ...func(*autoscaling.Options)) (*autoscaling.SuspendProcessesOutput, error)
	TerminateInstanceInAutoScalingGroup(ctx context.Context, in *autoscaling.TerminateInstanceInAutoScalingGroupInput, opts ...func(*autoscaling.Options)) (*autoscaling.TerminateInstanceInAutoScalingGroupOutput, error)
}

// evictASG cordons every node in the ASG up front, then drains them,
// terminating each node's instance — and decrementing the ASG's desired
// capacity — as soon as that node has drained cleanly, so its capacity is
// freed without waiting for the rest of the group. Cordoning the whole group
// before any drain (as EKS does for a managed node group) keeps a pod evicted
// from one node from landing on a sibling node that is itself about to be
// drained. Once every node has drained, the ASG is locked at min=max=desired=0
// so nothing is ever relaunched.
//
// Safety rules:
//
//  1. An instance is only terminated once its node has drained cleanly. If a
//     drain fails, that instance is left running (its pods are still on it)
//     and the ASG is left at MinSize=0 with AZRebalance suspended but is NOT
//     locked at MaxSize=0, so a re-run can pick up where this one stopped.
//  2. Per-instance termination requires two precautions, applied up front by
//     prepareASGForTermination: (a) MinSize=0, because AWS rejects
//     TerminateInstanceInAutoScalingGroup with ShouldDecrementDesiredCapacity
//     while MinSize == DesiredCapacity; and (b) suspending the AZRebalance
//     process, because decrementing desired capacity one instance at a time
//     can otherwise trigger AZ rebalancing that terminates a not-yet-drained
//     instance in another AZ.
//  3. Crash window: because instances are terminated before the final
//     MaxSize=0 lock, a crash (or lost AWS connectivity) after the last
//     termination but before the lock leaves the ASG at DesiredCapacity=0
//     (so it will not relaunch on its own) but with its original MaxSize.
//     A re-run cannot rediscover it — target discovery is driven by surviving
//     Kubernetes nodes, which are now gone — so the operator must re-lock
//     MaxSize manually if an external scaler might raise the desired capacity.
//     In practice the command also scales cluster-autoscaler to zero, so
//     nothing routinely raises it.
//
// We never delete the ASG: it may be managed by Terraform/CloudFormation/Helm,
// and only the original owner should remove it.
func evictASG(ctx context.Context, clientset kubernetes.Interface, asg AutoscalingAPI, asgName string, nodes []string, drainOpts nodeDrainOptions) error {
	panic("TODO: evictASG — implemented in PR #10")
}
