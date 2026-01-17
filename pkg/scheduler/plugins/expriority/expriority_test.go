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

package expriority

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"volcano.sh/volcano/pkg/scheduler/api"
)

func TestPriorityExpression_Matches(t *testing.T) {
	tests := []struct {
		name     string
		expr     PriorityExpression
		priority int32
		want     bool
	}{
		// In operator tests
		{
			name:     "In operator - match",
			expr:     PriorityExpression{Operator: OperatorIn, Values: []int32{1, 2, 3}},
			priority: 2,
			want:     true,
		},
		{
			name:     "In operator - no match",
			expr:     PriorityExpression{Operator: OperatorIn, Values: []int32{1, 2, 3}},
			priority: 4,
			want:     false,
		},
		{
			name:     "In operator - empty values",
			expr:     PriorityExpression{Operator: OperatorIn, Values: []int32{}},
			priority: 1,
			want:     false,
		},
		// NotIn operator tests
		{
			name:     "NotIn operator - not in list",
			expr:     PriorityExpression{Operator: OperatorNotIn, Values: []int32{1, 3}},
			priority: 2,
			want:     true,
		},
		{
			name:     "NotIn operator - in list",
			expr:     PriorityExpression{Operator: OperatorNotIn, Values: []int32{1, 3}},
			priority: 1,
			want:     false,
		},
		// Between operator tests
		{
			name:     "Between operator - in range",
			expr:     PriorityExpression{Operator: OperatorBetween, Values: []int32{1, 3}},
			priority: 2,
			want:     true,
		},
		{
			name:     "Between operator - at lower bound",
			expr:     PriorityExpression{Operator: OperatorBetween, Values: []int32{1, 3}},
			priority: 1,
			want:     true,
		},
		{
			name:     "Between operator - at upper bound",
			expr:     PriorityExpression{Operator: OperatorBetween, Values: []int32{1, 3}},
			priority: 3,
			want:     true,
		},
		{
			name:     "Between operator - below range",
			expr:     PriorityExpression{Operator: OperatorBetween, Values: []int32{1, 3}},
			priority: 0,
			want:     false,
		},
		{
			name:     "Between operator - above range",
			expr:     PriorityExpression{Operator: OperatorBetween, Values: []int32{1, 3}},
			priority: 4,
			want:     false,
		},
		{
			name:     "Between operator - reversed values",
			expr:     PriorityExpression{Operator: OperatorBetween, Values: []int32{3, 1}},
			priority: 2,
			want:     true,
		},
		{
			name:     "Between operator - insufficient values",
			expr:     PriorityExpression{Operator: OperatorBetween, Values: []int32{1}},
			priority: 1,
			want:     false,
		},
		// Lt operator tests
		{
			name:     "Lt operator - less than",
			expr:     PriorityExpression{Operator: OperatorLt, Values: []int32{5}},
			priority: 3,
			want:     true,
		},
		{
			name:     "Lt operator - equal",
			expr:     PriorityExpression{Operator: OperatorLt, Values: []int32{5}},
			priority: 5,
			want:     false,
		},
		{
			name:     "Lt operator - greater than",
			expr:     PriorityExpression{Operator: OperatorLt, Values: []int32{5}},
			priority: 7,
			want:     false,
		},
		// Gt operator tests
		{
			name:     "Gt operator - greater than",
			expr:     PriorityExpression{Operator: OperatorGt, Values: []int32{5}},
			priority: 7,
			want:     true,
		},
		{
			name:     "Gt operator - equal",
			expr:     PriorityExpression{Operator: OperatorGt, Values: []int32{5}},
			priority: 5,
			want:     false,
		},
		{
			name:     "Gt operator - less than",
			expr:     PriorityExpression{Operator: OperatorGt, Values: []int32{5}},
			priority: 3,
			want:     false,
		},
		// Lte operator tests
		{
			name:     "Lte operator - less than",
			expr:     PriorityExpression{Operator: OperatorLte, Values: []int32{5}},
			priority: 3,
			want:     true,
		},
		{
			name:     "Lte operator - equal",
			expr:     PriorityExpression{Operator: OperatorLte, Values: []int32{5}},
			priority: 5,
			want:     true,
		},
		{
			name:     "Lte operator - greater than",
			expr:     PriorityExpression{Operator: OperatorLte, Values: []int32{5}},
			priority: 7,
			want:     false,
		},
		// Gte operator tests
		{
			name:     "Gte operator - greater than",
			expr:     PriorityExpression{Operator: OperatorGte, Values: []int32{5}},
			priority: 7,
			want:     true,
		},
		{
			name:     "Gte operator - equal",
			expr:     PriorityExpression{Operator: OperatorGte, Values: []int32{5}},
			priority: 5,
			want:     true,
		},
		{
			name:     "Gte operator - less than",
			expr:     PriorityExpression{Operator: OperatorGte, Values: []int32{5}},
			priority: 3,
			want:     false,
		},
		// Unknown operator
		{
			name:     "Unknown operator",
			expr:     PriorityExpression{Operator: "Unknown", Values: []int32{5}},
			priority: 5,
			want:     false,
		},
		// Edge cases with negative priorities
		{
			name:     "Lt operator - negative priority",
			expr:     PriorityExpression{Operator: OperatorLt, Values: []int32{0}},
			priority: -1,
			want:     true,
		},
		{
			name:     "In operator - negative values",
			expr:     PriorityExpression{Operator: OperatorIn, Values: []int32{-1, 0}},
			priority: -1,
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.expr.Matches(tt.priority); got != tt.want {
				t.Errorf("PriorityExpression.Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrioritySelector_Matches(t *testing.T) {
	tests := []struct {
		name     string
		selector *PrioritySelector
		priority int32
		want     bool
	}{
		{
			name:     "nil selector",
			selector: nil,
			priority: 5,
			want:     false,
		},
		{
			name:     "empty expressions",
			selector: &PrioritySelector{AnyExpressions: []PriorityExpression{}},
			priority: 5,
			want:     false,
		},
		{
			name: "single expression - match",
			selector: &PrioritySelector{
				AnyExpressions: []PriorityExpression{
					{Operator: OperatorIn, Values: []int32{1, 2, 3}},
				},
			},
			priority: 2,
			want:     true,
		},
		{
			name: "single expression - no match",
			selector: &PrioritySelector{
				AnyExpressions: []PriorityExpression{
					{Operator: OperatorIn, Values: []int32{1, 2, 3}},
				},
			},
			priority: 5,
			want:     false,
		},
		{
			name: "multiple expressions - first matches (OR logic)",
			selector: &PrioritySelector{
				AnyExpressions: []PriorityExpression{
					{Operator: OperatorLt, Values: []int32{0}},
					{Operator: OperatorIn, Values: []int32{0}},
				},
			},
			priority: -1,
			want:     true,
		},
		{
			name: "multiple expressions - second matches (OR logic)",
			selector: &PrioritySelector{
				AnyExpressions: []PriorityExpression{
					{Operator: OperatorLt, Values: []int32{0}},
					{Operator: OperatorIn, Values: []int32{0}},
				},
			},
			priority: 0,
			want:     true,
		},
		{
			name: "multiple expressions - none matches",
			selector: &PrioritySelector{
				AnyExpressions: []PriorityExpression{
					{Operator: OperatorLt, Values: []int32{0}},
					{Operator: OperatorIn, Values: []int32{0}},
				},
			},
			priority: 1,
			want:     false,
		},
		{
			name: "complex selector - priority <= 0",
			selector: &PrioritySelector{
				AnyExpressions: []PriorityExpression{
					{Operator: OperatorLte, Values: []int32{0}},
				},
			},
			priority: 0,
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.selector.Matches(tt.priority); got != tt.want {
				t.Errorf("PrioritySelector.Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name      string
		arguments map[string]interface{}
		wantOrder []string
	}{
		{
			name:      "default config",
			arguments: map[string]interface{}{},
			wantOrder: []string{SortByPriority},
		},
		{
			name: "custom sort order",
			arguments: map[string]interface{}{
				"sortOrder": []string{SortByPriority, SortByCreationTime},
			},
			wantOrder: []string{SortByPriority, SortByCreationTime},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := New(tt.arguments)
			ep := plugin.(*exPriorityPlugin)

			if len(ep.config.SortOrder) != len(tt.wantOrder) {
				t.Errorf("SortOrder length = %v, want %v", len(ep.config.SortOrder), len(tt.wantOrder))
				return
			}

			for i, order := range ep.config.SortOrder {
				if order != tt.wantOrder[i] {
					t.Errorf("SortOrder[%d] = %v, want %v", i, order, tt.wantOrder[i])
				}
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

func TestMaxRunTimeAnnotationKey(t *testing.T) {
	key := "crater.raids.io/max-run-time"
	plugin := New(map[string]interface{}{
		"maxRunTimeAnnotationKey": key,
	})
	ep := plugin.(*exPriorityPlugin)
	if ep.config.MaxRunTimeAnnotationKey != key {
		t.Errorf("MaxRunTimeAnnotationKey = %v, want %v", ep.config.MaxRunTimeAnnotationKey, key)
	}
}

func TestIsTaskTimedOut(t *testing.T) {
	key := "crater.raids.io/max-run-time"
	plugin := New(map[string]interface{}{
		"maxRunTimeAnnotationKey": key,
	})
	ep := plugin.(*exPriorityPlugin)
	now := time.Date(2026, time.January, 1, 10, 0, 0, 0, time.UTC)

	newTask := func(start time.Time, annotation string) *api.TaskInfo {
		startTime := metav1.NewTime(start)
		annotations := map[string]string{}
		if annotation != "" {
			annotations[key] = annotation
		}
		return &api.TaskInfo{
			Namespace: "default",
			Name:      "test-pod",
			Pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: annotations,
				},
				Status: corev1.PodStatus{
					StartTime: &startTime,
				},
			},
		}
	}

	tests := []struct {
		name string
		task *api.TaskInfo
		want bool
	}{
		{
			name: "timed out task",
			task: newTask(now.Add(-2*time.Hour), "1h"),
			want: true,
		},
		{
			name: "not timed out task",
			task: newTask(now.Add(-30*time.Minute), "1h"),
			want: false,
		},
		{
			name: "missing annotation",
			task: newTask(now.Add(-2*time.Hour), ""),
			want: false,
		},
		{
			name: "missing start time",
			task: &api.TaskInfo{
				Namespace: "default",
				Name:      "test-pod",
				Pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							key: "1h",
						},
					},
				},
			},
			want: false,
		},
		{
			name: "invalid duration",
			task: newTask(now.Add(-2*time.Hour), "invalid"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ep.isTaskTimedOut(tt.task, now); got != tt.want {
				t.Errorf("isTaskTimedOut() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBlockingConfig(t *testing.T) {
	tests := []struct {
		name              string
		arguments         map[string]interface{}
		wantBlocking      bool
		wantBlockingScope string
	}{
		{
			name:              "no blocking config",
			arguments:         map[string]interface{}{},
			wantBlocking:      false,
			wantBlockingScope: BlockingScopeQueue, // default
		},
		{
			name: "blocking with default scope",
			arguments: map[string]interface{}{
				"blocking": PrioritySelector{
					AnyExpressions: []PriorityExpression{
						{Operator: OperatorGte, Values: []int32{100}},
					},
				},
			},
			wantBlocking:      true,
			wantBlockingScope: BlockingScopeQueue,
		},
		{
			name: "blocking with cluster scope",
			arguments: map[string]interface{}{
				"blocking": PrioritySelector{
					AnyExpressions: []PriorityExpression{
						{Operator: OperatorGte, Values: []int32{100}},
					},
				},
				"blockingScope": BlockingScopeCluster,
			},
			wantBlocking:      true,
			wantBlockingScope: BlockingScopeCluster,
		},
		{
			name: "blocking with queue scope",
			arguments: map[string]interface{}{
				"blocking": PrioritySelector{
					AnyExpressions: []PriorityExpression{
						{Operator: OperatorGte, Values: []int32{100}},
					},
				},
				"blockingScope": BlockingScopeQueue,
			},
			wantBlocking:      true,
			wantBlockingScope: BlockingScopeQueue,
		},
		{
			name: "blocking with invalid scope defaults to queue",
			arguments: map[string]interface{}{
				"blocking": PrioritySelector{
					AnyExpressions: []PriorityExpression{
						{Operator: OperatorGte, Values: []int32{100}},
					},
				},
				"blockingScope": "invalid",
			},
			wantBlocking:      true,
			wantBlockingScope: BlockingScopeQueue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := New(tt.arguments)
			ep := plugin.(*exPriorityPlugin)

			hasBlocking := ep.config.Blocking != nil
			if hasBlocking != tt.wantBlocking {
				t.Errorf("Blocking configured = %v, want %v", hasBlocking, tt.wantBlocking)
			}

			if ep.config.BlockingScope != tt.wantBlockingScope {
				t.Errorf("BlockingScope = %v, want %v", ep.config.BlockingScope, tt.wantBlockingScope)
			}
		})
	}
}
