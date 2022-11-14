# Otterize intents operator

<img title="Otter Manning Helm" src="./otterhelm.png" width=200 />


![build](https://github.com/otterize/intents-operator/actions/workflows/build.yaml/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/otterize/intents-operator/src)](https://goreportcard.com/report/github.com/otterize/intents-operator/src)
[![community](https://img.shields.io/badge/slack-Otterize_Slack-purple.svg?logo=slack)](https://joinslack.otterize.com)

* [About](#about) 
* [Quick tutorials](https://docs.otterize.com/quick-tutorials/)
* [How does the intents operator work?](#how-does-the-intents-operator-work)
  * [Network policies](#network-policies)
  * [Kafka mTLS & ACLs](#kafka-mtls--acls)
  * [Deducing workload identities](#identities)
* [Bootstrapping](#bootstrapping)
* [Read more](#read-more)
* [Development](#development)
* [Contributing](#contributing)
* [Slack](#slack)


## About
The Otterize intents operator is a tool used to easily automate the creation of network policies and Kafka ACLs
in a kubernetes cluster using a human-readable format via a custom resource.

Users declare which client intends to access which server (represented as the kind `ClientIntents`) and the operator automatically labels the relevant pods accordingly, and creates the relevant network policies and Kafka ACLs.


An example of a `ClientIntents` resource enabling traffic from 
`my-client` to `web-server` and `kafka-server`:
```yaml
apiVersion: k8s.otterize.com/v1alpha1
kind: ClientIntents
metadata:
  name: intents-sample
spec:
  service:
    name: my-client
  calls:
    - name: web-server
      type: http
    - name: kafka-server
      type: kafka
```

## How does the intents operator work?

### Network policies
The intents operator automatically creates, updates and deletes network policies, and automatically labels client and server pods, to match declarations in client intents files.
The policies created are `Ingress` based, so source pods are labeled with a `can-access-<target>=true` 
while destination pods are labeled with `has-identity=<target>`.

The example above results in the following network policy being created: 
```yaml
Name: access-to-web-server
Spec:
  # This label is added to the server by the intents operator
  PodSelector: intents.otterize.com/server=web-server-default-33a0f0
  Allowing ingress traffic:
    To Port: <any> (traffic allowed to all ports)
    From:
      # This label is added to the client by the intents operator
      PodSelector: intents.otterize.com/access-web-server-default-33a0f0=true
  Policy Types: Ingress
```

For more usage example see the [network policy tutorial](https://docs.otterize.com/quick-tutorials/k8s-network-policies).

### Kafka mTLS & ACLs
The intents operator automatically creates, updates, and deletes ACLs in Kafka clusters running within your Kubernetes cluster. 
It works together with SPIRE and the [Otterize SPIRE integration operator](https://github.com/otterize/spire-integration-operator) 
to automatically manage and distribute certificates, easily enabling secure access to Kafka from client pods, all in your Kubernetes cluster.

With Kafka, you can also control access to individual topics, like so:
```yaml
apiVersion: k8s.otterize.com/v1alpha1
kind: ClientIntents
metadata:
  name: kafka-sample
spec:
  service:
    name: my-client
  calls:
    - name: kafka-server
      type: kafka
      topics:
        - name: orders
          operations: [ produce ]
```

Read more about it in the [secure kafka access tutorial](https://docs.otterize.com/quick-tutorials/k8s-kafka-mtls).

### Identities
Pods in the cluster are dynamically labeled with their owner's identity. If a `ReplicaSet` named `myclient` owns 5 pods
and a `Deployment` named `myserver` owns 3 pods, and we enable `myclient -> myserver` access via `ClientIntents`, all 5
source pods would be able to access all 3 target pods.

Pod identities can be overridden using the custom annotation `intents.otterize.com/service-name`
mentioning the desired service name in the value. This is useful, for example, for owner-less pods.


## Bootstrapping
To bootstrap client intents files for the services running in your cluster, you can use the [Otterize network 
mapper](https://github.com/otterize/network-mapper) to automatically detect pod-to-pod calls.

## Read more
The Otterize intents operator is a part of [Otterize OSS](https://otterize.com/open-source) and is an implementation of [intent-based access control](https://otterize.com/ibac).

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
