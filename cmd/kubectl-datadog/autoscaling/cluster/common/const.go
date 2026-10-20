// Package common holds what the autoscaling subcommands share and that has no
// dependencies, so that it can also be imported from the e2e module.
package common

// MaxClusterNameLength is the longest supported length of the cluster name.
//
// The cluster name is part of the name of several AWS resources.
// It is constrained by the `KarpenterNodeRole-${ClusterName}` IAM role name limited to 64 characters.
const MaxClusterNameLength = 64 - len("KarpenterNodeRole-")
