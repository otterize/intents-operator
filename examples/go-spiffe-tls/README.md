# Mutually Authenticated TLS (mTLS) Example

This example is based on [go-spiffe's mTLS example](https://github.com/spiffe/go-spiffe/tree/main/v2/examples/spiffe-tls).

## Build
Run the build script
```shell
./build.sh
```

## Deploy
```shell
kubectl apply -f helm/go-spiffe-tls-deployment.yaml
```