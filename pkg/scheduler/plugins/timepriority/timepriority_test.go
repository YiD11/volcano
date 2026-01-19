/*
Copyright 2018 The Kubernetes Authors.
Copyright 2018-2026 The Volcano Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package timepriority

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"volcano.sh/volcano/pkg/scheduler/api"
	"volcano.sh/volcano/pkg/scheduler/plugins/util/priority"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name      string
		arguments map[string]interface{}
		wantRules int
	}{
		{
			name:      "no rules",
			arguments: map[string]interface{}{},
			wantRules: 0,
		},
		{
			name: "single rule",
			arguments: map[string]interface{}{
				"rules": []RawRule{
					{
						WaitingThreshold: "10m",
						SourcePriority: &priority.PrioritySelector{
							AnyExpressions: []priority.PriorityExpression{
								{Operator: priority.OperatorBetween, Values: []int32{0, 50}},
							},
						},
						TargetPriority: 100,
					},
				},
			},
			wantRules: 1,
		},
		{
			name: "multiple rules",
			arguments: map[string]interface{}{
				"rules": []RawRule{
					{
						WaitingThreshold: "10m",
						SourcePriority: &priority.PrioritySelector{
							AnyExpressions: []priority.PriorityExpression{
								{Operator: priority.OperatorBetween, Values: []int32{0, 50}},
							},
						},
						TargetPriority: 100,
					},
					{
						WaitingThreshold: "20m",
						SourcePriority: &priority.PrioritySelector{
							AnyExpressions: []priority.PriorityExpression{
								{Operator: priority.OperatorBetween, Values: []int32{0, 50}},
							},
						},
						TargetPriority: 200,
					},
				},
			},
			wantRules: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := New(tt.arguments)
			tp := plugin.(*timePriorityPlugin)

			if len(tp.config.Rules) != tt.wantRules {
				t.Errorf("Rules count = %v, want %v", len(tp.config.Rules), tt.wantRules)
			}
		})
	}
}

func TestPluginName(t *testing.T) {
	plugin := New(map[string]interface{}{})
	if plugin.Name() != PluginName {
		t.Errorf("Name() = %v, want %v", plugin.Name(), PluginName)
	}
}

func TestRulesSortOrder(t *testing.T) {
	arguments := map[string]interface{}{
		"rules": []RawRule{
			{
				WaitingThreshold: "5m",
				SourcePriority: &priority.PrioritySelector{
					AnyExpressions: []priority.PriorityExpression{
						{Operator: priority.OperatorLte, Values: []int32{50}},
					},
				},
				TargetPriority: 100,
			},
			{
				WaitingThreshold: "20m",
				SourcePriority: &priority.PrioritySelector{
					AnyExpressions: []priority.PriorityExpression{
						{Operator: priority.OperatorLte, Values: []int32{50}},
					},
				},
				TargetPriority: 300,
			},
			{
				WaitingThreshold: "10m",
				SourcePriority: &priority.PrioritySelector{
					AnyExpressions: []priority.PriorityExpression{
						{Operator: priority.OperatorLte, Values: []int32{50}},
					},
				},
				TargetPriority: 200,
			},
		},
	}

	plugin := New(arguments)
	tp := plugin.(*timePriorityPlugin)

	// Rules should be sorted by WaitingThreshold descending
	expectedOrder := []time.Duration{20 * time.Minute, 10 * time.Minute, 5 * time.Minute}
	for i, rule := range tp.config.Rules {
		if rule.WaitingThreshold != expectedOrder[i] {
			t.Errorf("Rule[%d].WaitingThreshold = %v, want %v", i, rule.WaitingThreshold, expectedOrder[i])
		}
	}
}

func TestOnSessionOpenPriorityEscalation(t *testing.T) {
	arguments := map[string]interface{}{
		"rules": []RawRule{
			{
				WaitingThreshold: "10m",
				SourcePriority: &priority.PrioritySelector{
					AnyExpressions: []priority.PriorityExpression{
						{Operator: priority.OperatorBetween, Values: []int32{0, 50}},
					},
				},
				TargetPriority: 100,
			},
			{
				WaitingThreshold: "20m",
				SourcePriority: &priority.PrioritySelector{
					AnyExpressions: []priority.PriorityExpression{
						{Operator: priority.OperatorBetween, Values: []int32{0, 50}},
					},
				},
				TargetPriority: 200,
			},
		},
	}

	plugin := New(arguments)
	tp := plugin.(*timePriorityPlugin)

	tests := []struct {
		name             string
		waitDuration     time.Duration
		originalPriority int32
		wantPriority     int32
	}{
		{
			name:             "job not waited long enough - no escalation",
			waitDuration:     5 * time.Minute,
			originalPriority: 10,
			wantPriority:     10,
		},
		{
			name:             "job waited 10m - escalate to 100",
			waitDuration:     10 * time.Minute,
			originalPriority: 10,
			wantPriority:     100,
		},
		{
			name:             "job waited 20m - escalate to 200",
			waitDuration:     20 * time.Minute,
			originalPriority: 10,
			wantPriority:     200,
		},
		{
			name:             "high priority job - no escalation",
			waitDuration:     30 * time.Minute,
			originalPriority: 100,
			wantPriority:     100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test getWaitingDuration logic indirectly
			job := &api.JobInfo{
				Name:              "test-job",
				Namespace:         "default",
				Priority:          tt.originalPriority,
				CreationTimestamp: metav1.NewTime(time.Now().Add(-tt.waitDuration)),
			}

			waitDuration := getWaitingDuration(job, time.Now())

			// Verify that waiting duration is calculated correctly (with some tolerance)
			if tt.waitDuration > 0 && (waitDuration < tt.waitDuration-time.Second || waitDuration > tt.waitDuration+time.Second) {
				t.Errorf("getWaitingDuration() = %v, want approximately %v", waitDuration, tt.waitDuration)
			}

			// Test rule matching logic
			matched := false
			for _, rule := range tp.config.Rules {
				if waitDuration >= rule.WaitingThreshold && rule.SourcePriority.Matches(job.Priority) {
					matched = true
					if rule.TargetPriority != tt.wantPriority {
						// Only check if we expected escalation
						if tt.wantPriority != tt.originalPriority {
							t.Errorf("Expected targetPriority %d, got %d", tt.wantPriority, rule.TargetPriority)
						}
					}
					break
				}
			}

			// If no escalation expected, verify no match
			if tt.wantPriority == tt.originalPriority && matched {
				t.Errorf("Expected no escalation but rule matched")
			}
		})
	}
}

func TestGetWaitingDuration(t *testing.T) {
	now := time.Date(2026, time.January, 1, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		job          *api.JobInfo
		wantDuration time.Duration
	}{
		{
			name:         "nil job",
			job:          nil,
			wantDuration: 0,
		},
		{
			name: "job with creation time",
			job: &api.JobInfo{
				CreationTimestamp: metav1.NewTime(now.Add(-15 * time.Minute)),
			},
			wantDuration: 15 * time.Minute,
		},
		{
			name: "job with zero creation time",
			job: &api.JobInfo{
				CreationTimestamp: metav1.Time{},
			},
			wantDuration: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getWaitingDuration(tt.job, now)
			if got != tt.wantDuration {
				t.Errorf("getWaitingDuration() = %v, want %v", got, tt.wantDuration)
			}
		})
	}
}

func TestInvalidRules(t *testing.T) {
	tests := []struct {
		name      string
		arguments map[string]interface{}
		wantRules int
	}{
		{
			name: "invalid duration format",
			arguments: map[string]interface{}{
				"rules": []RawRule{
					{
						WaitingThreshold: "invalid",
						SourcePriority: &priority.PrioritySelector{
							AnyExpressions: []priority.PriorityExpression{
								{Operator: priority.OperatorLte, Values: []int32{50}},
							},
						},
						TargetPriority: 100,
					},
				},
			},
			wantRules: 0,
		},
		{
			name: "negative duration",
			arguments: map[string]interface{}{
				"rules": []RawRule{
					{
						WaitingThreshold: "-10m",
						SourcePriority: &priority.PrioritySelector{
							AnyExpressions: []priority.PriorityExpression{
								{Operator: priority.OperatorLte, Values: []int32{50}},
							},
						},
						TargetPriority: 100,
					},
				},
			},
			wantRules: 0,
		},
		{
			name: "missing source priority",
			arguments: map[string]interface{}{
				"rules": []RawRule{
					{
						WaitingThreshold: "10m",
						SourcePriority:   nil,
						TargetPriority:   100,
					},
				},
			},
			wantRules: 0,
		},
		{
			name: "mix of valid and invalid rules",
			arguments: map[string]interface{}{
				"rules": []RawRule{
					{
						WaitingThreshold: "invalid",
						SourcePriority: &priority.PrioritySelector{
							AnyExpressions: []priority.PriorityExpression{
								{Operator: priority.OperatorLte, Values: []int32{50}},
							},
						},
						TargetPriority: 100,
					},
					{
						WaitingThreshold: "10m",
						SourcePriority: &priority.PrioritySelector{
							AnyExpressions: []priority.PriorityExpression{
								{Operator: priority.OperatorLte, Values: []int32{50}},
							},
						},
						TargetPriority: 200,
					},
				},
			},
			wantRules: 1, // only the valid rule
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := New(tt.arguments)
			tp := plugin.(*timePriorityPlugin)

			if len(tp.config.Rules) != tt.wantRules {
				t.Errorf("Rules count = %v, want %v", len(tp.config.Rules), tt.wantRules)
			}
		})
	}
}
