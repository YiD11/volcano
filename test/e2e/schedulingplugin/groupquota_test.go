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

package schedulingplugin

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	e2eutil "volcano.sh/volcano/test/e2e/util"
)

const (
	groupQuotaPluginName = "groupquota"
	groupAnnotationKey   = "example.com/group"
)

var _ = Describe("GroupQuota Plugin E2E", func() {
	It("prioritizes jobs from under-quota groups over over-quota groups", func() {
		cmc := e2eutil.NewConfigMapCase("volcano-system", "integration-scheduler-configmap")
		gqArgs := map[string]interface{}{
			"annotationKey": groupAnnotationKey,
			"resourceMap": map[string]string{
				"cpu": "1000m",
			},
		}
		modifier := func(sc *e2eutil.SchedulerConfiguration) bool {
			return upsertPlugin(sc, e2eutil.PluginOption{
				Name:      groupQuotaPluginName,
				Arguments: gqArgs,
			})
		}
		cmc.ChangeBy(func(data map[string]string) (changed bool, changedBefore map[string]string) {
			return e2eutil.ModifySchedulerConfig(data, modifier)
		})
		defer cmc.UndoChanged()

		ctx := e2eutil.InitTestContext(e2eutil.Options{
			NodesNumLimit:      1,
			NodesResourceLimit: e2eutil.CPU2Mem2,
		})
		defer e2eutil.CleanupTestContext(ctx)

		holderJob := e2eutil.CreateJobWithPodGroup(ctx, &e2eutil.JobSpec{
			Name: "groupquota-holder",
			Tasks: []e2eutil.TaskSpec{
				{
					Img:     e2eutil.DefaultNginxImage,
					Req:     e2eutil.CPU1Mem1,
					Min:     1,
					Rep:     1,
					Command: "sleep 30",
				},
			},
		}, "", map[string]string{groupAnnotationKey: "team-a"})
		err := e2eutil.WaitJobReady(ctx, holderJob)
		Expect(err).NotTo(HaveOccurred())

		teamAJob := e2eutil.CreateJobWithPodGroup(ctx, &e2eutil.JobSpec{
			Name: "groupquota-team-a",
			Tasks: []e2eutil.TaskSpec{
				{
					Img:      e2eutil.DefaultNginxImage,
					Req:      e2eutil.CPU1Mem1,
					Min:      1,
					Rep:      1,
					Command:  "sleep 30",
					SchGates: []corev1.PodSchedulingGate{{Name: "gate"}},
				},
			},
		}, "", map[string]string{groupAnnotationKey: "team-a"})
		err = e2eutil.WaitTasksPending(ctx, teamAJob, 1)
		Expect(err).NotTo(HaveOccurred())

		teamBJob := e2eutil.CreateJobWithPodGroup(ctx, &e2eutil.JobSpec{
			Name: "groupquota-team-b",
			Tasks: []e2eutil.TaskSpec{
				{
					Img:      e2eutil.DefaultNginxImage,
					Req:      e2eutil.CPU1Mem1,
					Min:      1,
					Rep:      1,
					Command:  "sleep 30",
					SchGates: []corev1.PodSchedulingGate{{Name: "gate"}},
				},
			},
		}, "", map[string]string{groupAnnotationKey: "team-b"})
		err = e2eutil.WaitTasksPending(ctx, teamBJob, 1)
		Expect(err).NotTo(HaveOccurred())

		err = e2eutil.RemovePodSchGates(ctx, teamAJob)
		Expect(err).NotTo(HaveOccurred())
		err = e2eutil.RemovePodSchGates(ctx, teamBJob)
		Expect(err).NotTo(HaveOccurred())

		err = e2eutil.WaitTasksReady(ctx, teamBJob, 1)
		Expect(err).NotTo(HaveOccurred())

		err = e2eutil.WaitTasksReady(ctx, teamAJob, 1)
		Expect(err).NotTo(HaveOccurred())
	})

	It("schedules jobs from the same under-quota group fairly", func() {
		cmc := e2eutil.NewConfigMapCase("volcano-system", "integration-scheduler-configmap")
		gqArgs := map[string]interface{}{
			"annotationKey": groupAnnotationKey,
			"resourceMap": map[string]string{
				"cpu": "4",
			},
		}
		modifier := func(sc *e2eutil.SchedulerConfiguration) bool {
			return upsertPlugin(sc, e2eutil.PluginOption{
				Name:      groupQuotaPluginName,
				Arguments: gqArgs,
			})
		}
		cmc.ChangeBy(func(data map[string]string) (changed bool, changedBefore map[string]string) {
			return e2eutil.ModifySchedulerConfig(data, modifier)
		})
		defer cmc.UndoChanged()

		ctx := e2eutil.InitTestContext(e2eutil.Options{
			NodesNumLimit:      1,
			NodesResourceLimit: e2eutil.CPU1Mem1,
		})
		defer e2eutil.CleanupTestContext(ctx)

		firstJob := e2eutil.CreateJobWithPodGroup(ctx, &e2eutil.JobSpec{
			Name: "groupquota-same-first",
			Tasks: []e2eutil.TaskSpec{
				{
					Img:      e2eutil.DefaultNginxImage,
					Req:      e2eutil.CPU1Mem1,
					Min:      1,
					Rep:      1,
					Command:  "sleep 30s",
					SchGates: []corev1.PodSchedulingGate{{Name: "gate"}},
				},
			},
		}, "", map[string]string{groupAnnotationKey: "team-c"})
		err := e2eutil.WaitTasksPending(ctx, firstJob, 1)
		Expect(err).NotTo(HaveOccurred())

		time.Sleep(2 * time.Second)

		secondJob := e2eutil.CreateJobWithPodGroup(ctx, &e2eutil.JobSpec{
			Name: "groupquota-same-second",
			Tasks: []e2eutil.TaskSpec{
				{
					Img:      e2eutil.DefaultNginxImage,
					Req:      e2eutil.CPU1Mem1,
					Min:      1,
					Rep:      1,
					Command:  "sleep 30s",
					SchGates: []corev1.PodSchedulingGate{{Name: "gate"}},
				},
			},
		}, "", map[string]string{groupAnnotationKey: "team-c"})
		err = e2eutil.WaitTasksPending(ctx, secondJob, 1)
		Expect(err).NotTo(HaveOccurred())

		err = e2eutil.RemovePodSchGates(ctx, firstJob)
		Expect(err).NotTo(HaveOccurred())
		err = e2eutil.RemovePodSchGates(ctx, secondJob)
		Expect(err).NotTo(HaveOccurred())

		err = e2eutil.WaitJobReady(ctx, firstJob)
		Expect(err).NotTo(HaveOccurred())
		err = e2eutil.WaitJobStatePending(ctx, secondJob)
		Expect(err).NotTo(HaveOccurred())
	})

	It("treats jobs without group annotation as not over quota", func() {
		cmc := e2eutil.NewConfigMapCase("volcano-system", "integration-scheduler-configmap")
		gqArgs := map[string]interface{}{
			"annotationKey": groupAnnotationKey,
			"resourceMap": map[string]string{
				"cpu": "500m",
			},
		}
		modifier := func(sc *e2eutil.SchedulerConfiguration) bool {
			return upsertPlugin(sc, e2eutil.PluginOption{
				Name:      groupQuotaPluginName,
				Arguments: gqArgs,
			})
		}
		cmc.ChangeBy(func(data map[string]string) (changed bool, changedBefore map[string]string) {
			return e2eutil.ModifySchedulerConfig(data, modifier)
		})
		defer cmc.UndoChanged()

		ctx := e2eutil.InitTestContext(e2eutil.Options{
			NodesNumLimit:      1,
			NodesResourceLimit: e2eutil.CPU1Mem1,
		})
		defer e2eutil.CleanupTestContext(ctx)

		noAnnotationJob := e2eutil.CreateJob(ctx, &e2eutil.JobSpec{
			Name: "groupquota-no-annotation",
			Tasks: []e2eutil.TaskSpec{
				{
					Img:     e2eutil.DefaultNginxImage,
					Req:     e2eutil.CPU1Mem1,
					Min:     1,
					Rep:     1,
					Command: "sleep 30s",
				},
			},
		})
		err := e2eutil.WaitJobReady(ctx, noAnnotationJob)
		Expect(err).NotTo(HaveOccurred())
	})
})

func upsertPlugin(sc *e2eutil.SchedulerConfiguration, plugin e2eutil.PluginOption) bool {
	for tierIdx := range sc.Tiers {
		for pluginIdx := range sc.Tiers[tierIdx].Plugins {
			if sc.Tiers[tierIdx].Plugins[pluginIdx].Name == plugin.Name {
				sc.Tiers[tierIdx].Plugins[pluginIdx] = plugin
				return true
			}
		}
	}
	if len(sc.Tiers) == 0 {
		sc.Tiers = append(sc.Tiers, e2eutil.Tier{Plugins: []e2eutil.PluginOption{plugin}})
		return true
	}
	sc.Tiers[0].Plugins = append([]e2eutil.PluginOption{plugin}, sc.Tiers[0].Plugins...)
	return true
}
