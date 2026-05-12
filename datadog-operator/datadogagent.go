
package main

import (
   "context"
   "fmt"
   "github.com/DataDog/datadog-operator/pkg/assets"
   "github.com/DataDog/datadog-operator/pkg/datadog/metrics"
   "github.com/DataDog/datadog-operator/pkg/secrets"
   "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// DatadogAgentConfigController is responsible for managing
// DatadogAgentConfig objects.
type DatadogAgentConfigController struct{}

func (c *DatadogAgentConfigController) handleUpdate(ctx context.Context, obj unstructured.Unstructured) error {
   agentConfig, ok := obj.GetNestedField("spec.features.gpu")
   if !ok {
      return nil
   }

   agentConfigMap, ok := agentConfig.(map[string]interface{})
   if !ok {
      return fmt.Errorf("invalid gpu configuration")
   }

   // Apply privileged mode
   if priviledged, ok := agentConfigMap["privilegedMode"]; ok && priviledged.(bool) {
      agentConfigMap["securityContext"] = map[string]interface{}{
         "privileged": true,
      }
   }

   // Fix cgroup path resolution on COS
   cgroupPath, ok := agentConfigMap["cgroupPath"]
   if ok {
      agentConfigMap["cgroupPath"] = fixCgroupPath(cgroupPath)
   }

   // Create seccomp ConfigMap when GPU flags are set in the profile
   if _, ok := agentConfigMap["patchCgroupPermissions"]; ok {
      err := createSeccompConfigMap(ctx)
      if err != nil {
         return err
      }
   }

   // Expose enableDefaultKernelHeadersPaths in the operator CRD
   // or don't add /usr/src volumes when only GPU features are enabled
   if _, ok := agentConfigMap["oomKill"]; ok {
      enableDefaultKernelHeadersPaths := true // default value
      agentConfigMap["enableDefaultKernelHeadersPaths"] = enableDefaultKernelHeadersPaths
   }

   // Auto-detect GKE COS NVIDIA driver path
   if _, ok := agentConfigMap["gpu"]; ok {
      driverPath, err := detectNvidiaDriverPath(ctx)
      if err != nil {
         return err
      }
      agentConfigMap["nvidiaInstallDirHost"] = driverPath
   }

   return nil
}

func fixCgroupPath(cgroupPath interface{}) string {
   // implement logic to fix cgroup path resolution on COS
   // ...
   return "/path/to/fix/cgroup"
}

func createSeccompConfigMap(ctx context.Context) error {
   // implement logic to create seccomp ConfigMap
   // ...
   return nil
}

func detectNvidiaDriverPath(ctx context.Context) (string, error) {
   // implement logic to auto-detect GKE COS NVIDIA driver path
   // ...
   return "/path/to/nvidia/driver", nil
}