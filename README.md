# Spifferize
Otterize-spiffe integration

## Admission Controller
### Build
```shell
cd src
./admission_controller/build_locally_and_push.sh
```

### Deploy
```shell
cd src
helm install spifferize-admission-controller ./admission_controller/helm
```

### Upgrade
```shell
cd src
helm upgrade spifferize-admission-controller ./admission_controller/helm
```