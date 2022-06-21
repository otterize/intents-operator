# Mutually Authenticated TLS (mTLS) Example

This example is based on [go-spiffe's mTLS example](https://github.com/spiffe/go-spiffe/tree/main/v2/examples/spiffe-tls). 

## Build

1. Login to AWS ECR
```shell
aws ecr-public get-login-password --region us-east-1 | docker login --username AWS --password-stdin public.ecr.aws/e3b4k2v5
```

2. Run the build script
```shell
./build.sh
```

## Deploy
```shell
kubectl apply -f helm/go-spiffe-tls-deployment.yaml
```

