# Spifferize
Otterize-spiffe integration

## Admission Controller
### Prerequisites 
```shell
brew install kustomize
```

### Build
```shell
cd src
./operator/build_locally_and_push.sh
```

### Deploy
```shell
cd src
./operator/deploy.sh
```

### Uninstall
```shell
./operator/undeploy.sh
```
