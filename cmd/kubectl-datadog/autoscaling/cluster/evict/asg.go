package evict

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"k8s.io/client-go/kubernetes"
)

type AutoscalingAPI interface {
	UpdateAutoScalingGroup(ctx context.Context, in *autoscaling.UpdateAutoScalingGroupInput, opts ...func(*autoscaling.Options)) (*autoscaling.UpdateAutoScalingGroupOutput, error)
	SuspendProcesses(ctx context.Context, in *autoscaling.SuspendProcessesInput, opts ...func(*autoscaling.Options)) (*autoscaling.SuspendProcessesOutput, error)
	TerminateInstanceInAutoScalingGroup(ctx context.Context, in *autoscaling.TerminateInstanceInAutoScalingGroupInput, opts ...func(*autoscaling.Options)) (*autoscaling.TerminateInstanceInAutoScalingGroupOutput, error)
}

func evictASG(ctx context.Context, clientset kubernetes.Interface, asg AutoscalingAPI, asgName string, nodes []string, drainOpts nodeDrainOptions) error {
	panic("TODO: evictASG — implemented in PR https://github.com/DataDog/datadog-operator/pull/3175")
}
