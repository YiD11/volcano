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

package priority

import (
	"slices"

	"k8s.io/klog/v2"
)

// Operator constants for priority expression matching
const (
	OperatorIn      = "In"
	OperatorNotIn   = "NotIn"
	OperatorBetween = "Between"
	OperatorLt      = "Lt"
	OperatorGt      = "Gt"
	OperatorLte     = "Lte"
	OperatorGte     = "Gte"
)

// PriorityExpression defines a single priority matching expression
type PriorityExpression struct {
	Operator string  `json:"operator"`
	Values   []int32 `json:"values"`
}

// Matches checks if the given priority matches this expression
func (expr *PriorityExpression) Matches(priority int32) bool {
	switch expr.Operator {
	case OperatorIn:
		return slices.Contains(expr.Values, priority)
	case OperatorNotIn:
		return !slices.Contains(expr.Values, priority)
	case OperatorBetween:
		if len(expr.Values) >= 2 {
			minVal, maxVal := expr.Values[0], expr.Values[1]
			if minVal > maxVal {
				minVal, maxVal = maxVal, minVal
			}
			return priority >= minVal && priority <= maxVal
		}
		return false
	case OperatorLt:
		return len(expr.Values) > 0 && priority < expr.Values[0]
	case OperatorGt:
		return len(expr.Values) > 0 && priority > expr.Values[0]
	case OperatorLte:
		return len(expr.Values) > 0 && priority <= expr.Values[0]
	case OperatorGte:
		return len(expr.Values) > 0 && priority >= expr.Values[0]
	default:
		klog.Warningf("Unknown priority expression operator: %s", expr.Operator)
		return false
	}
}

// PrioritySelector defines a set of priority expressions combined with OR logic (anyExpressions)
type PrioritySelector struct {
	AnyExpressions []PriorityExpression `json:"anyExpressions"`
}

// Matches checks if the given priority matches any of the expressions (OR logic)
func (sel *PrioritySelector) Matches(priority int32) bool {
	if sel == nil || len(sel.AnyExpressions) == 0 {
		return false
	}
	for _, expr := range sel.AnyExpressions {
		if expr.Matches(priority) {
			return true
		}
	}
	return false
}
