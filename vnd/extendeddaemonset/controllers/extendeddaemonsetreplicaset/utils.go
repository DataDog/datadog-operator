// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package extendeddaemonsetreplicaset

import (
	"context"
	"sync"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/DataDog/extendeddaemonset/controllers/extendeddaemonsetreplicaset/strategy"
	podutils "github.com/DataDog/extendeddaemonset/pkg/controller/utils/pod"
)

func createPods(logger logr.Logger, client client.Client, scheme *runtime.Scheme, podAffinitySupported bool, replicaset *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet, podsToCreate []*strategy.NodeItem) []error {
	var errs []error
	var wg sync.WaitGroup
	errsChan := make(chan error, len(podsToCreate))
	for id := range podsToCreate {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			nodeItem := podsToCreate[id]
			newPod, err := podutils.CreatePodFromDaemonSetReplicaSet(scheme, replicaset, nodeItem.Node, nodeItem.ExtendedDaemonsetSetting, podAffinitySupported)
			if err != nil {
				logger.Error(err, "Generate pod template failed", "name", newPod.GenerateName)
				errsChan <- err
			}
			logger.V(1).Info("Create pod", "name", newPod.GenerateName, "node", podsToCreate[id], "addAffinity", podAffinitySupported)
			err = client.Create(context.TODO(), newPod)
			if err != nil {
				logger.Error(err, "Create pod failed", "name", newPod.GenerateName)
				errsChan <- err
			}
		}(id)
	}
	go func() {
		wg.Wait()
		close(errsChan)
	}()

	for err := range errsChan {
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}

func deletePods(logger logr.Logger, c client.Client, podByNodeName map[*strategy.NodeItem]*corev1.Pod, nodes []*strategy.NodeItem) []error {
	var errs []error
	var wg sync.WaitGroup
	errsChan := make(chan error, len(nodes))
	for _, node := range nodes {
		wg.Add(1)
		go func(n *strategy.NodeItem) {
			defer wg.Done()
			logger.V(1).Info("Delete pod", "name", podByNodeName[n].Name, "node", n.Node.Name)
			err := c.Delete(context.TODO(), podByNodeName[n])
			if err != nil {
				errsChan <- err
			}
		}(node)
	}
	go func() {
		wg.Wait()
		close(errsChan)
	}()

	for err := range errsChan {
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}
