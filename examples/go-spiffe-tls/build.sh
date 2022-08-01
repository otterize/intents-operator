# /bin/sh
REPO=353146681200.dkr.ecr.us-east-1.amazonaws.com
set -ex
aws ecr get-login-password --region us-east-1 | docker login --username AWS --password-stdin $REPO

IMAGE=$REPO/otterize-tools:go-spiffe-server-latest
docker buildx build --platform linux/amd64 -f ./src/server.Dockerfile ./src/ -t $IMAGE
docker push $IMAGE


IMAGE=$REPO/otterize-tools:go-spiffe-client-latest
docker buildx build --platform linux/amd64 -f ./src/client.Dockerfile ./src/ -t $IMAGE
docker push $IMAGE