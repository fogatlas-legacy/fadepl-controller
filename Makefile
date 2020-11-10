ACCOUNT := fogatlas
REPO := fadepl-controller
TAG := latest
IMAGE := $(ACCOUNT)/$(REPO)


help:                    ## Show help message
	@printf "PLEASE REMEMBER TO LOGIN INTO YOUR DOCKER REGISTRY BEFORE INVOKING ANY TARGET!!\n\n"
	@printf "Variables you can set are:\n"
	@printf "\tREGISTRY: the registry server url\n"
	@printf "\tACCOUNT: the user/organization to be used\n"
	@printf "\tREPO: the repository/image inside the registry\n"
	@printf "\tTAG: the tag of the image\n"
	@fgrep -h "##" $(MAKEFILE_LIST) | fgrep -v fgrep | sed -e 's/##//'

build:                   ## Build the image
	@docker build -t $(IMAGE):$(TAG) .

push:                    ## Push the image
	@docker push $(IMAGE):$(TAG)

fadepl-controller.yaml:  ## Update the yaml for deploying fadepl-controller
	@echo "Udating k8s/$@"
ifndef REGISTRY
	@sed -e 's=<put here the image url>=$(IMAGE):$(TAG)=g' k8s/$@.template > k8s/$@
else
	@sed -e 's=<put here the image url>=$(REGISTRY)/$(IMAGE):$(TAG)=g' k8s/$@.template > k8s/$@
endif
