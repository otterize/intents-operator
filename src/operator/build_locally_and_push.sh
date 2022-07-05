#! /usr/bin/env bash
REPO=353146681200.dkr.ecr.us-east-1.amazonaws.com
IMAGE=$REPO/otterize-tools:spifferize-operator-latest
set -ex
aws ecr get-login-password --region us-east-1 | docker login --username AWS --password-stdin $REPO
cd $(dirname $0)/..
docker build -t $IMAGE -f operator.Dockerfile .
docker push $IMAGE
