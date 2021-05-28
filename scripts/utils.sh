#!/bin/bash

set pipefail -eou

function configure_environment(){
  kubectl create namespace fogatlas
  kubectl apply -f "$PROJECT_ROOT/k8s/registry-credentials.yaml"
  kubectl apply -f "$PROJECT_ROOT/k8s/foggy.yaml"
}
