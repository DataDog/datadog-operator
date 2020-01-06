// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagentdeployment

import (
	"reflect"
	"testing"

	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
)

func Test_eventInfo_getReason(t *testing.T) {
	type fields struct {
		objName      string
		objNamespace string
		objKind      string
		eventType    datadog.EventType
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "DaemonSet creation",
			fields: fields{
				objName:      "foo",
				objNamespace: "bar",
				objKind:      "DaemonSet",
				eventType:    datadog.CreationEvent,
			},
			want: "Create DaemonSet",
		},
		{
			name: "Service deletion",
			fields: fields{
				objName:      "foo",
				objNamespace: "bar",
				objKind:      "Service",
				eventType:    datadog.DeletionEvent,
			},
			want: "Delete Service",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ei := &eventInfo{
				objName:      tt.fields.objName,
				objNamespace: tt.fields.objNamespace,
				objKind:      tt.fields.objKind,
				eventType:    tt.fields.eventType,
			}
			if got := ei.getReason(); got != tt.want {
				t.Errorf("eventInfo.getReason() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_eventInfo_getMessage(t *testing.T) {
	type fields struct {
		objName      string
		objNamespace string
		objKind      string
		eventType    datadog.EventType
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "nominal case",
			fields: fields{
				objName:      "foo",
				objNamespace: "bar",
				objKind:      "DaemonSet",
				eventType:    datadog.CreationEvent,
			},
			want: "bar/foo",
		},
		{
			name: "empty namespace",
			fields: fields{
				objName:      "foo",
				objNamespace: "",
				objKind:      "ClusterRole",
				eventType:    datadog.CreationEvent,
			},
			want: "/foo",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ei := &eventInfo{
				objName:      tt.fields.objName,
				objNamespace: tt.fields.objNamespace,
				objKind:      tt.fields.objKind,
				eventType:    tt.fields.eventType,
			}
			if got := ei.getMessage(); got != tt.want {
				t.Errorf("eventInfo.getMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_eventInfo_getDDEvent(t *testing.T) {
	type fields struct {
		objName      string
		objNamespace string
		objKind      string
		eventType    datadog.EventType
	}
	tests := []struct {
		name   string
		fields fields
		want   datadog.Event
	}{
		{
			name: "DaemonSet creation",
			fields: fields{
				objName:      "foo",
				objNamespace: "bar",
				objKind:      "DaemonSet",
				eventType:    datadog.CreationEvent,
			},
			want: datadog.Event{
				Title: "Create DaemonSet bar/foo",
				Type:  datadog.CreationEvent,
			},
		},
		{
			name: "Service deletion",
			fields: fields{
				objName:      "foo",
				objNamespace: "bar",
				objKind:      "Service",
				eventType:    datadog.DeletionEvent,
			},
			want: datadog.Event{
				Title: "Delete Service bar/foo",
				Type:  datadog.DeletionEvent,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ei := &eventInfo{
				objName:      tt.fields.objName,
				objNamespace: tt.fields.objNamespace,
				objKind:      tt.fields.objKind,
				eventType:    tt.fields.eventType,
			}
			if got := ei.getDDEvent(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("eventInfo.getDDEvent() = %v, want %v", got, tt.want)
			}
		})
	}
}
