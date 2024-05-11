// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package limits

import "testing"

func TestCalculatePodToCreateAndDelete(t *testing.T) {
	type args struct {
		params Parameters
	}
	tests := []struct {
		name           string
		args           args
		wantNbCreation int
		wantNbDeletion int
	}{
		{
			name: "10 nodes, no pods already exist, maxPodCreation=20",
			args: args{
				params: Parameters{
					NbNodes: 10,
					NbPods:  0,

					NbAvailablesPod:   0,
					NbCreatedPod:      0,
					MaxUnavailablePod: 5,
					MaxPodCreation:    20,
				},
			},
			wantNbCreation: 10,
			wantNbDeletion: 0,
		},
		{
			name: "10 nodes, no pods already exist, maxPodCreation=5",
			args: args{
				params: Parameters{
					NbNodes: 10,
					NbPods:  0,

					NbAvailablesPod:   0,
					NbCreatedPod:      0,
					MaxUnavailablePod: 5,
					MaxPodCreation:    5,
				},
			},
			wantNbCreation: 5,
			wantNbDeletion: 0,
		},
		{
			name: "10 nodes, 7 pods exist not ready, maxPodCreation=5",
			args: args{
				params: Parameters{
					NbNodes: 10,
					NbPods:  7,

					NbAvailablesPod:   0,
					NbCreatedPod:      0,
					MaxUnavailablePod: 5,
					MaxPodCreation:    5,
				},
			},
			wantNbCreation: 3,
			wantNbDeletion: 0,
		},
		{
			name: "10 nodes, 7 pods exist and 5ready, maxPodCreation=5",
			args: args{
				params: Parameters{
					NbNodes: 10,
					NbPods:  7,

					NbAvailablesPod:   5,
					NbCreatedPod:      5,
					MaxUnavailablePod: 5,
					MaxPodCreation:    5,
				},
			},
			wantNbCreation: 3,
			wantNbDeletion: 0,
		},
		{
			name: "10 nodes, 10 pods exist and 7ready, maxPodCreation=5",
			args: args{
				params: Parameters{
					NbNodes: 10,
					NbPods:  10,

					NbAvailablesPod:   7,
					NbCreatedPod:      7,
					MaxUnavailablePod: 5,
					MaxPodCreation:    5,
				},
			},
			wantNbCreation: 0,
			wantNbDeletion: 2,
		},
		{
			name: "10 nodes, 10 pods exist and 10ready, maxPodCreation=5",
			args: args{
				params: Parameters{
					NbNodes: 10,
					NbPods:  10,

					NbAvailablesPod:   10,
					NbCreatedPod:      10,
					MaxUnavailablePod: 5,
					MaxPodCreation:    5,
				},
			},
			wantNbCreation: 0,
			wantNbDeletion: 5,
		},
		{
			name: "10 nodes, 10 pods exist and 10ready, maxPodCreation=5",
			args: args{
				params: Parameters{
					NbNodes: 10,
					NbPods:  10,

					NbAvailablesPod:   10,
					NbCreatedPod:      10,
					MaxUnavailablePod: 3,
					MaxPodCreation:    5,
				},
			},
			wantNbCreation: 0,
			wantNbDeletion: 3,
		},
		{
			name: "10 nodes, 10 pods exist and 10ready, maxPodCreation=5,MaxUnavailablePod=3",
			args: args{
				params: Parameters{
					NbNodes: 10,
					NbPods:  7,

					NbAvailablesPod:   7,
					NbCreatedPod:      7,
					MaxUnavailablePod: 3,
					MaxPodCreation:    5,
				},
			},
			wantNbCreation: 3,
			wantNbDeletion: 0,
		},
		{
			name: "10 nodes, 10 pods exist and 10ready, maxPodCreation=5,MaxUnavailablePod=3",
			args: args{
				params: Parameters{
					NbNodes: 10,
					NbPods:  10,

					NbAvailablesPod:   10,
					NbCreatedPod:      10,
					MaxUnavailablePod: 3,
					MaxPodCreation:    5,
				},
			},
			wantNbCreation: 0,
			wantNbDeletion: 3,
		},
		{
			name: "10 nodes, 10 pods exist and 10ready, maxPodCreation=5,MaxUnavailablePod=3",
			args: args{
				params: Parameters{
					NbNodes: 10,
					NbPods:  10,

					NbAvailablesPod:   9,
					NbCreatedPod:      9,
					MaxUnavailablePod: 3,
					MaxPodCreation:    5,
				},
			},
			wantNbCreation: 0,
			wantNbDeletion: 2,
		},
		{
			name: "10 nodes, 10 old ds pods exist and 0ready, maxPodCreation=5,MaxUnavailablePod=3",
			args: args{
				params: Parameters{
					NbNodes: 10,
					NbPods:  10,

					NbAvailablesPod:    0,
					NbOldAvailablesPod: 10,
					NbCreatedPod:       0,
					MaxUnavailablePod:  3,
					MaxPodCreation:     5,
				},
			},
			wantNbCreation: 0,
			wantNbDeletion: 3,
		},
		{
			name: "166 nodes, 149 old ds pods exist and 0ready, maxPodCreation=20,MaxUnavailablePod=19",
			args: args{
				params: Parameters{
					NbNodes: 166,
					NbPods:  149,

					NbAvailablesPod:    0,
					NbOldAvailablesPod: 149,
					NbCreatedPod:       0,
					MaxUnavailablePod:  19,
					MaxPodCreation:     20,
				},
			},
			wantNbCreation: 17,
			wantNbDeletion: 2,
		},
		{
			name: "10 nodes, 10 pods exist and 7 ready, 3 initially unready pods, maxPodCreation=5,MaxUnavailablePod=3",
			args: args{
				params: Parameters{
					NbNodes: 10,
					NbPods:  10,

					NbUnreadyPods:     3,
					NbAvailablesPod:   7,
					NbCreatedPod:      10,
					MaxUnavailablePod: 3,
					MaxPodCreation:    5,
				},
			},
			wantNbCreation: 0,
			wantNbDeletion: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNbCreation, gotNbDeletion := CalculatePodToCreateAndDelete(tt.args.params)
			if gotNbCreation != tt.wantNbCreation {
				t.Errorf("CalculatePodToCreateAndDelete() gotNbCreation = %v, want %v", gotNbCreation, tt.wantNbCreation)
			}
			if gotNbDeletion != tt.wantNbDeletion {
				t.Errorf("CalculatePodToCreateAndDelete() gotNbDeletion = %v, want %v", gotNbDeletion, tt.wantNbDeletion)
			}
		})
	}
}
