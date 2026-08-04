package evict

import (
	"context"
	"errors"
	"fmt"
	"log"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"k8s.io/client-go/kubernetes"

	commonaws "github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/aws"
)

type AutoscalingAPI interface {
	UpdateAutoScalingGroup(ctx context.Context, in *autoscaling.UpdateAutoScalingGroupInput, opts ...func(*autoscaling.Options)) (*autoscaling.UpdateAutoScalingGroupOutput, error)
	SuspendProcesses(ctx context.Context, in *autoscaling.SuspendProcessesInput, opts ...func(*autoscaling.Options)) (*autoscaling.SuspendProcessesOutput, error)
	TerminateInstanceInAutoScalingGroup(ctx context.Context, in *autoscaling.TerminateInstanceInAutoScalingGroupInput, opts ...func(*autoscaling.Options)) (*autoscaling.TerminateInstanceInAutoScalingGroupOutput, error)
}

func evictASG(ctx context.Context, clientset kubernetes.Interface, asg AutoscalingAPI, asgName string, nodes []string, drainOpts nodeDrainOptions) error {
	if drainOpts.DryRun {
		log.Printf("[dry-run] would suspend AZRebalance and set MinSize=0 on ASG %s", asgName)
	} else if err := prepareASGForTermination(ctx, asg, asgName); err != nil {
		return fmt.Errorf("prepare ASG %s for termination: %w", asgName, err)
	}

	cordoned, errs := cordonNodes(ctx, clientset, nodes, drainOpts.DryRun)
	for _, node := range cordoned {
		nodeName := node.Name
		id, hasInstanceID := commonaws.ExtractEC2InstanceID(node)
		if !hasInstanceID {
			log.Printf("Warning: node %s has unexpected providerID %q; its instance will be terminated by the final scale-to-zero instead", nodeName, node.Spec.ProviderID)
		}
		if err := drainNode(ctx, clientset, nodeName, drainOpts); err != nil {
			errs = append(errs, fmt.Errorf("drain node %s: %w", nodeName, err))
			continue // do NOT terminate this instance: workloads are still on it
		}
		if !hasInstanceID {
			continue
		}
		if drainOpts.DryRun {
			log.Printf("[dry-run] would terminate instance %s in ASG %s (decrementing desired capacity)", id, asgName)
			continue
		}
		if err := terminateASGInstance(ctx, asg, id); err != nil {
			errs = append(errs, fmt.Errorf("terminate instance %s in ASG %s: %w", id, asgName, err))
		}
	}

	if len(errs) > 0 {
		log.Printf("ASG %s: at least one node failed to cordon, drain, or terminate; leaving the ASG at MinSize=0 without locking MaxSize. Re-run after addressing the errors above.", asgName)
		return errors.Join(errs...)
	}

	if drainOpts.DryRun {
		log.Printf("[dry-run] would scale ASG %s to min=max=desired=0", asgName)
		return nil
	}

	log.Printf("ASG %s: all nodes drained; locking the ASG at min=max=desired=0.", asgName)
	if err := scaleASGToZero(ctx, asg, asgName); err != nil {
		errs = append(errs, fmt.Errorf("scale ASG %s to 0: %w", asgName, err))
	}
	return errors.Join(errs...)
}

// prepareASGForTermination makes the ASG safe for the per-instance termination
// performed during the drain loop:
//
//   - AZRebalance is suspended so that decrementing desired capacity one
//     instance at a time cannot trigger AZ rebalancing — which would terminate
//     a not-yet-drained instance in another availability zone.
//   - MinSize is set to 0 so that TerminateInstanceInAutoScalingGroup may
//     decrement DesiredCapacity (AWS rejects the decrement while
//     MinSize == DesiredCapacity).
func prepareASGForTermination(ctx context.Context, asg AutoscalingAPI, asgName string) error {
	if _, err := asg.SuspendProcesses(ctx, &autoscaling.SuspendProcessesInput{
		AutoScalingGroupName: awssdk.String(asgName),
		ScalingProcesses:     []string{"AZRebalance"},
	}); err != nil {
		return fmt.Errorf("suspend AZRebalance: %w", err)
	}
	if _, err := asg.UpdateAutoScalingGroup(ctx, &autoscaling.UpdateAutoScalingGroupInput{
		AutoScalingGroupName: awssdk.String(asgName),
		MinSize:              awssdk.Int32(0),
	}); err != nil {
		return fmt.Errorf("set MinSize=0: %w", err)
	}
	log.Printf("Prepared ASG %s for termination (AZRebalance suspended, MinSize=0).", asgName)
	return nil
}

// terminateASGInstance terminates a single (already drained) instance and
// decrements the ASG's desired capacity so it is not relaunched.
func terminateASGInstance(ctx context.Context, asg AutoscalingAPI, instanceID string) error {
	if _, err := asg.TerminateInstanceInAutoScalingGroup(ctx, &autoscaling.TerminateInstanceInAutoScalingGroupInput{
		InstanceId:                     awssdk.String(instanceID),
		ShouldDecrementDesiredCapacity: awssdk.Bool(true),
	}); err != nil {
		return err
	}
	log.Printf("Terminated instance %s and decremented ASG desired capacity.", instanceID)
	return nil
}

func scaleASGToZero(ctx context.Context, asg AutoscalingAPI, asgName string) error {
	if _, err := asg.UpdateAutoScalingGroup(ctx, &autoscaling.UpdateAutoScalingGroupInput{
		AutoScalingGroupName: awssdk.String(asgName),
		MinSize:              awssdk.Int32(0),
		MaxSize:              awssdk.Int32(0),
		DesiredCapacity:      awssdk.Int32(0),
	}); err != nil {
		return err
	}
	log.Printf("Scaled ASG %s to min=max=desired=0.", asgName)
	return nil
}
