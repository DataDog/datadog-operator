// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

const (
	PrivateActionRunnerConfigPath = "/etc/datadog-agent/privateactionrunner.yaml"

	privateActionRunnerVolumeNameSuffix         = "privateactionrunner-config"
	privateActionRunnerProcessVolumeNameSuffix  = "privateactionrunner-processes"
	privateActionRunnerFileName                 = "privateactionrunner.yaml"
	privateActionRunnerControlProcessFileName   = "datadog-agent-par-control.yaml"
	privateActionRunnerExecutorProcessFileName  = "datadog-agent-action-executor.yaml"
	privateActionRunnerProcessesPath            = "/opt/datadog-agent/processes.d"
	privateActionRunnerProcessManagerPath       = "/opt/datadog-agent/embedded/bin/dd-procmgrd"
	privateActionRunnerProcessManagerSocketPath = "/opt/datadog-agent/run/dd-procmgrd.sock"
	privateActionRunnerProcessEnvironmentPath   = "/opt/datadog-agent/run/privateactionrunner.env"
	privateActionRunnerPythonPath               = "/opt/datadog-agent/embedded/bin/python3"
	privateActionRunnerSuffix                   = "private-action-runner"

	hostVarLogVolumeName = "host-varlog"
	hostVarLogHostPath   = "/var/log"
	hostVarLogMountPath  = "/host/var/log"
)

// dd-procmgrd deliberately starts children with a clean environment. Persist
// the sidecar's Kubernetes-provided environment in its writable run directory,
// then replace the writer process with dd-procmgrd as PID 1. Values containing
// newlines cannot be represented by dd-procmgrd's environment-file format and
// are omitted.
const privateActionRunnerProcessManagerBootstrap = `import os
env_path = "` + privateActionRunnerProcessEnvironmentPath + `"
with open(env_path, "w", encoding="utf-8") as env_file:
    for key, value in os.environ.items():
        if "\n" not in key and "=" not in key and "\n" not in value:
            env_file.write(f"{key}={value}\n")
os.chmod(env_path, 0o600)
os.execv("` + privateActionRunnerProcessManagerPath + `", ["` + privateActionRunnerProcessManagerPath + `"])
`

const privateActionRunnerControlProcessConfig = `description: Datadog Private Action Runner - Control Plane
command: /opt/datadog-agent/embedded/bin/par-control
args:
  - --bootstrap-command
  - /opt/datadog-agent/embedded/bin/privateactionrunner
  - bootstrap-par-control
  - -c=/etc/datadog-agent/datadog.yaml
  - -E=/etc/datadog-agent/privateactionrunner.yaml
auto_start: true
stop_timeout: 180
restart: on-failure
restart_sec: 2
start_limit_interval_sec: 10
start_limit_burst: 5
environment_file: /opt/datadog-agent/run/privateactionrunner.env
stdout: inherit
stderr: inherit
`

const privateActionRunnerExecutorProcessConfig = `description: Datadog Private Action Runner - On-Demand Executor
command: /opt/datadog-agent/embedded/bin/privateactionrunner
args:
  - run-executor
  - -c=/etc/datadog-agent/datadog.yaml
  - -E=/etc/datadog-agent/privateactionrunner.yaml
auto_start: false
restart: never
environment_file: /opt/datadog-agent/run/privateactionrunner.env
stdout: inherit
stderr: inherit
`
