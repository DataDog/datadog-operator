# Breaking Change: Operator v1.18.0/v1.21.0 - Updates to Agent DaemonSet Labels, Selectors & Names

Starting in **Operator v1.18.0**, we are introducing changes to the **labels, selectors, and naming conventions** used for the Agent DaemonSets. These updates aim to improve consistency and reduce the length of DaemonSet names managed by [DatadogAgentProfiles (DAPs)][1].

These changes may affect other Kubernetes resources that rely on matching these labels, such as:

- Network Policies
- Service Mesh configurations
- Vertical Pod Autoscalers (VPA)
- Admission Controllers or Mutating Webhooks

If your setup makes any assumptions about the Agent pod labels or DaemonSet names (e.g., for targeting or exclusion), you may need to **update those configurations** to avoid unexpected behavior.

---

## What’s Changing?

### Default setup (DAPs disabled):
| Operator Version | DaemonSet Name Change | Pod Label Change | Selector Change |
|------------------|-----------------------|------------------|-----------------|
| **v1.21**        | _No change_           | _No change_      | `agent.datadoghq.com/name: <dda-name>` → `agent.datadoghq.com/instance: <dda-name>-agent` |


### DAPs enabled:
| Operator Version | DaemonSet Type | DaemonSet Name Change | Pod Label Change | Selector Change |
|------------------|----------------|-----------------------|------------------|-----------------|
| **v1.18**        | Default DS     | _No change_           | _No change_      | _No change_     |
|                  | DAP DS         | _No change_           | `app.kubernetes.io/instance: <dda-name>-agent` → `<dap-name>-agent` | _No change_ |
| **v1.21**        | Default DS     | _No change_           | _No change_      | `agent.datadoghq.com/name: <dda-name>` → `agent.datadoghq.com/instance: <dda-name>-agent` |
|                  | DAP DS         | `datadog-agent-with-profile-<dda-name>-<dap-name>` → `<dap-name>-agent` | _No change_       | `agent.datadoghq.com/name: <dda-name>` → `agent.datadoghq.com/instance: <dap-name>-agent` |

---

## Migration Paths

Due to the immutability of label selectors in Kubernetes, the Operator cannot update DaemonSets in place. Instead, it must delete and recreate them potentially leading to Agent downtime or undesired disruption.

In order to minimize downtime, the Operator can [orphan its dependent pods](2). This method keeps Agent Pods running while the Operator deletes and recreates DaemonSets with the necessary selector and name changes. This process is fully decoupled from the Operator upgrade and other DaemonSet updates and enables a zero-downtime migration.

For control over when label and name changes are applied, you can use following annotation (introduced for the DatadogAgent in `v1.18`) that allows you to apply the changes coming in `v1.21` ahead of time:
   ```yaml
   metadata:
     annotations:
       agent.datadoghq.com/update-metadata: "true"
   ```

### Default setup (DAPs disabled)

#### Operator ≥v1.18 and <v1.21

To update the Agent DaemonSet selector ahead of time, you can add the annotation `agent.datadoghq.com/update-metadata: "true"` to the DatadogAgent object.

#### Operator 1.21+

The Agent DaemonSet's pod selector will automatically be updated whether or not the `agent.datadoghq.com/update-metadata: "true"` annotation is present.

### DAPs enabled

#### Operator ≥v1.18 and <v1.21

Starting in Operator v1.18, the Operator will automatically change the following DAP DaemonSet and Pod label value: `app.kubernetes.io/instance: <dda-name>-agent` → `<dap-name>-agent`. After this change is rolled out, you can update the Agent DaemonSet selector and name ahead of time by adding the annotation `agent.datadoghq.com/update-metadata: "true"` to the DatadogAgent object.

#### Operator 1.21+

The Agent DaemonSet selector and name will automatically be updated whether or not the `agent.datadoghq.com/update-metadata: "true"` annotation is present.


[1]: https://github.com/DataDog/datadog-operator/blob/main/docs/datadog_agent_profiles.md
[2]: https://kubernetes.io/docs/tasks/administer-cluster/use-cascading-deletion/#set-orphan-deletion-policy
