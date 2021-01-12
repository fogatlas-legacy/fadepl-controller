# FADepl Controller
FADepl controller extends k8s in order to handle geographically distributed clusters where
network infrastructure takes a relevant role. The FADepl controller is based on the k8s
[sample-controller](https://github.com/kubernetes/sample-controller) and is able to handle
the placement of FADepl resources (see below) on a distributed k8s cluster.


## Resources (CRD)

Apart from the usual resources defined in k8s, FADepl controller defines the following CustomResourceDefinition
1. _Region_ as aggregation of computational power. Among other parameters, Regions are characterized by a location.
See [here](./examples/region.yaml) for an
example of Region.
1. _External Endpoint_ intended as Sensors, Cameras, Things or more generally any external endpoint that can
work as source or sink of data. See [here](./examples/externalendpoint.yaml)
for an example of External Endpoint.
1. _Link_ describing the connectivity between different Regions.
See  [here](./examples/link.yaml) for an example of Link.
1. _FADepl_ that is an extended definition of deployment. Its aim is to model an
entire cloud-native application (not just a single microservice/deploment).
See  [here](./examples/fadepl.yaml) for an example of FADepl.

All these types have been defined in the file in the _crd-client-go_ repository.

## Placement algorithm

FADepl controller places FADepl resources on a distributed k8s cluster selecting the
"best" region for that FADepl. Note that FADepl controller stops its placement at region
level avoiding to go at nodes level: the reason is because, once selected the region, the
exact node, inside that region, where a given deployment will be placed, will be selected by
k8s itself.

Currently fadepl controller implements three placement algorithms
1. **Silly**: it assumes that the field _regionrequired_ is defined for each microservice
and just places a microservice following the _regionrequired_ field: no resource required
(cpu, ram, mips, network) are taken into account.
Moreover you can decide to put a given microservice in multiple
regions and to customize the number of replicas and image for each of them. Silly but powerful...;
1. **ResourceBased**: it assumes that the structure of the application to be placed is a linear northbound
(ie from the things to the cloud) chain. Consider it like a _pipeline_.
No need to specify _regionrequired_ with this algorithm but if _regionrequired_ is specified,
it must be just one for each microservice.
This algorithm considers the resources already allocated and computes the placement based
on the available resources (both computational and network ones). Moreover the algorithm
takes into consideration the maximum resource available in a given region (i.e. the maximum
amount of resource a given node can offer in a given region): the algorithm can select a
region only if _maxResource > requestedResource_.
The algorithm find the placement that minimized the usage of the resources.
1. **DAG**: it is an extension of the _ResourceBased_ algorithm able to place applications
that can be represented by a _Directed Acyclic Graph_. We assume that the information/data flows
South - North from the sensors to the cloud. However, an actuation can be modeled considering
a flow from a microservice to an instance of sensor (external endpoint) different from the one
that sends the data. The rules and constraints are the same as the ones already detailed for
the _ResourceBased_: in particular the placement of
a parent microservice depends on the placement of its children and on its own requirements.
The relationship _child - parent_ is determined by the data flow (the source is the child while
the destination is the parent).

The following figure shows a example of application graph that could be placed by the
_DAG_ algorithm.

![application model](./docs/images/dag-application-graph.png)

In a nutshell:
* if no _regionrequired_ => ResourceBased or DAG
* if one _regionrequired_ for each microservice => either ResourceBased or DAG or Silly
* if many _regionrequired_ for each microservice => Silly

New algorithms can be implemented and added to fadepl controller loading them
in the _./fadepl/setup.go_ file (check function _registerAlgorithms_ for more details).
Such algorithms must be developed in golang and must adhere to the
following interface:

```sh
type PlacementAlgorithm interface {
	Init(name string, kubeclientset kubernetes.Interface, faDeplclientset clientset.Interface)
	CalculatePlacement(fadepl *fadeplv1alpha1.FADepl) (err error)
	CalculateUpdate(fadepl *fadeplv1alpha1.FADepl) (err error)
}
```

**Note**: only the _Silly_ algorithm implementation is currently publicly available.

### How different models of CPU are handled

**Assumption:** what follows works in the assumption that each region is homogeneous
in terms of CPU model/architecture and that the algorithm used is DAG (with Silly algorithm it
doesn't work).

Since in fog environment we can have different models of CPU installed, the user has
the possibility to express the amount of CPU in terms of both MIPS or "k8s Quantity".
The second case is the standard one in k8s while the first one can be used in order to
take care of environment having different CPU: the user can request a given amount of MIPS
for her application and the algorithm computes the placement according to the MIPS available
for each region. Once placement is computed, then MIPS are converted in the corresponding
"k8s Quantity" of the region where the microservice will be placed.

Note that if both MIPS and "k8s Quantity" are specified, MIPS value takes the precedence.

### Handling GPU

The support for GPU in k8s is currently experimental and is based on the so-called *Device Plugin*.

Not all the GPU vendors offer an implementation of a device plugin and not for all types of GPUs.
For example NVIDIA offers a device plugin for X86 based GPUs but not for ARM based.

Moreover in case of heterogeneous GPU (i.e. different types of GPU on the same node), it is not possible
to distinguish among the resource requested just looking at the Deployment specification:
```
resources:
	  limits:
	    nvidia.com/gpu: 1
```
Even though device plugin offers a lot of functionality, namely checking the number of GPU,
their health, their occupancy and ensure isolation (only one container can use a given GPU),
for all the above mentioned reasons we think that the best way to proceed is to use
a simple Node Selector - Node Label combination instead of the device plugin one.
So we have to follow these steps:
* label the nodes according to their GPUs:
    ```
    accelerator: <gpu-type>
    ```
* once a FADepl is submitted with:
    ```
    nodeSelector:
      accelerator: <gpu-type>
    ```  
    then the FogAtlas scheduling algorithm (ResourceBased and DAG ones) is executed only among
    those regions that have nodes labeled with that accelerator type.

### Caveats
1. In case of Silly algorithm, no check of resource availability is made: it means that, if a region required is
specified together with a nodeSelector (e.g. for requesting a node with a GPU)
and if the two specification aren't coherent, then the pod will remain in a Pending state forever.
1. In case of ResourceBased and DAG algorithm, if region required is specified together with
a nodeSelector (specifying a GPU), then the algorithm checks if the region has the requested GPU. If no,
then the placement fails.
1. In case of ResourceBased and DAG algorithm, when a microservice require the usage of a GPU,
then the other constraints (CPU, RAM) are checked only against regions that have at least
one GPU.
1. No algorithm checks the occupancy of the GPU resources and doesn't compute those resources
for optimizing the placement: what is checked is only if the resource is present (or not).
1. Only DAG and Silly algorithms can work without dataflows connecting microservices belonging
to the application. ResourceBased is not able to work without the specification of dataflows.  
1. Multiple containers per Pod are allowed but in this case we have to give up to the
features of (i) MIPS specification and (ii) multiple regions required with and override
of the images and/or replicas for each single region (feasible only with the Silly algorithm).
If this is the case, the controller logs a warning and ignores the features.

## What you need to do in order to play with FADepl controller

### Prerequisites

**Note**: FogAtlas has been tested in the following environment:
* Ubuntu 18.04
* Kuberntes v1.15.x (but should work also with v1.19.x)

The following prerequisites apply to the usage of the FADepl controller:
1. Set up a k8s cluster (>= v1.15) with more than one worker
1. Create the CRDs (Regions, Links, ExternalEndpoints, FADepl). Check out the _crd-client-go_
repository and look at the [How to install CRDs](https://github.com/fogatlas/crd-client-go#how-to-install-crds)
section
1. Create requested RBAC on the cluster. You can find an example in
[this file](./k8s/foggy.yaml). Do the following:
   ```sh
   kubectl apply -f k8s/foggy.yaml
   ```
1. Create a docker container image for the FADepl controller (see Dockerfile): the
_makefile_ provided can help doing this (see makefile targets).

### Actions

1. Define the infrastructural topology of your cluster in terms of Regions, Links and
ExternalEndpoints. You can find an example [here](./examples):
   ```sh
   kubectl apply -f examples/region.yaml
   kubectl apply -f examples/link.yaml
   kubectl apply -f examples/externalendpoint.yaml
   ```
1. Label the worker nodes in order to group them into Region, according to the defined topology.
Of course coherence is requested between this labeling and the definition of Regions/Links etc.
Label to be set are:
    * region-name=[region-name]
    * region=[region-id]
    * tier=[tier number]
    ```sh
    #region id, region name and tier can be found in the region.yaml file
    kubectl label nodes <your-node-name> region=<region id>
    kubectl label nodes <your-node-name> region-name=<region name>
    # cloud has always tier=0; first level is tier=1 and so on towards the edge
    kubectl label nodes <your-node-name> tier=<region tier>
    ```
1. Customize the file _./k8s/fadepl-controller.yaml.template_ according to your needs: for example
replace the image with the url of your docker registry (using the makefile you can replace it
with the right syntax and content). Once done change its name to _./k8s/fadepl-controller.yaml_
Beware that if you are using a private docker registry, you need also to create a secret
in k8s and associate it to the service account. Deploy the controller with the following command:
   ```sh
   kubectl apply -f  k8s/fadepl-controller.yaml
   ```
1. Create a FADepl resource in order to deploy an application. You can find an example
[here](./examples/fadepl-silly.yaml) (in the example file we put just two nginx images.
	Change them as you like):
   ```sh
   kubectl apply -f examples/fadepl-silly.yaml
   ```
1. See what happens. You should see something like this where _.reg.003-003_ and
   _.reg.002-002_ are the identifiers of the regions where the deployments/pods have
	 been placed:
   ```sh
   kubectl get fadepls
   ------------------------------------------------------------------------
	 NAME         AGE
	 simple-app   9s
   ------------------------------------------------------------------------

   kubectl get deployments
	 ------------------------------------------------------------------------
	 NAME                    READY   UP-TO-DATE   AVAILABLE   AGE
	 driver.reg.003-003      0/1     1            0           5s
	 processor.reg.002-002   0/1     1            0           5s
	 ------------------------------------------------------------------------

	 kubectl get pods
	 ------------------------------------------------------------------------
	 NAME                                    READY   STATUS    RESTARTS   AGE
	 driver.reg.003-003-6d6d858d87-wrfb6     1/1     Running   0          25s
	 processor.reg.002-002-bfc5c77bd-tbmrp   1/1     Running   0          25s
	 ------------------------------------------------------------------------
   ```
1. Delete the FADepl:
   ```sh
   kubectl delete fadepl <name of the fadepl>
   ```

## License

Copyright 2019 FBK CREATE-NET

Licensed under the Apache License, Version 2.0 (the “License”); you may not use this
file except in compliance with the License. You may obtain a copy of the License
[here](http://www.apache.org/licenses/LICENSE-2.0).

Unless required by applicable law or agreed to in writing, software distributed under
the License is distributed on an “AS IS” BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
either express or implied. See the License for the specific language governing permissions
and limitations under the License.


