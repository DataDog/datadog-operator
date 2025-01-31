// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventcollection

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	kubeAPIServerConfigFileName   = "kubernetes_apiserver.yaml"
	kubeAPIServerConfigFolderName = "kubernetes_apiserver.d"
	eventCollectionRBACPrefix     = "event"

	kubernetesAPIServerCheckConfigVolumeName = "kubernetes-apiserver-check-config"
	// DefaultKubeAPIServerConf default Kubernetes APIServer ConfigMap name
	defaultKubeAPIServerConf string = "kube-apiserver-config"
)

// getRBACResourceName return the RBAC resources name
func getRBACResourceName(owner metav1.Object, suffix string) string {
	return fmt.Sprintf("%s-%s-%s-%s", owner.GetNamespace(), owner.GetName(), eventCollectionRBACPrefix, suffix)
}
