# /bin/sh
REPO=353146681200.dkr.ecr.us-east-1.amazonaws.com
set -ex
aws ecr get-login-password --region us-east-1 | docker login --username AWS --password-stdin $REPO

IMAGE=$REPO/otterize:kafka-mtls-client-latest
docker buildx build --platform linux/amd64 -f ./src/Dockerfile ./src/ -t $IMAGE
docker push $IMAGE