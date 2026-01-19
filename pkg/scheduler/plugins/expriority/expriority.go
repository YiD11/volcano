/*
Copyright 2018 The Kubernetes Authors.
Copyright 2018-2026 The Volcano Authors.

Modifications made by Volcano authors:
- Extended priority plugin with configurable priority selectors
- Added support for multiple sort orders (priority, creationTime)
- Added configurable preemptible and reclaimable priority ranges
- Added head-of-line blocking feature for priority-based job admission control

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
	"fmt"
	"time"

	"k8s.io/klog/v2"

	"volcano.sh/volcano/pkg/scheduler/api"
	"volcano.sh/volcano/pkg/scheduler/framework"
	"volcano.sh/volcano/pkg/scheduler/plugins/util"
	"volcano.sh/volcano/pkg/scheduler/plugins/util/priority"
)

// PluginName indicates name of volcano scheduler plugin.
const PluginName = "ex-priority"

// SortOrder constants
const (
	SortByPriority     = "priority"
	SortByCreationTime = "creationTime"
)

// BlockingScope constants define how blocking applies
const (
	BlockingScopeCluster = "cluster" // blocking applies cluster-wide
	BlockingScopeQueue   = "queue"   // blocking applies per-queue (default)
)

// Config holds the plugin configuration
type Config struct {
	SortOrder               []string                   `json:"sortOrder"`
	Preemptible             *priority.PrioritySelector `json:"preemptible"`
	Reclaimable             *priority.PrioritySelector `json:"reclaimable"`
	Blocking                *priority.PrioritySelector `json:"blocking"`      // priority range that can cause head-of-line blocking
	BlockingScope           string                     `json:"blockingScope"` // "cluster" or "queue" (default: "queue")
	MaxRunTimeAnnotationKey string                     `json:"maxRunTimeAnnotationKey"`
}

// exPriorityPlugin is the extended priority plugin
type exPriorityPlugin struct {
	pluginArguments framework.Arguments
	config          Config
}

// New returns an ex-priority plugin
func New(arguments framework.Arguments) framework.Plugin {
	ep := &exPriorityPlugin{
		pluginArguments: arguments,
		config: Config{
			SortOrder: []string{SortByPriority}, // default sort by priority only
		},
	}
	ep.parseArguments()
	return ep
}

func (ep *exPriorityPlugin) Name() string {
	return PluginName
}

// parseArguments parses plugin arguments into Config
func (ep *exPriorityPlugin) parseArguments() {
	// Parse sortOrder
	if sortOrder, ok := framework.Get[[]string](ep.pluginArguments, "sortOrder"); ok {
		ep.config.SortOrder = sortOrder
		klog.V(4).Infof("ex-priority plugin sortOrder: %v", ep.config.SortOrder)
	}

	// Parse preemptible
	if preemptible, ok := framework.Get[priority.PrioritySelector](ep.pluginArguments, "preemptible"); ok {
		ep.config.Preemptible = &preemptible
		klog.V(4).Infof("ex-priority plugin preemptible: %+v", ep.config.Preemptible)
	}

	// Parse reclaimable
	if reclaimable, ok := framework.Get[priority.PrioritySelector](ep.pluginArguments, "reclaimable"); ok {
		ep.config.Reclaimable = &reclaimable
		klog.V(4).Infof("ex-priority plugin reclaimable: %+v", ep.config.Reclaimable)
	}

	// Parse blocking
	if blocking, ok := framework.Get[priority.PrioritySelector](ep.pluginArguments, "blocking"); ok {
		ep.config.Blocking = &blocking
		klog.V(4).Infof("ex-priority plugin blocking: %+v", ep.config.Blocking)
	}

	// Parse blockingScope (default: "queue")
	ep.config.BlockingScope = BlockingScopeQueue
	if blockingScope, ok := framework.Get[string](ep.pluginArguments, "blockingScope"); ok {
		if blockingScope == BlockingScopeCluster || blockingScope == BlockingScopeQueue {
			ep.config.BlockingScope = blockingScope
		}
		klog.V(4).Infof("ex-priority plugin blockingScope: %v", ep.config.BlockingScope)
	}

	// Parse maxRunTimeAnnotationKey (optional)
	if maxRunTimeAnnotationKey, ok := framework.Get[string](ep.pluginArguments, "maxRunTimeAnnotationKey"); ok {
		ep.config.MaxRunTimeAnnotationKey = maxRunTimeAnnotationKey
		klog.V(4).Infof("ex-priority plugin maxRunTimeAnnotationKey: %v", ep.config.MaxRunTimeAnnotationKey)
	}
}

// getTaskCreationTime returns the creation time of a task
func getTaskCreationTime(task *api.TaskInfo) time.Time {
	if task == nil || task.Pod == nil {
		return time.Time{}
	}
	if task.Pod.Status.StartTime != nil {
		return task.Pod.Status.StartTime.Time
	}
	if !task.Pod.CreationTimestamp.IsZero() {
		return task.Pod.CreationTimestamp.Time
	}
	return time.Time{}
}

func (ep *exPriorityPlugin) isTaskTimedOut(task *api.TaskInfo, now time.Time) bool {
	if ep.config.MaxRunTimeAnnotationKey == "" || task == nil || task.Pod == nil {
		return false
	}
	annotations := task.Pod.Annotations
	if annotations == nil {
		return false
	}
	value, ok := annotations[ep.config.MaxRunTimeAnnotationKey]
	if !ok || value == "" {
		return false
	}
	if task.Pod.Status.StartTime == nil {
		return false
	}
	maxRunTime, err := time.ParseDuration(value)
	if err != nil || maxRunTime <= 0 {
		klog.V(4).Infof("ex-priority plugin failed to parse maxRunTime annotation %q on task <%s/%s>: %v",
			ep.config.MaxRunTimeAnnotationKey, task.Namespace, task.Name, err)
		return false
	}
	deadline := task.Pod.Status.StartTime.Add(maxRunTime)
	return !deadline.After(now)
}

// compareTasks compares two tasks based on the configured sort order
func (ep *exPriorityPlugin) compareTasks(l, r *api.TaskInfo) int {
	for _, order := range ep.config.SortOrder {
		switch order {
		case SortByPriority:
			if l.Priority > r.Priority {
				return -1
			}
			if l.Priority < r.Priority {
				return 1
			}
		case SortByCreationTime:
			lTime := getTaskCreationTime(l)
			rTime := getTaskCreationTime(r)
			if !lTime.IsZero() && !rTime.IsZero() && !lTime.Equal(rTime) {
				if lTime.Before(rTime) {
					return -1
				}
				return 1
			}
		}
	}
	return 0
}

// compareJobs compares two jobs based on the configured sort order
func (ep *exPriorityPlugin) compareJobs(l, r *api.JobInfo) int {
	for _, order := range ep.config.SortOrder {
		switch order {
		case SortByPriority:
			if l.Priority > r.Priority {
				return -1
			}
			if l.Priority < r.Priority {
				return 1
			}
		case SortByCreationTime:
			if !l.CreationTimestamp.Time.IsZero() && !r.CreationTimestamp.Time.IsZero() {
				if l.CreationTimestamp.Before(&r.CreationTimestamp) {
					return -1
				}
				if r.CreationTimestamp.Before(&l.CreationTimestamp) {
					return 1
				}
			}
		}
	}
	return 0
}

// compareSubJobs compares two sub-jobs based on the configured sort order
func (ep *exPriorityPlugin) compareSubJobs(l, r *api.SubJobInfo) int {
	for _, order := range ep.config.SortOrder {
		switch order {
		case SortByPriority:
			if l.Priority > r.Priority {
				return -1
			}
			if l.Priority < r.Priority {
				return 1
			}
			// SubJobInfo doesn't have CreationTimestamp, so we skip it
		}
	}
	return 0
}

// hasBlockingJobAhead checks if there is a blocking-priority job ahead of the current job.
// A job is considered "ahead" if it has higher priority and matches the blocking selector.
// The scope of blocking (cluster-wide or per-queue) is determined by ep.config.BlockingScope.
func (ep *exPriorityPlugin) hasBlockingJobAhead(ssn *framework.Session, currentJob *api.JobInfo) bool {
	if ep.config.Blocking == nil {
		return false
	}

	for _, job := range ssn.Jobs {
		// Skip non-Pending jobs
		if !job.IsPending() {
			continue
		}
		// Skip the current job itself
		if job.UID == currentJob.UID {
			continue
		}
		// If scope is "queue", only consider jobs in the same queue
		if ep.config.BlockingScope == BlockingScopeQueue && job.Queue != currentJob.Queue {
			continue
		}
		// Check if the job has blocking priority and is higher priority than current job
		if ep.config.Blocking.Matches(job.Priority) && job.Priority > currentJob.Priority {
			klog.V(4).Infof("Job <%s/%s> (priority: %d) is blocked by job <%s/%s> (priority: %d)",
				currentJob.Namespace, currentJob.Name, currentJob.Priority,
				job.Namespace, job.Name, job.Priority)
			return true
		}
	}
	return false
}

func (ep *exPriorityPlugin) OnSessionOpen(ssn *framework.Session) {
	klog.V(4).Infof("Enter ex-priority plugin with config: %+v", ep.config)

	// Task order function
	taskOrderFn := func(l interface{}, r interface{}) int {
		lv := l.(*api.TaskInfo)
		rv := r.(*api.TaskInfo)

		klog.V(4).Infof("ExPriority TaskOrder: <%v/%v> priority is %v, <%v/%v> priority is %v",
			lv.Namespace, lv.Name, lv.Priority, rv.Namespace, rv.Name, rv.Priority)

		return ep.compareTasks(lv, rv)
	}
	ssn.AddTaskOrderFn(ep.Name(), taskOrderFn)

	// Job order function
	jobOrderFn := func(l, r interface{}) int {
		lv := l.(*api.JobInfo)
		rv := r.(*api.JobInfo)

		klog.V(4).Infof("ExPriority JobOrderFn: <%v/%v> priority: %d, <%v/%v> priority: %d",
			lv.Namespace, lv.Name, lv.Priority, rv.Namespace, rv.Name, rv.Priority)

		return ep.compareJobs(lv, rv)
	}
	ssn.AddJobOrderFn(ep.Name(), jobOrderFn)

	// SubJob order function
	subJobOrderFn := func(l, r interface{}) int {
		lv := l.(*api.SubJobInfo)
		rv := r.(*api.SubJobInfo)

		klog.V(4).Infof("ExPriority SubJobOrderFn: <%v> priority: %d, <%v> priority: %d",
			lv.UID, lv.Priority, rv.UID, rv.Priority)

		return ep.compareSubJobs(lv, rv)
	}
	ssn.AddSubJobOrderFn(ep.Name(), subJobOrderFn)

	// Job enqueueable function - implements head-of-line blocking at enqueue phase
	if ep.config.Blocking != nil {
		jobEnqueueableFn := func(obj interface{}) int {
			job := obj.(*api.JobInfo)

			// If the job itself is a blocking-priority job, allow it to be enqueued
			if ep.config.Blocking.Matches(job.Priority) {
				return util.Permit
			}

			// If there's a higher-priority blocking job ahead, reject enqueuing
			if ep.hasBlockingJobAhead(ssn, job) {
				klog.V(3).Infof("Job <%s/%s> enqueue blocked due to head-of-line blocking",
					job.Namespace, job.Name)
				return util.Reject
			}

			return util.Abstain
		}
		ssn.AddJobEnqueueableFn(ep.Name(), jobEnqueueableFn)

		// Job valid function - implements head-of-line blocking at allocate phase
		jobValidFn := func(obj interface{}) *api.ValidateResult {
			job := obj.(*api.JobInfo)

			// Skip blocking check for blocking-priority jobs themselves
			if ep.config.Blocking.Matches(job.Priority) {
				return nil
			}

			// If there's a higher-priority blocking job ahead, reject allocation
			if ep.hasBlockingJobAhead(ssn, job) {
				return &api.ValidateResult{
					Pass:    false,
					Reason:  "blocked by higher priority job",
					Message: fmt.Sprintf("head-of-line blocking: higher priority job is pending (scope: %s)", ep.config.BlockingScope),
				}
			}

			return nil
		}
		ssn.AddJobValidFn(ep.Name(), jobValidFn)
	}

	// Preemptable function - determines which tasks can be preempted
	preemptableFn := func(preemptor *api.TaskInfo, preemptees []*api.TaskInfo) ([]*api.TaskInfo, int) {
		preemptorJob := ssn.Jobs[preemptor.Job]

		var victims []*api.TaskInfo
		now := time.Now()
		for _, preemptee := range preemptees {
			preempteeJob := ssn.Jobs[preemptee.Job]

			if ep.isTaskTimedOut(preemptee, now) {
				klog.V(4).Infof("Allow preempting timed-out task <%v/%v> of job priority %d",
					preemptee.Namespace, preemptee.Name, preempteeJob.Priority)
				victims = append(victims, preemptee)
				continue
			}

			// Check if preemptee is in the preemptible priority range
			if ep.config.Preemptible != nil {
				if !ep.config.Preemptible.Matches(preempteeJob.Priority) {
					klog.V(4).Infof("Cannot preempt task <%v/%v> because job priority %d is not in preemptible range",
						preemptee.Namespace, preemptee.Name, preempteeJob.Priority)
					continue
				}
			}

			if preempteeJob.UID != preemptorJob.UID {
				// Preemption between different Jobs: compare job priorities
				if preempteeJob.Priority >= preemptorJob.Priority {
					klog.V(4).Infof("Cannot preempt task <%v/%v> "+
						"because preemptee job has greater or equal job priority (%d) than preemptor (%d)",
						preemptee.Namespace, preemptee.Name, preempteeJob.Priority, preemptorJob.Priority)
				} else {
					victims = append(victims, preemptee)
				}
			} else {
				// Same job's different tasks: compare task priorities
				if preemptee.Priority >= preemptor.Priority {
					klog.V(4).Infof("Cannot preempt task <%v/%v> "+
						"because preemptee task has greater or equal task priority (%d) than preemptor (%d)",
						preemptee.Namespace, preemptee.Name, preemptee.Priority, preemptor.Priority)
				} else {
					victims = append(victims, preemptee)
				}
			}
		}

		klog.V(4).Infof("Victims from ExPriority plugin preemptableFn are %+v", victims)
		return victims, util.Permit
	}
	ssn.AddPreemptableFn(ep.Name(), preemptableFn)

	// Reclaimable function - determines which tasks can be reclaimed
	reclaimableFn := func(reclaimer *api.TaskInfo, reclaimees []*api.TaskInfo) ([]*api.TaskInfo, int) {
		reclaimerJob := ssn.Jobs[reclaimer.Job]

		var victims []*api.TaskInfo
		now := time.Now()
		for _, reclaimee := range reclaimees {
			reclaimeeJob := ssn.Jobs[reclaimee.Job]

			if ep.isTaskTimedOut(reclaimee, now) {
				klog.V(4).Infof("Allow reclaiming timed-out task <%v/%v> of job priority %d",
					reclaimee.Namespace, reclaimee.Name, reclaimeeJob.Priority)
				victims = append(victims, reclaimee)
				continue
			}

			// Check if reclaimee is in the reclaimable priority range
			if ep.config.Reclaimable != nil {
				if !ep.config.Reclaimable.Matches(reclaimeeJob.Priority) {
					klog.V(4).Infof("Cannot reclaim task <%v/%v> because job priority %d is not in reclaimable range",
						reclaimee.Namespace, reclaimee.Name, reclaimeeJob.Priority)
					continue
				}
			}

			if reclaimeeJob.UID != reclaimerJob.UID {
				// Reclaim between different Jobs: compare job priorities
				if reclaimeeJob.Priority >= reclaimerJob.Priority {
					klog.V(4).Infof("Cannot reclaim task <%v/%v> "+
						"because reclaimee job has greater or equal job priority (%d) than reclaimer (%d)",
						reclaimee.Namespace, reclaimee.Name, reclaimeeJob.Priority, reclaimerJob.Priority)
				} else {
					victims = append(victims, reclaimee)
				}
			} else {
				// Same job's different tasks: compare task priorities
				if reclaimee.Priority >= reclaimer.Priority {
					klog.V(4).Infof("Cannot reclaim task <%v/%v> "+
						"because reclaimee task has greater or equal task priority (%d) than reclaimer (%d)",
						reclaimee.Namespace, reclaimee.Name, reclaimee.Priority, reclaimer.Priority)
				} else {
					victims = append(victims, reclaimee)
				}
			}
		}

		klog.V(4).Infof("Victims from ExPriority plugin reclaimableFn are %+v", victims)
		return victims, util.Permit
	}
	ssn.AddReclaimableFn(ep.Name(), reclaimableFn)

	// Job starving function - determines if a job is starving for resources
	jobStarvingFn := func(obj interface{}) bool {
		ji := obj.(*api.JobInfo)
		return ji.ReadyTaskNum()+ji.WaitingTaskNum() < int32(len(ji.Tasks))
	}
	ssn.AddJobStarvingFns(ep.Name(), jobStarvingFn)
}

func (ep *exPriorityPlugin) OnSessionClose(ssn *framework.Session) {}
