#!/bin/bash

if [ -z "$HELM_CHARTS_DIR" ]; then
  HELM_CHARTS_DIR=../../helm-charts
fi

TEMPLATE_DIR=$HELM_CHARTS_DIR/intents-operator/templates

# copy clientIntents CRD
cp ./config/crd/k8s.otterize.com_clientintents.patched ./otterizecrds/clientintents-customresourcedefinition.yaml

cp ./config/crd/k8s.otterize.com_kafkaserverconfigs.patched ./otterizecrds/kafkaserverconfigs-customresourcedefinition.yaml

cp ./config/crd/k8s.otterize.com_protectedservices.patched ./otterizecrds/protectedservices-customresourcedefinition.yaml

cp ./config/crd/k8s.otterize.com_postgresqlserverconfigs.patched ./otterizecrds/postgresqlserverconfigs-customresourcedefinition.yaml

cp ./config/crd/k8s.otterize.com_mysqlserverconfigs.patched ./otterizecrds/mysqlserverconfigs-customresourcedefinition.yaml

# copy webhook and cluster role
cp ./config/webhook/manifests-patched $TEMPLATE_DIR"/"otterize-validating-webhook-configuration.yaml
cp ./config/rbac/manifests-patched.yaml $TEMPLATE_DIR"/"intents-operator-manager-clusterrole.yaml
