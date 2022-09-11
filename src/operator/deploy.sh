REPO=353146681200.dkr.ecr.us-east-1.amazonaws.com
IMAGE=$REPO/otterize:spire-integration-operator-latest
cd config/manager
kustomize edit set image controller=${IMAGE}
cd -
kustomize build config/default | kubectl apply -f -