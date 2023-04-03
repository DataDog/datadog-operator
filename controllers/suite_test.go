// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build integration
// +build integration

package controllers

import (
	"context"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"

	gc "github.com/onsi/ginkgo/config"
	"github.com/onsi/ginkgo/reporters"
	st "github.com/onsi/ginkgo/reporters/stenographer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/controllers/testutils"
	"github.com/DataDog/datadog-operator/pkg/config"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	mgrCancel context.CancelFunc
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	stenographer := st.NewFakeStenographer()
	reporterConfig := gc.DefaultReporterConfigType{}
	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{reporters.NewDefaultReporter(reporterConfig, stenographer)})
}

var _ = BeforeSuite(func() {
	logger := zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true))
	logf.SetLogger(logger)
	var err error

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases", "v1")},
		ErrorIfCRDPathMissing: true,
	}
	Expect(err).ToNot(HaveOccurred())

	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = datadoghqv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = apiregistrationv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	// Create some Nodes
	node1 := testutils.NewNode("node1", nil)
	Expect(k8sClient.Create(context.Background(), node1)).Should(Succeed())
	node2 := testutils.NewNode("node2", nil)
	Expect(k8sClient.Create(context.Background(), node2)).Should(Succeed())

	err = patchCRDsForV1()
	Expect(err).ToNot(HaveOccurred())

	// Start controllers
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).ToNot(HaveOccurred())

	options := SetupOptions{
		SupportExtendedDaemonset: ExtendedDaemonsetOptions{
			Enabled: false,
		},
		Creds:                 config.Creds{APIKey: "dummy_api_key", AppKey: "dummy_app_key"},
		DatadogAgentEnabled:   true,
		DatadogMonitorEnabled: true,
	}

	err = SetupControllers(logger, mgr, options)
	Expect(err).ToNot(HaveOccurred())

	var mgrCtx context.Context
	mgrCtx, mgrCancel = context.WithCancel(ctrl.SetupSignalHandler())

	go func() {
		err = mgr.Start(mgrCtx)
		Expect(err).ToNot(HaveOccurred())
	}()
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	if mgrCancel != nil {
		mgrCancel()
	}
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

// Applies the patch defined in config/test-v1/storagev1_in_datadogagents.yaml
func patchCRDsForV1() error {
	crdKey := client.ObjectKey{
		Namespace: "default",
		Name:      "datadogagents.datadoghq.com",
	}

	crd := v1.CustomResourceDefinition{}
	if err := k8sClient.Get(context.TODO(), crdKey, &crd); err != nil {
		return err
	}

	// Versions[0] is v1alpha1 and [1] is v2alpha1
	crd.Spec.Versions[0].Storage = true
	crd.Spec.Versions[0].Served = true
	crd.Spec.Versions[1].Storage = false
	crd.Spec.Versions[1].Served = false

	if err := k8sClient.Update(context.TODO(), &crd); err != nil {
		return err
	}

	// Wait until the CRD is accessible. Otherwise, we might get errors when
	// trying to create objects of this type.
	Eventually(func() bool {
		err := k8sClient.Get(context.Background(), crdKey, &crd)
		return err == nil
	}, timeout, interval).Should(BeTrue())

	return nil
}
