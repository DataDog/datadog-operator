// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package common

import (
	"io"

	"github.com/olekukonko/tablewriter"
)

// newPodsTable returns a table to print pods.
func newPodsTable(out io.Writer) *tablewriter.Table {
	table := tablewriter.NewWriter(out)
	table.SetHeader([]string{"Pod", "Ready", "Phase", "Reason", "Not ready containers", "Restarts", "Node", "Node Ready", "Age"})
	table.SetBorders(tablewriter.Border{Left: false, Top: false, Right: false, Bottom: false})
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetRowLine(false)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetHeaderLine(false)

	return table
}
