# Otterize intents operator

<img title="Otter Manning Helm" src="./otterhelm.png" width=200 />


![build](https://github.com/otterize/intents-operator/actions/workflows/build.yaml/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/otterize/intents-operator/src)](https://goreportcard.com/report/github.com/otterize/intents-operator/src)
[![community](https://img.shields.io/badge/slack-Otterize_Slack-purple.svg?logo=slack)](https://joinslack.otterize.com)

[About](#about) | [Quick tutorial](https://docs.otterize.com/quick-tutorials/k8s-network-policies) | [How does the intents operator work?](#how-does-the-intents-operator-work) | [Contributing](#contributing) | [Slack](#slack)

## About
The Otterize intents operator is an open source Kubernetes operator for easily managing service-to-service authorization by declaring the calls each service needs to make, using [client intents files](https://otterize.com/ibac). The intents operator uses these files to configure network policies, Kafka ACLs, and other enforcement points (in the future) to allow just the intended calls. 

The Otterize intents operator is a part of [Otterize OSS](https://otterize.com/oss) and works within a single Kubernetes cluster.

Here's an example of a CRD-formatted intents file for a service called **"checkoutservice"**, which intends to call **"ecomm-events"** (a Kafka cluster) and **"shippingervice"** (an HTTP server):
```yaml
apiVersion: k8s.otterize.com/v1
kind: ClientIntents
metadata:
  name: checkoutservice
  namespace: production
spec:
  service:
    name: checkoutservice
  calls:
    - name: ecomm-events
      type: kafka
      topics: 
        - name: orders
          operation: produce
    - name: shippingservice
      type: http
      resources:
      - path: /shipments
        methods: [ get, post ]
```
In this example the developers of **"checkoutservice"** chose to declare more granular information about the calls they'll make, allowing tighter enforcement of authorization.

## Try out the intents operator!
Check out [the quick tutorial for using the intents operator to manage network policies](https://docs.otterize.com/quick-tutorials/k8s-network-policies).


Developers create and maintain these intents file for each service alongside its code. The intents are expressed in terms they're used to -- services accessing services, not pod labels and such -- without worrying about how access is controlled. 
With this [intent-based access control](https://otterize.com/ibac) approach, you can implement a zero-trust Kubernetes cluster with minimal developer friction.

To bootstrap client intents files for the services running in your cluster, you can use the [Otterize network mapper](https://github.com/otterize/network-mapper) to automatically detect pod-to-pod calls in your cluster and build a network map. The map can be exported as a set of client intents files. When applied to your cluster (`kubectl apply`), the Otterize intents operator will configure network policies, Kafka ACLs, etc. to authorize just these calls. Developers can then evolve the intents files as their needs evolve, and the authorization will automatically evolve with them, so access is always minimized to what's needed.

If credentials such as X509 certificates are needed for authentication and authorization -- for example, to connect to Kafka with mTLS -- the Otterize intents operator works with SPIRE and the [SPIRE integration operator](https://github.com/otterize/spire-integration-operator) to automatically establish pod service identities, generate trusted credentials for each client service, and deliver them to the pod in a locally-mounted volume.


## How does the intents operator work?

### Network policies
The intents operator automatically creates, updates and deletes network policies, and automatically labels client and server pods, to reflect precisely the client-to-server calls declared in client intents files.

In the example above, the `checkoutservice` intends to call the `shippingservice`. When the CRD is applied through `kubectl apply`, the intents operator labels the `checkoutservice` and `shippingservice` pods, and creates a network policy for the ingress of the `shippingservice` that references these labels and allows calls to the `shippingservice` from the `checkoutservice`.

See [service names and pod labels](#service_names_and_pod_labels) to learn how the right service names are inferred for pods, and how pods are labeled.

### Kafka mTLS & ACLs
The intents operator automatically creates, updates, and deletes ACLs in Kafka clusters running within your Kubernetes cluster. It works together with SPIRE and the [Otterize SPIRE integration operator](https://github.com/otterize/spire-integration-operator) to easily enable secure access to Kafka from client pods, all in your Kubernetes cluster.

The Otterize SPIRE integration operator automatically registers client pods with a SPIRE server, and writes the trusted credentials generated by SPIRE into Kubernetes secrets for use by those pods. The intents operator will in turn reflect the kafka-type intents as Kafka ACLs associated with those pod identities, so client pods get the precise access declared in their intents files.

<!-- For brevity, this README only covers Network Policies internals. [Learn more about using the Intents Operator with Kafka mTLS & ACLs](https://docs.otterize.com/documentation/quick-tutorials/kafka-mtls) -->

### Service names and pod labels
Client intents files use service names to refer to client and server services. How do Otterize operators decide what is the name of the service that runs within the pod? The algorithm is as follows:
1. If the pod has an `intents.otterize.com/service-name` annotation, its value is used as the service name. This allows developers and automations to explicitly name services, if needed.
2. If there is no `intents.otterize.com/service-name` annotation, a recursive look up is performed for the Kubernetes resource owner of the pod, until the root resource is reached, and its name is used as the service name. For example, if you have a `Deployment` named `checkoutservice`, which then creates and owns a `ReplicaSet`, which then creates and owns a `Pod`, then the service name for that pod is `checkoutservice` - same as the name of the `Deployment`. This is intended to capture the likely-more-meaningful "human name" of the service.

Pods are then labeled with values derived from service names. For example, 
the service name is combined with the namespace of the pod and hashed to form the value of the label `intents.otterize.com/server`. This label is then used as a selector for network policies. Another label, `intents.otterize.com/access-server-<servicename>-<servicehash>`, is applied to client pods which have declared their intent to access the server. This label is used as the selector to determine which client pods are allowed to access the server pod.

## Contributing
1. Feel free to fork and open a pull request! Include tests and document your code in [Godoc style](https://go.dev/blog/godoc)
2. In your pull request, please refer to an existing issue or open a new one.
3. See our [Contributor License Agreement](https://github.com/otterize/cla/).

## Slack
[Join the Otterize Slack!](https://joinslack.otterize.com)
