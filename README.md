# Spifferize
Otterize-SPIFFE/SPIRE integration

## Installation instructions
### Prerequisites
```shell
brew install kustomize
```

### SPIRE Server & Agent Installation
1. Follow [SPIRE's Quickstart for Kubernetes](https://spiffe.io/docs/latest/try/getting-started-k8s/)

2. Add a SPIRE server entry for spifferize's operator (replace example.com with your trust domain)
```shell
kubectl exec -n spire spire-server-0 -- /opt/spire/bin/spire-server entry create \
    -spiffeID spiffe://example.org/ns/operator-system \
    -parentID spiffe://example.org/ns/spire/sa/spire-agent \
    -selector k8s:ns:operator-system -admin
```

### Spifferize Operator Installation
1. Build operator image & push to ECR
```shell
cd src
./operator/build_locally_and_push.sh
```

2. Deploy the operator to your kubernetes cluster 
```shell
cd src
./operator/deploy.sh
```

3. Undeploy operator
```shell
./operator/undeploy.sh
```


## Usage
1. Label your pods with: `otterize/service-name: <servicename>`
2. The operator automatically adds and maintains a SPIRE entries matching pods by namespace + `otterize/service-name` label,
    Mapping them to SpiffeID `spiffe://<trustdomain>/env/<namespace>/service/<servicename>`
