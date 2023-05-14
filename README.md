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
in a Kubernetes cluster using a human-readable format, via a custom resource.

Users declare each client's intents to access specific servers (represented as the kind `ClientIntents`); 
the operator automatically labels the relevant pods accordingly, 
and creates the corresponding network policies and Kafka ACLs.

Here is an example of a `ClientIntents` resource enabling traffic from `my-client` to `web-server` and `kafka-server`:
```yaml
apiVersion: k8s.otterize.com/v1alpha2
kind: ClientIntents
metadata:
  name: intents-sample
spec:
  service:
    name: my-client
  calls:
    - name: web-server
    - name: kafka-server
      type: kafka
```

## How does the intents operator work?

### Network policies
The intents operator automatically creates, updates and deletes network policies, and automatically labels client and server pods, 
to match declarations in client intents files.
The policies created are `Ingress`-based, so source pods are labeled with a `can-access-<target>=true` 
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
It works works with the [credentials operator](https://github.com/otterize/credentials-operator) to automatically:
- Establish pod service identities.
- Generate trusted credentials for each client service.
- Deliver the credentials to the pod's containers within a locally-mounted volume.

With Kafka, you can also control access to individual topics, like so:
```yaml
apiVersion: k8s.otterize.com/v1alpha2
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

Read more about it in the [secure Kafka access tutorial](https://docs.otterize.com/quick-tutorials/k8s-kafka-mtls).

### Istio AuthorizationPolicy
The intents operator automatically creates, updates and deletes Istio authorization policies, automatically looks up service accounts for client pods and labels server pods, to reflect precisely the client-to-server calls declared in client intents files.

The intents operator can also be configured to process client intents *without* creating and managing network policies, to provide visibility on what would happen once enforcement via Istio authorization policy is activated. More information can be found in the [shadow vs active enforcement documentation](/shadow-vs-active-enforcement).

In the example above, the `my-client` service intends to call the `web-server`. When the CRD is applied through `kubectl apply`, the intents operator labels the `web-server` pod, looks up `my-client`'s service account, and creates an authorization policy for the `web-server` that references the service account for `my-client` and allows calls to the `web-server` from the `my-client`.

The intents operator uses the resolved identity as the service name, and combines it with the namespace of the pod and hashed to form the value of the label `intents.otterize.com/server`.
This label is used as a selector for servers in Istio authorization policies. The same algorithm is used to look up the client from the service name in the client intents, for whom the service account is looked up.

Finally, an Istio authorization policy is created that allows communication between the client's service account and the server. If the service account covers clients other than the one requested, an event is generated on the ClientIntents to warn about this, and this appears as a warning on Otterize Cloud.

Read more about it in the [Istio AuthorizationPolicy tutorial](https://docs.otterize.com/quick-tutorials/k8s-istio-authorization-policies).

### Identities
Pods in the cluster are dynamically labeled with their owner's identity. If a `ReplicaSet` named `my-client` owns 5 pods
and a `Deployment` named `my-server` owns 3 pods, and we enable `my-client` &rarr; `my-server` access via `ClientIntents`, all 5
source pods would be able to access all 3 target pods.

Pod identities can be overridden by setting the value of the custom annotation `intents.otterize.com/service-name`
to the desired service name. This is useful, for example, for pods without any owner.


## Bootstrapping
To bootstrap client intents files for the services running in your cluster, you can use the [Otterize network 
mapper](https://github.com/otterize/network-mapper), which automatically detects pod-to-pod calls.

## Read more
The Otterize intents operator is a part of [Otterize OSS](https://otterize.com/open-source) 
and is an implementation of [intent-based access control](https://otterize.com/ibac).

## Development
Run the `make` command inside `src/operator` directory. Some useful commands are:
* `make build` to compile the go code.
* `make deploy` to generate Kubernetes Deployment object which deploys the project to your local cluster.

## Contributing
1. Feel free to fork and open a pull request! Include tests and document your code in [Godoc style](https://go.dev/blog/godoc)
2. In your pull request, please refer to an existing issue or open a new one.
3. Changes to Kubernetes objects will make changes to the Helm chart in the [helm-charts repo](https://github.com/otterize/helm-charts), 
which is a submodule in this repository, so you'll need to open a PR there as well.
4. See our [Contributor License Agreement](https://github.com/otterize/cla/).

## Slack
To join the conversation, ask questions, and engage with other users, join the Otterize Slack!

[![button](https://i.ibb.co/vwRP6xK/Group-3090-2.png)](https://joinslack.otterize.com)

## Usage telemetry
The operator reports anonymous usage information back to the Otterize team, to help the team understand how the software is used in the community and what aspects users find useful. No personal or organizational identifying information is transmitted in these metrics: they only reflect patterns of usage. You may opt out at any time through a single configuration flag.

To **disable** sending usage information:
- Via the Otterize OSS Helm chart: `--set global.telemetry.enabled=false`.
- Via an environment variable: `OTTERIZE_TELEMETRY_ENABLED=false`.
- If running an operator directly: `--telemetry-enabled=false`.

If the `telemetry` flag is omitted or set to `true`, telemetry will be enabled: usage information will be reported.

Read more about it in the [Usage telemetry Documentation](https://docs.otterize.com/otterize-oss/usage-telemetry)
