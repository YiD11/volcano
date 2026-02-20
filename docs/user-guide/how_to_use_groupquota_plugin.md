# How to use GroupQuota plugin

## 1. Enable plugin in scheduler config

Edit the Volcano scheduler configmap and add `groupquota` in a tier with `EnabledJobOrder` behavior.

```yaml
kind: ConfigMap
apiVersion: v1
metadata:
  name: volcano-scheduler-configmap
  namespace: volcano-system
data:
  volcano-scheduler.conf: |
    actions: "enqueue, allocate, backfill"
    tiers:
    - plugins:
      - name: priority
      - name: gang
      - name: conformance
    - plugins:
      - name: drf
      - name: predicates
      - name: proportion
      - name: groupquota
        arguments:
          annotationKey: "volcano.sh/groupquota"
          resourceMap:
            cpu: "20"
            memory: "64Gi"
            nvidia.com/gpu: "8"
```

## 2. Mark PodGroups with group annotation

`groupquota` reads group identity from PodGroup annotation key configured by `annotationKey`.

Example PodGroup metadata:

```yaml
metadata:
  annotations:
    volcano.sh/groupquota: "team-a"
```

## 3. Behavior

- The plugin calculates current allocated resources for each group.
- If a group reaches or exceeds configured quota on any tracked resource, jobs in that group are de-prioritized in job ordering.
- This is a soft ordering policy, not a hard reject.

## 4. Notes

- Combine with fairness plugins (`drf`, `capacity`, `proportion`) for complete multi-tenant control.
- Validate tier ordering and plugin configuration in staging before production rollout.
