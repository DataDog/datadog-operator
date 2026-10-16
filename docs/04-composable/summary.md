# Composable node Agent DaemonSet: DDOT standalone POC

**Status:** POC, validated on kind · **Branch:** `composable-agent-ds`

## Problem

- DDOT standalone deployment in Kubernetes is supported only by upstream tooling
- Operator supports running it alongside core Agent
- Operator creates Agent Daemonset assuming core Agent is always present

## Goal

- Run standalone DDOT collector using Operator.
- Keep **the same DaemonSet**, switching is a normal rolling update, not a recreate. 
- Validate POC using a small set of telemetry collected in both modes.
- Use experimental annotation `experimental.agent.datadoghq.com/node-agent-mode: standalone` on the DatadogAgent.

## Scope and limitations

- Annotation only, not inferred from feature configs.
- Tag name differences between modes.
- Standalone drops everything which needs core Agent or DCA, not sure if some of these can be replaced by DDOT with different config.
- An arbitrary DDOT config can't simply be kept when the core Agent is disabled; some changes are required. For example:
  - **Hostname:** standalone DDOT reports the plain node name, while the core Agent reports `<node>-<cluster>` when a cluster name is set, so telemetry moves to a different host after the switch. The POC config sets the host explicitly in both modes. That also stops app SDKs from reporting the pod name as the host:
    ```yaml
                resource/k8s:
                  attributes:
                    - key: datadog.host.name
                      value: ${env:K8S_NODE_NAME}-${env:DD_CLUSTER_NAME}
                      action: upsert
    ```
    - `<node>-<cluster>` matches the core Agent on kind and other kubelet-derived hostnames. On cloud providers the core Agent uses the instance hostname instead.
    - The operator sets `K8S_NODE_NAME` only in standalone, so the Agent+DDOT config adds it through an override.
  - **Tagging:** `infraattributes` gets tags from the core Agent, so a config using it fails in standalone and needs `k8s_attributes` instead. The operator makes this swap only in its default config.

## Future work

- Operator to migrate and adopt DDOT managed by upstream OTEL tooling. 
- Scrappy design and implementation. Needs careful review if/how it fits with other Operator features, leaked resources between modes, etc.
- Make standalone FHP (alone or with DDOT) actually run and validate it; the POC renders the container but doesn't configure its runtime.
- Auditing feature level requirements and telemetry gaps.

Details: `docs/04-composable/test-setup/`.
