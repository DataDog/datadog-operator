package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/fleet"
)

func main() {
	var (
		action       string
		experimentID string
		namespace    string
		name         string
		patch        string
	)

	flag.StringVar(&action, "action", "", "start, stop, or promote")
	flag.StringVar(&experimentID, "id", "", "experiment ID")
	flag.StringVar(&namespace, "namespace", "datadog", "DDA namespace")
	flag.StringVar(&name, "name", "datadog", "DDA name")
	flag.StringVar(&patch, "patch", "", "JSON merge patch for start")
	flag.Parse()

	if action == "" {
		fmt.Fprintln(os.Stderr, "Usage: fleet-test -action start|stop|promote -id <experiment-id> [-patch <json>]")
		os.Exit(1)
	}

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	logger := ctrl.Log.WithName("fleet-test")

	// Build K8s client
	scheme := runtime.NewScheme()
	_ = v2alpha1.AddToScheme(scheme)

	cfg, err := ctrl.GetConfig()
	if err != nil {
		logger.Error(err, "Failed to get kubeconfig")
		os.Exit(1)
	}

	kubeClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		logger.Error(err, "Failed to create K8s client")
		os.Exit(1)
	}

	// Build the daemon (no RC client needed for direct testing)
	daemon := fleet.NewDaemonForTesting(logger, kubeClient)

	ctx := context.Background()

	switch action {
	case "start":
		if patch == "" {
			fmt.Fprintln(os.Stderr, "Error: -patch is required for start")
			os.Exit(1)
		}
		if experimentID == "" {
			fmt.Fprintln(os.Stderr, "Error: -id is required")
			os.Exit(1)
		}

		// Build installer config with the patch
		configID := "test-config-" + experimentID
		daemon.InjectConfig(configID, namespace, name, json.RawMessage(patch))

		fmt.Printf("Starting experiment %s on %s/%s\n", experimentID, namespace, name)
		err = daemon.StartExperiment(ctx, experimentID, configID)

	case "stop":
		if experimentID == "" {
			fmt.Fprintln(os.Stderr, "Error: -id is required")
			os.Exit(1)
		}
		fmt.Printf("Stopping experiment %s\n", experimentID)
		err = daemon.StopExperiment(ctx, experimentID)

	case "promote":
		if experimentID == "" {
			fmt.Fprintln(os.Stderr, "Error: -id is required")
			os.Exit(1)
		}
		fmt.Printf("Promoting experiment %s\n", experimentID)
		err = daemon.PromoteExperiment(ctx, experimentID)

	case "status":
		dda := &v2alpha1.DatadogAgent{}
		if getErr := kubeClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, dda); getErr != nil {
			logger.Error(getErr, "Failed to get DDA")
			os.Exit(1)
		}
		if dda.Status.Experiment == nil {
			fmt.Println("No experiment running")
		} else {
			data, _ := json.MarshalIndent(dda.Status.Experiment, "", "  ")
			fmt.Printf("Experiment:\n%s\n", string(data))
		}
		fmt.Printf("APM enabled: %v\n", dda.Spec.Features != nil && dda.Spec.Features.APM != nil && dda.Spec.Features.APM.Enabled != nil && *dda.Spec.Features.APM.Enabled)
		return

	default:
		fmt.Fprintf(os.Stderr, "Unknown action: %s\n", action)
		os.Exit(1)
	}

	if err != nil {
		logger.Error(err, "Failed", "action", action)
		os.Exit(1)
	}
	fmt.Printf("Success: %s experiment %s\n", action, experimentID)

	// Print current state
	dda := &v2alpha1.DatadogAgent{}
	if getErr := kubeClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, dda); getErr == nil {
		if dda.Status.Experiment != nil {
			data, _ := json.MarshalIndent(dda.Status.Experiment, "", "  ")
			fmt.Printf("Experiment status:\n%s\n", string(data))
		}
	}
}

// Unused but needed for the fleetManagementOperation import
var _ = schema.GroupVersionKind{}
