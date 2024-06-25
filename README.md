# Rollout Controller

### Pre-requisites
- install below tools
  - kustomize
  - docker (for building image and virtualization (for kind))
  - kind (for creating local k8s cluster)

### Develop controller
- Run unit tests  
  `make test`
- Run integration tests  
  `make test-integration`

### Build  controller
- regenerate the manifests - CRD, roles etc  
  `make manifests`
- build the binary  
  `make build`
- install CRD in clusters  
  `make install`

### Run locally
- export KUBECONFIG=<Path to Config> or set in ~/kube/.config  
  `make run`

### Build and push docker image
- update image location in Makefile or pass in command line
- build docker image  
  `make docker-build`
  `make docker-build IMG=myregistry/myoperator:0.0.1`
- push image to repo  
  `make docker-push`

### Deploy the CRD and controller
- deploy the controller as deployment using previously pushed docker image as container  
  `make deploy`

### Test with sample data
- Create the namespace and apply the deployments
- Verify that deployments are restarted based on default config
- Deploy the flipper CR to change matching criteria
- Check the deployments are restarted based on new config