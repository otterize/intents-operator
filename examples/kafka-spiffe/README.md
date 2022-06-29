# Kafka with MTLS

In this example we have:
- kafka chart (Bitnami) configured to use mtls.
- java-spiffe-helper sidecar that creates keystores from spiffe

## Build

1. Login to AWS ECR
```shell
aws ecr-public get-login-password --region us-east-1 | docker login --username AWS --password-stdin public.ecr.aws/e3b4k2v5
```

2. Run the build script
```shell
./build.sh
```

3. Create spire entry for kafka. (dns is required!)
```shell
kubectl exec -n spire spire-server-0 -- \
    /opt/spire/bin/spire-server entry create \
    -spiffeID spiffe://example.org/ns/default/sa/kafka \
    -parentID spiffe://example.org/ns/spire/sa/spire-agent \
    -selector k8s:pod-image:public.ecr.aws/e3b4k2v5/kafka-spiffe-sidecar:latest -dns "kafka-spiffe-0.kafka-spiffe-headless.default.svc.cluster.local"
  ```

## Deploy
```shell
helm install  kafka-spiffe ./helm
```

