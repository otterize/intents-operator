# Otterize intents operator

<img title="Otter Manning Helm" src="./otterhelm.png" width=200 />


![build](https://github.com/otterize/intents-operator/actions/workflows/build.yaml/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/otterize/intents-operator/src)](https://goreportcard.com/report/github.com/otterize/intents-operator/src)
[![community](https://img.shields.io/badge/slack-Otterize_Slack-purple.svg?logo=slack)](https://joinslack.otterize.com)

[About](#about) | [Quick tutorial](https://docs.otterize.com/quick-tutorials/k8s-network-policies) | [How does the intents operator work?](#how-does-the-intents-operator-work) | [Contributing](#contributing) | [Slack](#slack)

## About
The Otterize intents operator is a tool used to easily automate the creation of network policies and Kafka ACLs
in a kubernetes cluster using a human-readable format via a custom resource.

Users declare which client should access which server (represented as the kind `ClientIntents`) and the operator automatically labels the relevant pods accordingly, creates the relevant network policies and Kafka ACLs.


An example of a `ClientIntents` resource enabling traffic from `checkoutservice` to `shippingservice` in namespace `default`:
```yaml
apiVersion: k8s.otterize.com/v1
kind: ClientIntents
metadata:
  name: checkoutservice
spec:
  service:
    name: checkoutservice
  calls:
    - name: shippingservice
      type: http
```

## How does the intents operator work?

### Identities
Pods in the cluster are dynamically labeled with their owner's identity. If a `ReplicaSet` named `myclient` owns 5 pods 
and a `Deployment` named `myserver` owns 3 pods, and we enable `myclient -> myserver` access via `ClientIntents`, all 5 
source pods would be able to access all 3 target pods.

Pod identities can be overridden using the custom annotation `intents.otterize.com/service-name` 
mentioning the desired service name in the value. This is useful, for example, for owner-less pods.

### Network policies
The intents operator automatically creates, updates and deletes network policies, and automatically labels client and server pods, to match declarations in client intents files.
The policies created are `Ingress` based, so the source pods are labeled with `access-to-<target>-enabled`.
The example above results in the following network policy being created: 
```yaml
Name: access-to-shippingservice-from-default # the namespace where the client resides
Spec:
  PodSelector: intents.otterize.com/server=shippingservice-default-33a0f0
  Allowing ingress traffic:
    To Port: <any> (traffic allowed to all ports)
    From:
      # The label is added to the client
      PodSelector: intents.otterize.com/access-shippingservice-default-33a0f0=true
  Policy Types: Ingress
```

For more usage example see our [Network Policy Tutorial](https://docs.otterize.com/quick-tutorials/k8s-network-policies).

### Kafka mTLS & ACLs
The intents operator automatically creates, updates, and deletes ACLs in Kafka clusters running within your Kubernetes cluster. It works together with SPIRE and the [Otterize SPIRE integration operator](https://github.com/otterize/spire-integration-operator) to easily enable secure access to Kafka from client pods, all in your Kubernetes cluster.

Read more about it in our [Secure Kafka Access Tutorial](https://docs.otterize.com/quick-tutorials/k8s-kafka-mtls).

## Development
Inside src/operator directory you may run make command.
Most useful command:
* `make build` to compile the go code
* `make deploy` to generate Kubernetes object deploy the project to your local cluster

## Contributing
1. Feel free to fork and open a pull request! Include tests and document your code in [Godoc style](https://go.dev/blog/godoc)
2. In your pull request, please refer to an existing issue or open a new one.
3. Changes to Kubernetes objects will make changes to the Helm chart in the [helm-charts repo](https://github.com/otterize/helm-charts), which is a submodule in this repository, so you'll need to open a PR there as well.
4. See our [Contributor License Agreement](https://github.com/otterize/cla/).

## Slack
[Join the Otterize Slack!](https://joinslack.otterize.com)
