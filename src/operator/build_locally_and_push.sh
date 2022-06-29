#! /usr/bin/env bash
set -ex
aws ecr get-login-password --region us-east-1 | docker login --username AWS --password-stdin 353146681200.dkr.ecr.us-east-1.amazonaws.com
cd $(dirname $0)/..
IMAGE=353146681200.dkr.ecr.us-east-1.amazonaws.com/otterize-tools:spifferize-operator-latest
docker build -t $IMAGE -f operator.Dockerfile .
docker push $IMAGE
