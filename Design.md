# Rollout controller

## Introduction
  The Rollout controller periodically restarts all the deployments in the cluster which match a certain criteria. The matching criteria can be changed by deploying an appropriate flipper CR in the cluster.

## Functional Requirements
- The rollout-controller periodically restarts all the deployment which match certain criteria.
- By default, the matching criteria includes deployments in all namespaces and matching labels "mesh":"true" with interval of 10minutes.
- The Matching criteria can be updated by applying a CR of type 'flipper' and the controller takes the updated config to match and restart deployments.
  - Flipper CRD has properties interval, match.labels and list of namespace.

## Assumptions
- At a time only one flipper CR will be used for determining the criteria.

## Design
- Rollout Controller:
  - The controller is based on controller-runtime.
  - It primarily watches the Deployment and triggers rollout restart if they match the matching configuration.
  - Rollout restart is done by updating the field `obj.Spec.Template.ObjectMeta.Annotations["kubectl.kubernetes.io/restartedAt"]` and setting a new timestamp. This causes deployment to trigger a rolling restart of all pods. This is similar to what `kubectl rollout restart deployment...` command does.
  - Periodic restart is done by Requeuing the Events back into work queue with required restartInterval.
  - The Controller also watches any changes in the flipper CR and updates the matching configuration whenever a new and valid CR is applied to the cluster. It also reverts back to default config when the flipper CR is deleted from cluster.


## Testing