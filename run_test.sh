#!/bin/bash

set -e  # Exit immediately if a command exits with a non-zero status
set -o pipefail  # Prevent errors in a pipeline from being masked

# Environment variables
REGISTRY="${REGISTRY:-your-registry}"
OPERATOR_IMAGE="${OPERATOR_IMAGE:-your-operator-image}"
OPERATOR_TAG="${OPERATOR_TAG:-your-operator-tag}"

NAMESPACE="otterize-system"
TUTORIAL_NAMESPACE="otterize-tutorial-npol"

install_otterize() {
  echo "Installing Otterize..."

  OPERATOR_FLAGS=""
  TELEMETRY_FLAG="--set global.telemetry.enabled=false"

  # helm dep up ./helm-charts/otterize-kubernetes
  helm install otterize otterize/otterize-kubernetes -n $NAMESPACE --create-namespace $OPERATOR_FLAGS $TELEMETRY_FLAG
}

wait_for_otterize() {
  echo "Waiting for Otterize operator to be ready..."
  kubectl wait pods -n $NAMESPACE -l app=intents-operator --for condition=Ready --timeout=360s

  echo "Waiting for webhook to be ready..."
  POD_IP=$(kubectl get pod -l app=intents-operator -n $NAMESPACE -o=jsonpath='{.items[0].status.podIP}')
  kubectl wait -n $NAMESPACE --for=jsonpath='{.subsets[0].addresses[0].ip}'=$POD_IP endpoints/intents-operator-webhook-service

  echo "Waiting for CRD update..."
  kubectl wait --for=jsonpath='{.spec.conversion.webhook.clientConfig.service.namespace}'=$NAMESPACE customresourcedefinitions/clientintents.k8s.otterize.com
}

apply_intents() {
  echo "Applying intents..."
  source .github/workflows/test-bashrc.sh

  kubectl create namespace $TUTORIAL_NAMESPACE --dry-run=client -o yaml | kubectl apply -f -
  apply_intents_and_wait_for_webhook https://docs.otterize.com/code-examples/automate-network-policies/intents.yaml

  echo "Intents applied."
}

deploy_tutorial_services() {
  echo "Deploying tutorial services..."
  kubectl apply -f https://docs.otterize.com/code-examples/automate-network-policies/all.yaml
}

wait_for_pods() {
  echo "Waiting for tutorial pods to be ready..."
  kubectl wait pods -n $TUTORIAL_NAMESPACE -l app=client --for condition=Ready --timeout=180s
  kubectl wait pods -n $TUTORIAL_NAMESPACE -l app=client-other --for condition=Ready --timeout=180s
  kubectl wait pods -n $TUTORIAL_NAMESPACE -l app=server --for condition=Ready --timeout=180s
}

test_connectivity() {
  echo "Testing connectivity..."

  CLI1_POD=$(kubectl get pod --selector app=client -n $TUTORIAL_NAMESPACE -o json | jq -r ".items[0].metadata.name")
  CLI2_POD=$(kubectl get pod --selector app=client-other -n $TUTORIAL_NAMESPACE -o json | jq -r ".items[0].metadata.name")
  echo "Client: $CLI1_POD, Client-other: $CLI2_POD"

  source .github/workflows/test-bashrc.sh

  for i in {1..10}; do
    if ! kubectl get pod --selector app=client -n $TUTORIAL_NAMESPACE -o json | jq -r ".items[0].metadata.labels" | grep 'access-server'; then
      echo "Waiting for label..."
      sleep 1
    else
      echo "Label found."
      break
    fi
  done

  if ! kubectl get pod --selector app=client -n $TUTORIAL_NAMESPACE -o json | jq -r ".items[0].metadata.labels" | grep 'access-server'; then
    echo "Label not found."
    exit 1
  fi

  echo "Checking client log..."
  wait_for_log "$CLI1_POD" 30 "Hi, I am the server, you called, may I help you?"

  echo "Checking client-other log..."
  wait_for_log "$CLI2_POD" 30 "curl timed out"
}

restart_operator_check() {
  echo "Restarting the operator to check resilience..."

  kubectl delete pods -l app=intents-operator -n $NAMESPACE
  kubectl wait pods -n $NAMESPACE -l app=intents-operator --for condition=Ready --timeout=360s

  POD_IP=$(kubectl get pod -l app=intents-operator -n $NAMESPACE -o=jsonpath='{.items[0].status.podIP}')
  kubectl wait -n $NAMESPACE --for=jsonpath='{.subsets[0].addresses[0].ip}'=$POD_IP endpoints/intents-operator-webhook-service

  echo "Operator restart check completed."
}

# Main execution
install_otterize
wait_for_otterize
apply_intents
deploy_tutorial_services
wait_for_pods
test_connectivity
restart_operator_check

echo "Script execution completed successfully."
