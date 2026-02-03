/*
Copyright 2024 The Volcano Authors.

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

package groupquota

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog/v2"

	"volcano.sh/volcano/pkg/scheduler/api"
	"volcano.sh/volcano/pkg/scheduler/framework"
)

// PluginName indicates name of volcano scheduler plugin.
const PluginName = "groupquota"

type groupquotaPlugin struct {
	// Arguments given for the plugin
	pluginArguments framework.Arguments
}

// New return groupquota plugin
func New(arguments framework.Arguments) framework.Plugin {
	return &groupquotaPlugin{pluginArguments: arguments}
}

func (gp *groupquotaPlugin) Name() string {
	return PluginName
}

func (gp *groupquotaPlugin) OnSessionOpen(ssn *framework.Session) {
	annotationKey := "example.com/group"
	if arg, ok := gp.pluginArguments["annotationKey"]; ok {
		if val, ok := arg.(string); ok {
			annotationKey = val
		}
	} else {
		klog.Warningf("groupquota plugin: annotationKey argument not provided, using default %s", annotationKey)
	}

	quota := v1.ResourceList{}
	if rm, ok := gp.pluginArguments["resourceMap"]; ok {
		if resMap, ok := rm.(map[interface{}]interface{}); ok {
			for k, v := range resMap {
				kStr, okK := k.(string)
				vStr, okV := v.(string)
				if !okK || !okV {
					klog.Warningf("groupquota plugin: resourceMap key/value is not string, skipping %v: %v", k, v)
					continue
				}
				q, err := resource.ParseQuantity(vStr)
				if err != nil {
					klog.Errorf("groupquota plugin: failed to parse quantity for %s: %v", kStr, err)
					continue
				}
				quota[v1.ResourceName(kStr)] = q
			}
		} else if resMap, ok := rm.(map[string]interface{}); ok {
			for k, v := range resMap {
				vStr, ok := v.(string)
				if !ok {
					klog.Warningf("groupquota plugin: resourceMap value for %s is not string, skipping", k)
					continue
				}
				q, err := resource.ParseQuantity(vStr)
				if err != nil {
					klog.Errorf("groupquota plugin: failed to parse quantity for %s: %v", k, err)
					continue
				}
				quota[v1.ResourceName(k)] = q
			}
		} else {
			klog.Warningf("groupquota plugin: resourceMap is not a map, got %T", rm)
		}
	}

	groupUsage := make(map[string]v1.ResourceList)
	overQuotaGroups := make(map[string]bool)

	for _, job := range ssn.Jobs {
		if !isJobAllocated(job) {
			continue
		}

		if job.PodGroup == nil || job.PodGroup.Annotations == nil {
			continue
		}

		groupName, found := job.PodGroup.Annotations[annotationKey]
		if !found {
			continue
		}

		if _, ok := groupUsage[groupName]; !ok {
			groupUsage[groupName] = v1.ResourceList{}
		}

		addResourceList(groupUsage[groupName], job.Allocated)
	}

	for group, usage := range groupUsage {
		if isOverQuota(usage, quota) {
			overQuotaGroups[group] = true
			klog.V(4).Infof("groupquota: group %s is over quota", group)
		}
	}

	jobOrderFn := func(l, r interface{}) int {
		lv := l.(*api.JobInfo)
		rv := r.(*api.JobInfo)

		lGroup := getJobGroup(lv, annotationKey)
		rGroup := getJobGroup(rv, annotationKey)

		lOver := overQuotaGroups[lGroup]
		rOver := overQuotaGroups[rGroup]

		if lOver && !rOver {
			return 1 // r > l (r has higher priority)
		}
		if !lOver && rOver {
			return -1 // l > r (l has higher priority)
		}

		return 0
	}

	ssn.AddJobOrderFn(gp.Name(), jobOrderFn)
}

func (gp *groupquotaPlugin) OnSessionClose(ssn *framework.Session) {}

// Helper functions

func isJobAllocated(job *api.JobInfo) bool {
	// Check if job has any allocated resources/tasks.
	// In volcano, if a job is in Running or partially allocated state, it holds resources.
	// We check job.Allocated which is maintained by volcano.
	return !job.Allocated.IsEmpty()
}

func getJobGroup(job *api.JobInfo, key string) string {
	if job.PodGroup == nil || job.PodGroup.Annotations == nil {
		return ""
	}
	return job.PodGroup.Annotations[key]
}

func addResourceList(list v1.ResourceList, res *api.Resource) {
	// Convert api.Resource to v1.ResourceList and add
	// Since api.Resource separates scalar and dimension resources

	if res == nil {
		return
	}

	if res.MilliCPU > 0 {
		cpu := list[v1.ResourceCPU]
		cpu.Add(*resource.NewMilliQuantity(int64(res.MilliCPU), resource.DecimalSI))
		list[v1.ResourceCPU] = cpu
	}

	if res.Memory > 0 {
		mem := list[v1.ResourceMemory]
		mem.Add(*resource.NewQuantity(int64(res.Memory), resource.BinarySI))
		list[v1.ResourceMemory] = mem
	}

	for name, val := range res.ScalarResources {
		rName := v1.ResourceName(name)
		q := list[rName]
		q.Add(*resource.NewQuantity(int64(val), resource.DecimalSI))
		list[rName] = q
	}
}

func isOverQuota(usage, quota v1.ResourceList) bool {
	for name, limit := range quota {
		used, ok := usage[name]
		if !ok {
			continue
		}
		if used.Cmp(limit) >= 0 {
			return true
		}
	}
	return false
}
