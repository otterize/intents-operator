# This Dockerfile enables us to push the release-tag into the /version file of the operator, withour rebuilding the SOURCE_IMAGE.
ARG SOURCE_IMAGE

FROM alpine as releaser
ARG VERSION
RUN echo -n $VERSION > ./version

FROM $SOURCE_IMAGE
COPY --from=releaser /version .
