#!/bin/bash

TARGET_DIR=../../helm-charts/intents-operator/templates

# rename ctd files
src_dir=config/crd
src_prefix=k8s.otterize.com_
target_suffix=-customresourcedefinition.yaml
src_suffix=.yaml
for src_file in $(ls -p $src_dir | grep -v /); do
  src_name=$(echo $src_file | sed -e "s/^$src_prefix//" -e "s/$src_suffix//");
  target_file=$(echo $src_name""$target_suffix)
  src_path=$(echo $src_dir"/"$src_file)
  target_path=$(echo $TARGET_DIR"/"$target_file)
  cp $src_path $target_path
done

# copy webhook and cluster role
cp ./config/webhook/manifests-patched $TARGET_DIR"/"ValidatingWebhookConfiguration.yaml
cp ./config/rbac/role.yaml $TARGET_DIR"/"intents-operator-manager-clusterrole.yaml