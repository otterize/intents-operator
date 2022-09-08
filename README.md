# Otterize Intents Operator

![Otter Manning Helm](./otterhelm.png)


![build](https://img.shields.io/static/v1?label=build&message=passing&color=success)
![go report](https://img.shields.io/static/v1?label=go%20report&message=A%2B&color=success)
[![GoDoc reference example](https://img.shields.io/badge/godoc-reference-blue.svg)](https://godoc.org/nanomsg.org/go/mangos/v2)
![openssf](https://img.shields.io/static/v1?label=openssf%20best%20practices&message=passing&color=success)
![community](https://img.shields.io/badge/slack-Otterize_Slack-orange.svg?logo=slack)

[About](#about) | [Quickstart](https://docs.otterize.com/documentation/quick-tutorials/network-mapper) | [How does the Intents Operator work?](#how-does-the-intents-operator-work) | [Contributing](#contributing) | [Slack](#slack)

## About
The Otterize Intents Operator is an open source Kubernetes operator that enables you to declaratively manage service-to-service authorization using Network Policies and Kafka ACLs, and enables you to achieve zero-trust in your cluster while significantly reducing developer friction.

Developers declaratively specify which other services their service needs to access, in the terms and abstraction they're used to (so, services accessing services, rather than pod labels, etc.), and the Intents Operator applies Network Policies, pod labels and Kafka ACLs, depending on how you've deployed it, to enable the required access.

Installed together with the [Otterize Network Mapper](https://github.com/otterize/network-mapper) and SPIRE, you can automatically detect existing traffic between pods in your cluster, and generate ClientIntents (or manually author), a Kubernetes resource, that describes these communications. The Intents Operator then enforces the intents using Network Policies and Kafka ACLs.

Only ClientIntents need to be created - other resources such as credentials, network policies and labels are automatically created.


## How does the Intents Operator work?

### Network Policies
The Intents Operator facilitiates Kubernetes NetworkPolicy creation by creating appropriate network policies, as well as labeling pods, to allow access to services according to the ClientIntents that are declared.

This intent results in the creation of a NetworkPolicy for the service `server`, as well as labels the pods for the `client` and `server` services, so that an ingress network policy applies to the server and allows the client pods to connect. See [Service name resolution](#Service_name_resolution) to learn how service names are resolved.
```
    apiVersion: k8s.otterize.com/v1alpha1
    kind: ClientIntents
    metadata:
      name: client
    spec:
      service:
        name: client
      calls:
        - name: server
          type: HTTP
```

### Kafka mTLS & ACLs
The Intents Operator can manage Kafka ACLs for pods running in your Kubernetes cluster. This works best when integrated with SPIRE and the [SPIRE Integration Operator](https://github.com/otterize/spire-integration-operator). The SPIRE Integration Operator registers workloads with a bundled SPIRE server and writes credentials into secrets for use by your pods. The Intents Operator is then able to configure ACLs on a Kafka cluster with those workload identities so that your pods can access the Kafka cluster, or specific topics.

For brevity, this README only covers Network Policies internals. [Learn more about using the Intents Operator with Kafka mTLS & ACLs](https://docs.otterize.com/documentation/quick-tutorials/kafka-mtls)

### Service name resolution and automatic pod labeling
Service name resolution is performed one of two ways:
1. If an `otterize/service-name` label is present, that name is used.
2. If not, a recursive look up is performed for the Kubernetes resource owner for a Pod until the root is reached. For example, if you have a `Deployment` named `client`, which then creates and owns a `ReplicaSet`, which then creates and owns a `Pod`, then the service name for that pod is `client` - same as the name of the `Deployment`.

The value resulting from this process is then combined with the namespace of the pod, and hashed together to form the value of the label `otterize/server`. The `otterize/server` label is then used as a selector for network policies.

Another, similar label - `otterize/access-server-<servicename>-<servicehash>`, is applied to client pods which have declared their ClientIntent to access the server. This label is used as the selector for which client pods are allowed to access the server pods.

## Contributing
1. Feel free to fork and open a pull request! Include tests and document your code in [Godoc style](https://go.dev/blog/godoc)
2. In your pull request, please refer to an existing issue or open a new one.

## Slack
[Join the Otterize Slack!](https://join.slack.com/t/otterizeworkspace/shared_invite/zt-1fnbnl1lf-ub6wler4QrW6ZzIn2U9x1A)
