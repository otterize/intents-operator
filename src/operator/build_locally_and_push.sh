#! /usr/bin/env bash
REPO=353146681200.dkr.ecr.us-east-1.amazonaws.com
IMAGE=$REPO/otterize-tools:spire-integration-operator-latest
set -ex
aws ecr get-login-password --region us-east-1 | docker login --username AWS --password-stdin $REPO
cd $(dirname $0)/..
docker build -t $IMAGE -f spire-integration-operator.Dockerfile --secret id=github_token,src=$HOME/.github_token .
docker push $IMAGE
