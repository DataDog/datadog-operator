package evict

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"k8s.io/client-go/kubernetes"

	commonaws "github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/aws"
)

type EC2API interface {
	TerminateInstances(ctx context.Context, in *ec2.TerminateInstancesInput, opts ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error)
}

func evictStandalone(ctx context.Context, clientset kubernetes.Interface, ec2API EC2API, nodes []string, drainOpts nodeDrainOptions) error {
	cordoned, errs := cordonNodes(ctx, clientset, nodes, drainOpts.DryRun)
	for _, node := range cordoned {
		nodeName := node.Name
		if err := drainNode(ctx, clientset, nodeName, drainOpts); err != nil {
			errs = append(errs, fmt.Errorf("drain node %s: %w", nodeName, err))
			continue // do NOT terminate this instance: workloads are still on it
		}

		id, hasInstanceID := commonaws.ExtractEC2InstanceID(node)
		if !hasInstanceID {
			log.Printf("Warning: node %s has unexpected providerID %q; cannot terminate the underlying instance", nodeName, node.Spec.ProviderID)
			continue
		}
		if drainOpts.DryRun {
			log.Printf("[dry-run] would terminate standalone instance %s", id)
			continue
		}
		if _, err := ec2API.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
			InstanceIds: []string{id},
		}); err != nil {
			errs = append(errs, fmt.Errorf("terminate instance %s: %w", id, err))
		} else {
			log.Printf("Terminated standalone EC2 instance %s.", id)
		}
	}
	return errors.Join(errs...)
}
