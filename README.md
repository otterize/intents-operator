# Spifferize
Otterize-SPIFFE/SPIRE integration

## Kubernetes Operator
### Prerequisites 
```shell
brew install kustomize
```

### Build
```shell
cd src
./operator/build_locally_and_push.sh
```

### Deploy
```shell
cd src
./operator/deploy.sh
```

### Uninstall
```shell
./operator/undeploy.sh
```


## Integrating with SPIRE
### SPIRE Server & Agent Installation
Follow [SPIRE's Quickstart for Kubernetes](https://spiffe.io/docs/latest/try/getting-started-k8s/)

### Adding a server entry for spifferize's operator
```shell
kubectl exec -n spire spire-server-0 -- /opt/spire/bin/spire-server entry create \
    -spiffeID spiffe://example.org/ns/operator-system/sa/operator-system \
    -parentID spiffe://example.org/ns/spire/sa/spire-agent \
    -selector k8s:ns:operator-system -selector k8s:sa:operator-system -admin
```

### Integrating with k8s deployments
Add a label: `otterize/service-name: <servicename>`
