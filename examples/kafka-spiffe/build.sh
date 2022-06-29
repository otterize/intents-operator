# /bin/sh

docker buildx build --platform linux/x86_64 -f ./src/spiffe-java-helper/Dockerfile ./src/ --tag kafka-spiffe-sidecar
docker tag kafka-spiffe-sidecar:latest public.ecr.aws/e3b4k2v5/kafka-spiffe-sidecar:latest
docker push public.ecr.aws/e3b4k2v5/kafka-spiffe-sidecar:latest