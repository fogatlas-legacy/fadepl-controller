REGISTRY := your-registry-here
ACCOUNT := fogatlas
REPO := fadepl-controller
TAG := latest
IMAGE := $(ACCOUNT)/$(REPO)


export PROJECT_ROOT=./
export CRD_FOLDER=../crd-client-go

packages:=$(shell go list ./... | grep -v /vendor/)


help:                    ## Show help message
	@printf "PLEASE REMEMBER TO LOGIN INTO YOUR DOCKER REGISTRY BEFORE INVOKING ANY TARGET!!\n\n"
	@printf "Variables you can set are:\n"
	@printf "\tREGISTRY: the registry server url\n"
	@printf "\tACCOUNT: the user/organization to be used\n"
	@printf "\tREPO: the repository/image inside the registry\n"
	@printf "\tTAG: the tag of the image\n"
	@fgrep -h "##" $(MAKEFILE_LIST) | fgrep -v fgrep | sed -e 's/##//'

registry-login:         ## Login to the registry
	docker login $(REGISTRY)

build:                  ## Build the image
	@docker build -f Dockerfile -t $(REGISTRY)/$(IMAGE):$(TAG) .

push:                   ## Push the image
	@docker push $(REGISTRY)/$(IMAGE):$(TAG)

deploy:
	@$(PROJECT_ROOT)/scripts/deploy.sh

clean:
	kubectl delete -f "$(PROJECT_ROOT)/examples/fadepl-silly.yaml"
	kubectl delete -f "$(PROJECT_ROOT)/k8s/foggy.yaml"
	kubectl delete -f "$(CRD_FOLDER)/crd-definitions/fogatlas.fbk.eu_regions.yaml"
	kubectl delete -f "$(CRD_FOLDER)/crd-definitions/fogatlas.fbk.eu_links.yaml"
	kubectl delete -f "$(CRD_FOLDER)/crd-definitions/fogatlas.fbk.eu_externalendpoints.yaml"
	kubectl delete -f "$(CRD_FOLDER)/crd-definitions/fogatlas.fbk.eu_fadepls.yaml"
	kubectl delete namespace fogatlas

unit:
	@ go fmt $(packages)
	@ go vet $(packages)
	@ golint $(packages)
	@ shadow $(packages)
	@ go test -cover $(packages)
