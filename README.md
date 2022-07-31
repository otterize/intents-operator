# SPIRE Integration Operator

## Installation instructions
### Prerequisites
```shell
brew install kustomize
```

### SPIRE Server & Agent Installation
1. Follow [SPIRE's Quickstart for Kubernetes](https://spiffe.io/docs/latest/try/getting-started-k8s/)

2. Register a SPIRE server entry for the spire-integration-operator  (replace example.com with your trust domain)
```shell
kubectl exec -n spire spire-server-0 -- /opt/spire/bin/spire-server entry create \
    -spiffeID spiffe://example.org/ns/spire-integration-operator-system \
    -parentID spiffe://example.org/ns/spire/sa/spire-agent \
    -selector k8s:ns:spire-integration-operator-system -admin
```

### Spire Integration Operator Installation
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

## Configuring Kafka Servers
TBD

## Configuring Kafka Clients
TBD