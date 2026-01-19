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
	"sort"
	"time"

	"k8s.io/klog/v2"

	"volcano.sh/volcano/pkg/scheduler/api"
	"volcano.sh/volcano/pkg/scheduler/framework"
	"volcano.sh/volcano/pkg/scheduler/plugins/util/priority"
)

// PluginName indicates name of volcano scheduler plugin.
const PluginName = "time-priority"

// EscalationRule defines a single priority escalation rule
type EscalationRule struct {
	WaitingThreshold time.Duration              `json:"waitingThreshold"`
	SourcePriority   *priority.PrioritySelector `json:"sourcePriority"`
	TargetPriority   int32                      `json:"targetPriority"`
}

// Config holds the plugin configuration
type Config struct {
	Rules []EscalationRule `json:"rules"`
}

// timePriorityPlugin is the time-based priority escalation plugin
type timePriorityPlugin struct {
	pluginArguments framework.Arguments
	config          Config
}

// New returns a time-priority plugin
func New(arguments framework.Arguments) framework.Plugin {
	tp := &timePriorityPlugin{
		pluginArguments: arguments,
		config:          Config{},
	}
	tp.parseArguments()
	return tp
}

func (tp *timePriorityPlugin) Name() string {
	return PluginName
}

// RawRule is the raw configuration format from YAML
type RawRule struct {
	WaitingThreshold string                     `json:"waitingThreshold"`
	SourcePriority   *priority.PrioritySelector `json:"sourcePriority"`
	TargetPriority   int32                      `json:"targetPriority"`
}

// parseArguments parses plugin arguments into Config
func (tp *timePriorityPlugin) parseArguments() {
	// Parse rules array
	if rules, ok := framework.Get[[]RawRule](tp.pluginArguments, "rules"); ok {
		for i, rawRule := range rules {
			duration, err := time.ParseDuration(rawRule.WaitingThreshold)
			if err != nil {
				klog.Warningf("time-priority plugin: failed to parse waitingThreshold %q for rule %d: %v",
					rawRule.WaitingThreshold, i, err)
				continue
			}
			if duration <= 0 {
				klog.Warningf("time-priority plugin: invalid waitingThreshold %v for rule %d, must be positive",
					duration, i)
				continue
			}
			if rawRule.SourcePriority == nil {
				klog.Warningf("time-priority plugin: sourcePriority is required for rule %d", i)
				continue
			}

			rule := EscalationRule{
				WaitingThreshold: duration,
				SourcePriority:   rawRule.SourcePriority,
				TargetPriority:   rawRule.TargetPriority,
			}
			tp.config.Rules = append(tp.config.Rules, rule)
			klog.V(4).Infof("time-priority plugin: added rule %d: waitingThreshold=%v, targetPriority=%d",
				i, duration, rawRule.TargetPriority)
		}

		// Sort rules by WaitingThreshold descending (longest first for matching)
		sort.Slice(tp.config.Rules, func(i, j int) bool {
			return tp.config.Rules[i].WaitingThreshold > tp.config.Rules[j].WaitingThreshold
		})
	}

	klog.V(3).Infof("time-priority plugin initialized with %d rules", len(tp.config.Rules))
}

// getWaitingDuration returns how long the job has been waiting
func getWaitingDuration(job *api.JobInfo, now time.Time) time.Duration {
	if job == nil || job.CreationTimestamp.IsZero() {
		return 0
	}
	return now.Sub(job.CreationTimestamp.Time)
}



func (tp *timePriorityPlugin) OnSessionOpen(ssn *framework.Session) {
	klog.V(4).Infof("Enter time-priority plugin with %d rules", len(tp.config.Rules))

	if len(tp.config.Rules) == 0 {
		klog.V(3).Info("time-priority plugin: no rules configured, skipping")
		return
	}

	now := time.Now()

	// Directly modify job priorities based on waiting time
	// This ensures all other plugins see the escalated priority
	for _, job := range ssn.Jobs {
		if job == nil {
			continue
		}

		waitingDuration := getWaitingDuration(job, now)

		// Check rules in order (longest threshold first)
		for _, rule := range tp.config.Rules {
			if waitingDuration >= rule.WaitingThreshold {
				if rule.SourcePriority.Matches(job.Priority) {
					klog.V(3).Infof("time-priority plugin: job <%s/%s> priority escalated from %d to %d (waited %v >= %v)",
						job.Namespace, job.Name, job.Priority, rule.TargetPriority,
						waitingDuration, rule.WaitingThreshold)
					job.Priority = rule.TargetPriority
					break // Apply only the first matching rule (longest threshold)
				}
			}
		}
	}
}

func (tp *timePriorityPlugin) OnSessionClose(ssn *framework.Session) {}
