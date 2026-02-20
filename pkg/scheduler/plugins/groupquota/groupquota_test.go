/*
Copyright 2026 The Volcano Authors.

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
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	schedulingapi "volcano.sh/apis/pkg/apis/scheduling"
	"volcano.sh/volcano/pkg/scheduler/api"
	"volcano.sh/volcano/pkg/scheduler/conf"
	"volcano.sh/volcano/pkg/scheduler/framework"
	"volcano.sh/volcano/pkg/scheduler/uthelper"
)

func TestAddResourceListScalarUsesMilliQuantity(t *testing.T) {
	list := v1.ResourceList{}
	addResourceList(list, &api.Resource{
		ScalarResources: map[v1.ResourceName]float64{
			"nvidia.com/gpu": 1000,
		},
	})

	gpu := list[v1.ResourceName("nvidia.com/gpu")]
	if gpu.MilliValue() != 1000 {
		t.Fatalf("expected gpu milli value 1000, got %d", gpu.MilliValue())
	}
	if gpu.Value() != 1 {
		t.Fatalf("expected gpu value 1, got %d", gpu.Value())
	}
}

func TestGroupQuotaJobOrderWithMapStringStringArguments(t *testing.T) {
	trueValue := true
	arguments := framework.Arguments{
		annotationKeyArg: defaultAnnotationKey,
		resourceMapArg: map[string]string{
			"nvidia.com/gpu": "2",
		},
	}

	test := uthelper.TestCommonStruct{
		Name:    "groupquota job order",
		Plugins: map[string]framework.PluginBuilder{PluginName: New},
	}

	tiers := []conf.Tier{
		{
			Plugins: []conf.PluginOption{
				{
					Name:            PluginName,
					EnabledJobOrder: &trueValue,
					Arguments:       arguments,
				},
			},
		},
	}

	ssn := test.RegisterSession(tiers, nil)
	defer test.Close()

	ssn.Jobs = map[api.JobID]*api.JobInfo{
		"running-a": newJob("running-a", "team-a", api.NewResource(api.BuildResourceList("", "", api.ScalarResource{Name: "nvidia.com/gpu", Value: "2"}))),
		"running-b": newJob("running-b", "team-b", api.NewResource(api.BuildResourceList("", "", api.ScalarResource{Name: "nvidia.com/gpu", Value: "1"}))),
	}

	plugin := New(arguments)
	plugin.OnSessionOpen(ssn)

	pendingA := newJob("pending-a", "team-a", api.EmptyResource())
	pendingB := newJob("pending-b", "team-b", api.EmptyResource())

	if ssn.JobOrderFn(pendingA, pendingB) {
		t.Fatalf("expected over-quota group job to have lower priority")
	}
	if !ssn.JobOrderFn(pendingB, pendingA) {
		t.Fatalf("expected under-quota group job to have higher priority")
	}
}

func newJob(name, group string, allocated *api.Resource) *api.JobInfo {
	job := api.NewJobInfo(api.JobID(name))
	job.Name = name
	job.Namespace = "default"
	job.Allocated = allocated
	job.PodGroup = &api.PodGroup{
		PodGroup: schedulingapi.PodGroup{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      name,
				Annotations: map[string]string{
					defaultAnnotationKey: group,
				},
			},
		},
	}
	return job
}
