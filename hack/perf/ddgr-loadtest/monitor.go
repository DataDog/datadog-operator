package main

import "fmt"

const monitorJSONTemplate = `{
  "name": "ddgr-loadtest %d",
  "type": "metric alert",
  "query": "avg(last_5m):avg:datadog.estimated_usage.hosts{loadtest:ddgr-perf,index:%d} > 999999",
  "message": "ddgr-perf-test rev=%d",
  "tags": ["loadtest:ddgr-perf", "index:%d"],
  "options": {"thresholds": {"critical": 999999}, "notify_no_data": false}
}`

// BuildMonitorJSON returns the JSON payload embedded in a DDGR's spec.jsonSpec
// for the given index (unique per resource) and rev (incremented by churn).
func BuildMonitorJSON(index, rev int) string {
	return fmt.Sprintf(monitorJSONTemplate, index, index, rev, index)
}
