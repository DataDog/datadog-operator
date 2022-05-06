package apm

const (
	// DefaultAPMPortName default APM port name for container and service
	DefaultAPMPortName = "traceport"
	// DefaultAPMPortNumber default APM port opened for container and service
	DefaultAPMPortNumber = 8126
	// DefaultAPMSocketPath default path used by the trace-agent for Unix Domain Socket (UDS)
	DefaultAPMSocketPath = "/var/run/datadog/apm.socket"

	// APMSocketVolumeName name of the volume created for the Unix Domain Socket (UDS)
	APMSocketVolumeName = "apmsocket"

	// DDAPMEnabledEnvVar Environment Variable to enable APM
	DDAPMEnabledEnvVar = "DD_APM_ENABLED"
	// DDAPMReceiverSocketEnvVar Environment Variable used to specify the path of the Unix Domain Socket (UDS)
	DDAPMReceiverSocketEnvVar = "DD_APM_RECEIVER_SOCKET"
)
