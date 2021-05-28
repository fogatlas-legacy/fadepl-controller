#!/bin/bash

set pipefail -eou

source "$PROJECT_ROOT/scripts/utils.sh"

#minikube start

configure_environment

# Install CRDs
kubectl apply -f "$CRD_FOLDER/crd-definitions/fogatlas.fbk.eu_regions.yaml"
kubectl apply -f "$CRD_FOLDER/crd-definitions/fogatlas.fbk.eu_links.yaml"
kubectl apply -f "$CRD_FOLDER/crd-definitions/fogatlas.fbk.eu_externalendpoints.yaml"
kubectl apply -f "$CRD_FOLDER/crd-definitions/fogatlas.fbk.eu_fadepls.yaml"

# Install the topology
kubectl apply -f "$PROJECT_ROOT/examples/region.yaml"
kubectl apply -f "$PROJECT_ROOT/examples/link.yaml"
kubectl apply -f "$PROJECT_ROOT/examples/externalendpoint.yaml"

# Label the nodes in order to group them in regions, according to the defined topology.
# region id, region name and tier can be found in the region.yaml file
# cloud has always tier=0; first level is tier=1 and so on towards the edge
kubectl label nodes master region=001-001
kubectl label nodes master region-name=cloud
kubectl label nodes master tier=0
kubectl label nodes worker1 region=002-002
kubectl label nodes worker1 region-name=trento
kubectl label nodes worker1 tier=1
kubectl label nodes worker2 region=003-003
kubectl label nodes worker2 region-name=povo
kubectl label nodes worker2 tier=2

# deploying fadepl-controller
kubectl apply -f "$PROJECT_ROOT/k8s/fadepl-controller.yaml"
while [[ $(kubectl get pods -n fogatlas -l app=fadepl -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do
  echo "waiting for fadepl-controller" && sleep 5
done

# Deploy a fadepl
kubectl apply -f "$PROJECT_ROOT/examples/fadepl-silly.yaml"
