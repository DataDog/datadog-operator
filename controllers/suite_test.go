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

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	"sigs.k8s.io/controller-runtime/pkg/internal/testing/controlplane"
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

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
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

	testEnv.ControlPlane.Etcd = &controlplane.Etcd{}
	// default the args of the ETCD process
	_ = testEnv.ControlPlane.Etcd.Configure()
	// Add extra space to accept the CRD
	testEnv.ControlPlane.Etcd.Args = append(testEnv.ControlPlane.Etcd.Args, "--max-request-bytes=6291456")

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

	// Start controllers
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).ToNot(HaveOccurred())

	options := SetupOptions{
		SupportExtendedDaemonset: false,
		Creds:                    config.Creds{APIKey: "dummy_api_key", AppKey: "dummy_app_key"},
		DatadogMonitorEnabled:    true,
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
