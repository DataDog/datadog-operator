package evict

import (
	"context"
	"errors"
	"fmt"
	"log"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	commonaws "github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/aws"
)

// AutoscalingAPI is the subset of *autoscaling.Client used by evictASG.
// Defined as an interface so unit tests can stub the AWS SDK out cheaply.
type AutoscalingAPI interface {
	UpdateAutoScalingGroup(ctx context.Context, in *autoscaling.UpdateAutoScalingGroupInput, opts ...func(*autoscaling.Options)) (*autoscaling.UpdateAutoScalingGroupOutput, error)
	SuspendProcesses(ctx context.Context, in *autoscaling.SuspendProcessesInput, opts ...func(*autoscaling.Options)) (*autoscaling.SuspendProcessesOutput, error)
	TerminateInstanceInAutoScalingGroup(ctx context.Context, in *autoscaling.TerminateInstanceInAutoScalingGroupInput, opts ...func(*autoscaling.Options)) (*autoscaling.TerminateInstanceInAutoScalingGroupOutput, error)
}

// evictASG cordons and drains every node in the ASG, terminating each node's
// instance — and decrementing the ASG's desired capacity — as soon as that
// node has drained cleanly, so its capacity is freed without waiting for the
// rest of the group. Once every node has drained, the ASG is locked at
// min=max=desired=0 so nothing is ever relaunched.
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
	if drainOpts.DryRun {
		log.Printf("[dry-run] would suspend AZRebalance and set MinSize=0 on ASG %s", asgName)
	} else if err := prepareASGForTermination(ctx, asg, asgName); err != nil {
		return fmt.Errorf("prepare ASG %s for termination: %w", asgName, err)
	}

	var (
		errs        []error
		drainFailed bool
	)

	for _, nodeName := range nodes {
		node, err := clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			errs = append(errs, fmt.Errorf("node %s: %w", nodeName, err))
			drainFailed = true
			continue
		}
		id, hasInstanceID := commonaws.ExtractEC2InstanceID(node)
		if !hasInstanceID {
			log.Printf("Warning: node %s has unexpected providerID %q; its instance will be terminated by the final scale-to-zero instead", nodeName, node.Spec.ProviderID)
		}
		if err := cordonNode(ctx, clientset, nodeName, drainOpts.DryRun); err != nil {
			errs = append(errs, fmt.Errorf("cordon node %s: %w", nodeName, err))
			drainFailed = true
			continue
		}
		if err := drainNode(ctx, clientset, nodeName, drainOpts); err != nil {
			errs = append(errs, fmt.Errorf("drain node %s: %w", nodeName, err))
			drainFailed = true
			continue // do NOT terminate this instance: workloads are still on it
		}
		// The node drained cleanly: terminate its instance now, decrementing
		// the ASG's desired capacity so it is not relaunched. Nodes with an
		// unexpected providerID are left for the final scale-to-zero.
		if !hasInstanceID {
			continue
		}
		if drainOpts.DryRun {
			log.Printf("[dry-run] would terminate instance %s in ASG %s (decrementing desired capacity)", id, asgName)
			continue
		}
		if err := terminateASGInstance(ctx, asg, id); err != nil {
			errs = append(errs, fmt.Errorf("terminate instance %s in ASG %s: %w", id, asgName, err))
			drainFailed = true
		}
	}

	if drainFailed {
		log.Printf("ASG %s: at least one node failed to drain or terminate; leaving the ASG at MinSize=0 without locking MaxSize. Re-run after addressing the errors above.", asgName)
		return errors.Join(errs...)
	}

	// Every node drained and its instance was terminated. Lock the ASG at
	// min=max=desired=0 so nothing is ever relaunched, and to clean up any
	// instance that couldn't be terminated per-node (unexpected providerID).
	if drainOpts.DryRun {
		log.Printf("[dry-run] would scale ASG %s to min=max=desired=0", asgName)
		return nil
	}
	// All instances are now terminated; only the MaxSize=0 lock remains. A
	// crash here would leave the ASG at desired=0 but unlocked and no longer
	// rediscoverable by a re-run (see the crash-window note on evictASG).
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
