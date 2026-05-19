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
)

// AutoscalingAPI is the subset of *autoscaling.Client used by evictASG.
// Defined as an interface so unit tests can stub the AWS SDK out cheaply.
type AutoscalingAPI interface {
	UpdateAutoScalingGroup(ctx context.Context, in *autoscaling.UpdateAutoScalingGroupInput, opts ...func(*autoscaling.Options)) (*autoscaling.UpdateAutoScalingGroupOutput, error)
}

// evictASG cordons every node in the ASG, drains them (Eviction API), and —
// **only when all nodes drained cleanly** — neutralizes the ASG with a single
// UpdateAutoScalingGroup call that sets min=max=desired=0. The ASG itself
// terminates the (already drained) instances to satisfy the new size.
//
// Safety rules:
//
//  1. The function runs in two phases — drain ALL nodes first, then scale
//     the ASG. If any drain fails, the ASG is left at its current size so
//     a re-run can pick up where this one stopped.
//  2. We do NOT use per-instance TerminateInstanceInAutoScalingGroup with
//     `ShouldDecrementDesiredCapacity=true` because (a) AWS rejects it when
//     `MinSize == DesiredCapacity` (decrementing would drop desired below
//     min), and (b) decrementing desired one-by-one can trigger AZ
//     rebalancing which terminates instances in other AZs. A single
//     UpdateAutoScalingGroup(0,0,0) avoids both issues.
//
// We never delete the ASG: it may be managed by Terraform/CloudFormation/Helm,
// and only the original owner should remove it.
func evictASG(ctx context.Context, clientset kubernetes.Interface, asg AutoscalingAPI, asgName string, nodes []string, drainOpts nodeDrainOptions) error {
	var (
		errs        []error
		drainFailed bool
	)

	for _, nodeName := range nodes {
		if _, err := clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{}); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			errs = append(errs, fmt.Errorf("node %s: %w", nodeName, err))
			drainFailed = true
			continue
		}
		if err := cordonNode(ctx, clientset, nodeName, drainOpts.DryRun); err != nil {
			errs = append(errs, fmt.Errorf("cordon node %s: %w", nodeName, err))
			drainFailed = true
			continue
		}
		if err := drainNode(ctx, clientset, nodeName, drainOpts); err != nil {
			errs = append(errs, fmt.Errorf("drain node %s: %w", nodeName, err))
			drainFailed = true
			continue
		}
	}

	if drainFailed {
		log.Printf("ASG %s: at least one node failed to drain; leaving the ASG untouched. Re-run after addressing the errors above.", asgName)
		return errors.Join(errs...)
	}

	// Every node drained cleanly. Scale the ASG to 0 — it terminates the
	// drained instances asynchronously to satisfy the new size, which is
	// safe because they are empty and cordoned.
	if drainOpts.DryRun {
		log.Printf("[dry-run] would scale ASG %s to min=max=desired=0", asgName)
		return nil
	}
	if err := scaleASGToZero(ctx, asg, asgName); err != nil {
		errs = append(errs, fmt.Errorf("scale ASG %s to 0: %w", asgName, err))
	}
	return errors.Join(errs...)
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
