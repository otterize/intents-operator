#! /usr/bin/env bash
set -ex
cd $(dirname $0)/..
IMAGE=353146681200.dkr.ecr.us-east-1.amazonaws.com/otterize-tools:spifferize-admission-controller-latest
docker build -t $IMAGE -f spifferize-admission-controller.Dockerfile .
docker push $IMAGE
