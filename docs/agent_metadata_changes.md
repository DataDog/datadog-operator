# Breaking Change: Updates to Agent DaemonSet Labels, Selectors & Names

Starting in **Operator v1.21.0**, we are introducing changes to the **labels, selectors, and naming conventions** used for the Agent DaemonSets. These updates aim to improve consistency and reduce the length of DaemonSet names managed by [Datadog Agent Profiles (DAP)][1].

These changes may affect **other Kubernetes resources** that rely on **matching these labels**, such as:

- Network Policies
- Service Mesh configurations
- Vertical Pod Autoscalers (VPA)
- Admission Controllers or Mutating Webhooks

If your setup makes any assumptions about the Agent pod labels or DaemonSet names (e.g., for targeting or exclusion), you may need to **update those configurations** to avoid unexpected behavior.

---

## What’s Changing?

| Operator Version | DaemonSet Type | Pod Label Change | Selector Change | DaemonSet Name Change |
|------------------|----------------|------------------|------------------|------------------------|
| **v1.18**        | Default DS     | _No change_       | _No change_       | _No change_            |
|                  | DAP DS         | `app.kubernetes.io/instance: <dda-name>-agent` → `<dap-name>-agent` | _No change_ | _No change_ |
| **v1.21**        | Default DS     | _No change_       | `agent.datadoghq.com/name: <dda-name>` → `agent.datadoghq.com/instance: <dda-name>-agent` | _No change_ |
|                  | DAP DS         | _No change_       | `agent.datadoghq.com/name: <dda-name>` → `agent.datadoghq.com/instance: <dap-name>-agent` | `datadog-agent-with-profile-<dda-name>-<dap-name>` → `<dap-name>-agent` |

---

## Recommended Migration Path

Due to the immutability of label selectors in Kubernetes, the Operator **cannot update DaemonSets in place**. Instead, it must delete and recreate them potentially leading to **Agent downtime** or **undesired disruption**.

To give users control over when label and name changes are applied, we introduced a metadata update mechanism in `v1.18`. It allows you to apply the changes coming in `v1.21` ahead of time using pod orphaning. This process is fully decoupled from the Operator upgrade and other DaemonSet updates and enables a **controlled, zero-downtime migration**.

### Migration Steps

1. **Upgrade Operator to v1.18 – v1.20**  
   This version introduces support for controlled metadata migration via pod orphaning.

2. **Annotate the `DatadogAgent` resource**  
   Add the following annotation:
   ```yaml
   metadata:
     annotations:
       agent.datadoghq.com/update-metadata: "true"

[1]: https://github.com/DataDog/datadog-operator/blob/main/docs/datadog_agent_profiles.md
