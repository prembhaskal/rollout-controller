# Rollout Controller

### Pre-requisites
- install below tools
  - go version 1.22 or higher
  - kustomize (for deploying)
  - make (building)
  - docker (for building image and virtualization (for kind))
  - kind (for creating local k8s cluster)

### Tests
- Run unit tests  
  `make test`
- Run integration tests  
  `make test-integration`

### Build controller
- regenerate the manifests - CRD, roles etc  
  `make manifests`
- build the binary  
  `make build`
- install CRD in clusters  
  `make install`

### Run locally
- export KUBECONFIG=<Path-to-Config> or set in ~/kube/config  
  `make run`

### Build and push docker image
- update image location in Makefile or pass in command line
- build docker image  
  `make docker-build`
  `make docker-build IMG=myregistry/myoperator:0.0.1`
- push image to repo, should docker login to repo before push  
  `make docker-push`

### Deploy the CRD and controller
- deploy the controller as deployment using previously pushed docker image as container  
  `make deploy`

### Test with sample data
- Create the namespace and apply the deployments
- Verify that deployments are restarted based on default config
- Deploy the flipper CR to change matching criteria
- Check the deployments are restarted based on new config

### demo steps
Demo steps
```bash
# cleanup
kubectl delete -f testdata/deployments/
kubectl delete flipper/mesh-flipper
make undeploy

# build and deploy
make docker-build docker-push
# deploys flipper crd and rollout controller
make deploy

# monitor logs in another terminal
kubectl -n rollout get pods
kubectl -n rollout logs -f <pod-name>

# create orders deployment
kubectl apply -f testdata/deployments/orders-deployment.yaml
# create search deployment
kubectl apply -f testdata/deployments/search-deployment.yaml

# update matching config which matches only inventory ns
kubectl apply -f testdata/flipper/mesh-inventory/mesh-inventory-flip.yaml

# create payment deployments
kubectl apply -f testdata/deployments/payment-deployment.yaml

# deploy new flipper
kubectl apply -f testdata/flipper/non-mesh-inventory/nonmesh-payment-flip.yaml

# delete flipper to revert to default config
kubectl delete flipper/mesh-flipper

```