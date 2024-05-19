#!/bin/bash

if [ -z "$HELM_CHARTS_DIR" ]; then
  HELM_CHARTS_DIR=../../helm-charts
fi

TEMPLATE_DIR=$HELM_CHARTS_DIR/intents-operator/templates
CRD_DIR=$HELM_CHARTS_DIR/intents-operator/crds

# rename ctd files
src_dir=config/crd
src_prefix=k8s.otterize.com_
target_suffix=-customresourcedefinition.yaml
src_suffix=.yaml
for src_file in $(ls -p $src_dir | grep -v / | grep -v kustom | grep -v patched); do
  src_name=$(echo $src_file | sed -e "s/^$src_prefix//" -e "s/$src_suffix//");
  target_file=$(echo $src_name""$target_suffix)
  src_path=$(echo $src_dir"/"$src_file)
  target_path=$(echo $CRD_DIR"/"$target_file)
  cp $src_path $target_path
done
# copy clientIntents CRD
src_name=$(echo k8s.otterize.com_clientintents.yaml | sed -e "s/^$src_prefix//" -e "s/$src_suffix//");
target_file=$(echo $src_name""$target_suffix);
target_path=$(echo $CRD_DIR"/"$target_file);
cp ./config/crd/k8s.otterize.com_clientintents.patched $target_path
cp ./config/crd/k8s.otterize.com_clientintents.patched ./otterizecrds/clientintents-customresourcedefinition.yaml

src_name=$(echo k8s.otterize.com_kafkaserverconfigs.yaml | sed -e "s/^$src_prefix//" -e "s/$src_suffix//");
target_file=$(echo $src_name""$target_suffix);
target_path=$(echo $CRD_DIR"/"$target_file);
cp ./config/crd/k8s.otterize.com_kafkaserverconfigs.patched $target_path
cp ./config/crd/k8s.otterize.com_kafkaserverconfigs.patched ./otterizecrds/kafkaserverconfigs-customresourcedefinition.yaml

src_name=$(echo k8s.otterize.com_protectedservices.yaml | sed -e "s/^$src_prefix//" -e "s/$src_suffix//");
target_file=$(echo $src_name""$target_suffix);
target_path=$(echo $CRD_DIR"/"$target_file);
cp ./config/crd/k8s.otterize.com_protectedservices.patched $target_path
cp ./config/crd/k8s.otterize.com_protectedservices.patched ./otterizecrds/protectedservices-customresourcedefinition.yaml

src_name=$(echo k8s.otterize.com_postgresqlserverconfigs.yaml | sed -e "s/^$src_prefix//" -e "s/$src_suffix//");
target_file=$(echo $src_name""$target_suffix);
target_path=$(echo $CRD_DIR"/"$target_file);
cp ./config/crd/k8s.otterize.com_postgresqlserverconfigs.patched $target_path
cp ./config/crd/k8s.otterize.com_postgresqlserverconfigs.patched ./otterizecrds/postgresqlserverconfigs-customresourcedefinition.yaml


src_name=$(echo k8s.otterize.com_mysqlserverconfigs.yaml | sed -e "s/^$src_prefix//" -e "s/$src_suffix//");
target_file=$(echo $src_name""$target_suffix);
target_path=$(echo $CRD_DIR"/"$target_file);
cp ./config/crd/k8s.otterize.com_mysqlserverconfigs.patched $target_path
cp ./config/crd/k8s.otterize.com_mysqlserverconfigs.patched ./otterizecrds/mysqlserverconfigs-customresourcedefinition.yaml

# copy webhook and cluster role
cp ./config/webhook/manifests-patched $TEMPLATE_DIR"/"otterize-validating-webhook-configuration.yaml
cp ./config/rbac/manifests-patched.yaml $TEMPLATE_DIR"/"intents-operator-manager-clusterrole.yaml
