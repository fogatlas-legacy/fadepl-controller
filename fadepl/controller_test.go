/*
Modification:
Copyright 2019 FBK
Slightly modified in order to adapt to the fadepl needs.

Original work:

Copyright 2017 The Kubernetes Authors.

License of both original work and derivative one:
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package fadepl

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	log "github.com/sirupsen/logrus"
	fadeplv1alpha1 "github.com/fogatlas/crd-client-go/pkg/apis/fogatlas/v1alpha1"
	fadeplclientfake "github.com/fogatlas/crd-client-go/pkg/generated/clientset/versioned/fake"
	fadeplinformers "github.com/fogatlas/crd-client-go/pkg/generated/informers/externalversions"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

type fixture struct {
	fadeplClientSet       *fadeplclientfake.Clientset
	kubeClient            *k8sfake.Clientset
	fadeplInformerFactory fadeplinformers.SharedInformerFactory
	kubeInformerFactory   kubeinformers.SharedInformerFactory
	controller            *Controller
}

func initialize(t *testing.T) *fixture {
	f := newFixture()

	//start Informers
	stopCh := make(chan struct{})
	defer close(stopCh)
	f.fadeplInformerFactory.Start(stopCh)
	f.kubeInformerFactory.Start(stopCh)

	// create the Infrastructure
	setupInfra(f, t)
	log.SetLevel(log.ErrorLevel)
	log.SetFormatter(&log.TextFormatter{
		DisableColors: false,
		FullTimestamp: true,
	})
	return f
}

func newFixture() *fixture {
	f := &fixture{}
	objects := []runtime.Object{}
	f.fadeplClientSet = fadeplclientfake.NewSimpleClientset(objects...)
	f.kubeClient = k8sfake.NewSimpleClientset(objects...)
	f.fadeplInformerFactory = fadeplinformers.NewSharedInformerFactory(f.fadeplClientSet, time.Second*0)
	f.kubeInformerFactory = kubeinformers.NewSharedInformerFactory(f.kubeClient, time.Second*0)

	f.controller = NewController(f.kubeClient, f.fadeplClientSet, f.kubeInformerFactory.Apps().V1().Deployments(),
		f.fadeplInformerFactory.Fogatlas().V1alpha1().FADepls())
	registerAlgorithms(f.controller, f.kubeClient, f.fadeplClientSet)
	return f
}

func setupInfra(f *fixture, t *testing.T) {
	regions := []*fadeplv1alpha1.Region{
		setUpRegion("cloud", "001-001", 0, "Intel i7", 50000),
		setUpRegion("trento", "002-002", 1, "Intel i5", 30000),
		setUpRegion("povo", "003-003", 2, "Intel Atom", 5000),
	}

	links := []*fadeplv1alpha1.Link{
		setUpLink("link1", "link1", "003-003", "001-001", resource.MustParse("10M"), resource.MustParse("100")),
		setUpLink("link2", "link2", "002-002", "001-001", resource.MustParse("10M"), resource.MustParse("100")),
		setUpLink("link3", "link3", "003-003", "002-002", resource.MustParse("100M"), resource.MustParse("10")),
	}

	ees := []*fadeplv1alpha1.ExternalEndpoint{
		setUpExternalEndpoint("cam1", "cam1", "camera", "10.10.10.10", "003-003"),
		setUpExternalEndpoint("cam2", "cam2", "camera", "11.11.11.11", "003-003"),
		setUpExternalEndpoint("sens1", "sens1", "sensor", "12.12.12.12", "001-001"),
		setUpExternalEndpoint("broker1", "broker1", "service", "13.13.13.13", "002-002"),
	}

	nodes := []*v1.Node{
		setUpNode("master", "1200m", "1900Mi", "cloud", "001-001"),
		setUpNode("worker1", "2", "2Gi", "trento", "002-002"),
		setUpNode("worker2", "2", "2Gi", "povo", "003-003"),
	}

	for _, r := range regions {
		_, err := f.fadeplClientSet.FogatlasV1alpha1().Regions("default").Create(context.TODO(), r, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
	}

	for _, l := range links {
		_, err := f.fadeplClientSet.FogatlasV1alpha1().Links("default").Create(context.TODO(), l, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
	}

	for _, e := range ees {
		_, err := f.fadeplClientSet.FogatlasV1alpha1().ExternalEndpoints("default").Create(context.TODO(), e, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
	}

	for _, n := range nodes {
		_, err := f.kubeClient.CoreV1().Nodes().Create(context.TODO(), n, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func setUpNode(name string, cpu string, memory string, regionName string, regionID string) *v1.Node {
	resourceList := make(map[v1.ResourceName]resource.Quantity)
	resourceList[v1.ResourceCPU] = resource.MustParse(cpu)
	resourceList[v1.ResourceMemory] = resource.MustParse(memory)
	labels := make(map[string]string)
	labels["region"] = regionID
	labels["region-name"] = regionName
	return &v1.Node{
		TypeMeta: metav1.TypeMeta{APIVersion: v1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Status: v1.NodeStatus{
			Allocatable: resourceList,
		},
	}
}

func setUpRegion(name string, id string, tier int32, cpuModel string, cpu2mips int64) *fadeplv1alpha1.Region {
	return &fadeplv1alpha1.Region{
		TypeMeta: metav1.TypeMeta{APIVersion: fadeplv1alpha1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
		Spec: fadeplv1alpha1.RegionSpec{
			ID:       id,
			Name:     name,
			Tier:     tier,
			Location: "a location",
			Type:     "nodes",
			CPUModel: cpuModel,
			CPU2MIPS: cpu2mips,
		},
	}
}

func setUpLink(name string, id string, endpointA string, endpointB string, bw resource.Quantity,
	latency resource.Quantity) *fadeplv1alpha1.Link {
	return &fadeplv1alpha1.Link{
		TypeMeta: metav1.TypeMeta{APIVersion: fadeplv1alpha1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
		Spec: fadeplv1alpha1.LinkSpec{
			ID:        id,
			EndpointA: endpointA,
			EndpointB: endpointB,
			Bandwidth: bw,
			Latency:   latency,
			Status:    "up",
		},
	}
}

func setUpExternalEndpoint(name string, id string, eeType string, ip string, regionID string) *fadeplv1alpha1.ExternalEndpoint {
	return &fadeplv1alpha1.ExternalEndpoint{
		TypeMeta: metav1.TypeMeta{APIVersion: fadeplv1alpha1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
		Spec: fadeplv1alpha1.ExternalEndpointSpec{
			ID:        id,
			Name:      name,
			Type:      eeType,
			IPAddress: ip,
			Location:  "a location",
			RegionID:  regionID,
		},
	}
}

func cleanUp(f *fixture, t *testing.T, fadepl *fadeplv1alpha1.FADepl) {

	err := f.fadeplClientSet.FogatlasV1alpha1().FADepls(fadepl.Namespace).Delete(context.TODO(), fadepl.Name, metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("Error deleting FADepl (%s)", err)
	}
	label := "fadepl=" + fadepl.Name
	deployments, err := f.kubeClient.AppsV1().Deployments(fadepl.Namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: label})
	if err != nil {
		t.Fatalf("Error retrieving Deployments (%s)", err)
	}
	for _, d := range deployments.Items {
		err = f.kubeClient.AppsV1().Deployments(fadepl.Namespace).Delete(context.TODO(), d.Name, metav1.DeleteOptions{})
		if err != nil {
			t.Fatalf("Error deleting Deployments (%s)", err)
		}
	}
}

//
// newDeployment function
//
func newDeployment(name string, replicas *int32, cpuRequested string, memoryRequested string) apps.Deployment {
	selector := make(map[string]string)
	selector["name"] = name
	resourceList := make(map[v1.ResourceName]resource.Quantity)
	resourceList[v1.ResourceCPU] = resource.MustParse(cpuRequested)
	resourceList[v1.ResourceMemory] = resource.MustParse(memoryRequested)
	d := apps.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   metav1.NamespaceDefault,
			Annotations: make(map[string]string),
		},
		Spec: apps.DeploymentSpec{
			Replicas: replicas,
			Selector: &metav1.LabelSelector{MatchLabels: selector},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: selector,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  name,
							Image: "nginx",
							Resources: v1.ResourceRequirements{
								Requests: resourceList,
							},
						},
					},
				},
			},
		},
	}
	return d
}

//
// newFADeplSilly function
//
func newFADeplSilly(name string, replicas *int32) *fadeplv1alpha1.FADepl {
	return &fadeplv1alpha1.FADepl{
		TypeMeta: metav1.TypeMeta{APIVersion: fadeplv1alpha1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Finalizers: []string{"finalizer.fogatlas.fbk.eu"},
			Namespace:  metav1.NamespaceDefault,
		},
		Spec: fadeplv1alpha1.FADeplSpec{
			ExternalEndpoints: []string{"cam1"},
			Algorithm:         "Silly",
			Microservices: []*fadeplv1alpha1.FADeplMicroservice{
				{
					Name: "driver",
					Regions: []*fadeplv1alpha1.FARegion{
						{
							RegionRequired: "003-003",
						},
					},
					Deployment: newDeployment("driver", replicas, "100m", "100M"),
				},
				{
					Name: "processor",
					Regions: []*fadeplv1alpha1.FARegion{
						{
							RegionRequired: "002-002",
						},
					},
					Deployment: newDeployment("processor", replicas, "100m", "400M"),
				},
			},
			DataFlows: []*fadeplv1alpha1.FADeplDataFlow{
				{
					BandwidthRequired: resource.MustParse("5M"),
					LatencyRequired:   resource.MustParse("20"),
					SourceID:          "cam1",
					DestinationID:     "driver",
				},
				{
					BandwidthRequired: resource.MustParse("100k"),
					LatencyRequired:   resource.MustParse("500"),
					SourceID:          "driver",
					DestinationID:     "processor",
				},
			},
		},
	}
}

//
// expectedPlacementSilly function
//
func expectedPlacementSilly() fadeplv1alpha1.FADeplStatus {
	return fadeplv1alpha1.FADeplStatus{
		Placements: []*fadeplv1alpha1.FAPlacement{
			{
				Regions: []*fadeplv1alpha1.FARegion{
					{
						RegionRequired: "003-003",
						RegionSelected: "003-003",
					},
				},
				Microservice: "driver",
			},
			{
				Regions: []*fadeplv1alpha1.FARegion{
					{
						RegionRequired: "002-002",
						RegionSelected: "002-002",
					},
				},
				Microservice: "processor",
			},
		},
		CurrentStatus: 1,
	}
}

//
// newFADeplDAG function
//
func newFADeplDAG(name string, replicas *int32) *fadeplv1alpha1.FADepl {
	return &fadeplv1alpha1.FADepl{
		TypeMeta: metav1.TypeMeta{APIVersion: fadeplv1alpha1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Finalizers: []string{"finalizer.fogatlas.fbk.eu"},
			Namespace:  metav1.NamespaceDefault,
		},
		Spec: fadeplv1alpha1.FADeplSpec{
			ExternalEndpoints: []string{"cam1", "cam2", "broker1", "sens1"},
			Algorithm:         "DAG",
			Microservices: []*fadeplv1alpha1.FADeplMicroservice{
				{
					Name:       "processor",
					Deployment: newDeployment("processor", replicas, "100m", "100M"),
				},
				{
					Name: "driver1",
					Regions: []*fadeplv1alpha1.FARegion{
						{
							RegionRequired: "003-003",
						},
					},
					Deployment: newDeployment("driver1", replicas, "100m", "100M"),
				},
				{
					Name: "driver2",
					Regions: []*fadeplv1alpha1.FARegion{
						{
							RegionRequired: "003-003",
						},
					},
					Deployment: newDeployment("driver2", replicas, "100m", "100M"),
				},
				{
					Name:       "driver3",
					Deployment: newDeployment("driver3", replicas, "100m", "100M"),
				},
				{
					Name:       "analytics",
					Deployment: newDeployment("analytics", replicas, "200m", "200M"),
				},
			},
			DataFlows: []*fadeplv1alpha1.FADeplDataFlow{
				{
					BandwidthRequired: resource.MustParse("5M"),
					LatencyRequired:   resource.MustParse("20"),
					SourceID:          "cam1",
					DestinationID:     "driver1",
				},
				{
					BandwidthRequired: resource.MustParse("5M"),
					LatencyRequired:   resource.MustParse("20"),
					SourceID:          "cam2",
					DestinationID:     "driver2",
				},
				{
					BandwidthRequired: resource.MustParse("5M"),
					LatencyRequired:   resource.MustParse("20"),
					SourceID:          "broker1",
					DestinationID:     "driver3",
				},
				{
					BandwidthRequired: resource.MustParse("100k"),
					LatencyRequired:   resource.MustParse("500"),
					SourceID:          "driver1",
					DestinationID:     "processor",
				},
				{
					BandwidthRequired: resource.MustParse("100k"),
					LatencyRequired:   resource.MustParse("500"),
					SourceID:          "driver2",
					DestinationID:     "processor",
				},
				{
					BandwidthRequired: resource.MustParse("100k"),
					LatencyRequired:   resource.MustParse("500"),
					SourceID:          "driver3",
					DestinationID:     "processor",
				},
				{
					BandwidthRequired: resource.MustParse("100k"),
					LatencyRequired:   resource.MustParse("500"),
					SourceID:          "processor",
					DestinationID:     "analytics",
				},
				{
					BandwidthRequired: resource.MustParse("100k"),
					LatencyRequired:   resource.MustParse("500"),
					SourceID:          "analytics",
					DestinationID:     "sens1",
				},
			},
		},
	}
}

//
// expectedPlacementDAG function
//
func expectedPlacementDAG() fadeplv1alpha1.FADeplStatus {
	return fadeplv1alpha1.FADeplStatus{
		Placements: []*fadeplv1alpha1.FAPlacement{
			{
				Regions: []*fadeplv1alpha1.FARegion{
					{
						RegionRequired: "003-003",
						RegionSelected: "003-003",
						CPU2MIPSMilli:  5,
					},
				},
				Microservice: "driver1",
			},
			{
				Regions: []*fadeplv1alpha1.FARegion{
					{
						RegionRequired: "003-003",
						RegionSelected: "003-003",
						CPU2MIPSMilli:  5,
					},
				},
				Microservice: "driver2",
			},
			{
				Regions: []*fadeplv1alpha1.FARegion{
					{
						RegionSelected: "002-002",
						CPU2MIPSMilli:  30,
					},
				},
				Microservice: "driver3",
			},
			{
				Regions: []*fadeplv1alpha1.FARegion{
					{
						RegionSelected: "002-002",
						CPU2MIPSMilli:  30,
					},
				},
				Microservice: "processor",
			},
			{
				Regions: []*fadeplv1alpha1.FARegion{
					{
						RegionSelected: "002-002",
						CPU2MIPSMilli:  30,
					},
				},
				Microservice: "analytics",
			},
		},
		LinksOccupancy: []*fadeplv1alpha1.FALinkOccupancy{
			{
				LinkID:          "link3",
				BwAllocated:     resource.MustParse("200k"),
				PrevBwAllocated: resource.MustParse("0"),
				IsChanged:       false,
			},
		},
		CurrentStatus: 1,
	}
}

//
// newFADeplDAGLeaf function: note that driver2 is a leaf
//
func newFADeplDAGLeaf(name string, replicas *int32) *fadeplv1alpha1.FADepl {
	return &fadeplv1alpha1.FADepl{
		TypeMeta: metav1.TypeMeta{APIVersion: fadeplv1alpha1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Finalizers: []string{"finalizer.fogatlas.fbk.eu"},
			Namespace:  metav1.NamespaceDefault,
		},
		Spec: fadeplv1alpha1.FADeplSpec{
			ExternalEndpoints: []string{"cam1", "cam2", "broker1", "sens1"},
			Algorithm:         "DAG",
			Microservices: []*fadeplv1alpha1.FADeplMicroservice{
				{
					Name:       "processor",
					Deployment: newDeployment("processor", replicas, "100m", "100M"),
				},
				{
					Name: "driver1",
					Regions: []*fadeplv1alpha1.FARegion{
						{
							RegionRequired: "003-003",
						},
					},
					Deployment: newDeployment("driver1", replicas, "100m", "100M"),
				},
				{
					Name: "driver2",
					Regions: []*fadeplv1alpha1.FARegion{
						{
							RegionRequired: "003-003",
						},
					},
					Deployment: newDeployment("driver2", replicas, "100m", "100M"),
				},
				{
					Name:       "driver3",
					Deployment: newDeployment("driver3", replicas, "100m", "100M"),
				},
				{
					Name:       "analytics",
					Deployment: newDeployment("analytics", replicas, "200m", "200M"),
				},
			},
			DataFlows: []*fadeplv1alpha1.FADeplDataFlow{
				{
					BandwidthRequired: resource.MustParse("5M"),
					LatencyRequired:   resource.MustParse("20"),
					SourceID:          "cam1",
					DestinationID:     "driver1",
				},
				{
					BandwidthRequired: resource.MustParse("5M"),
					LatencyRequired:   resource.MustParse("20"),
					SourceID:          "broker1",
					DestinationID:     "driver3",
				},
				{
					BandwidthRequired: resource.MustParse("100k"),
					LatencyRequired:   resource.MustParse("500"),
					SourceID:          "driver1",
					DestinationID:     "processor",
				},
				{
					BandwidthRequired: resource.MustParse("100k"),
					LatencyRequired:   resource.MustParse("500"),
					SourceID:          "driver2",
					DestinationID:     "processor",
				},
				{
					BandwidthRequired: resource.MustParse("100k"),
					LatencyRequired:   resource.MustParse("500"),
					SourceID:          "driver3",
					DestinationID:     "processor",
				},
				{
					BandwidthRequired: resource.MustParse("100k"),
					LatencyRequired:   resource.MustParse("500"),
					SourceID:          "processor",
					DestinationID:     "analytics",
				},
				{
					BandwidthRequired: resource.MustParse("100k"),
					LatencyRequired:   resource.MustParse("500"),
					SourceID:          "analytics",
					DestinationID:     "sens1",
				},
			},
		},
	}
}

//
// expectedPlacementDAG function
//
func expectedPlacementDAGLeaf() fadeplv1alpha1.FADeplStatus {
	return fadeplv1alpha1.FADeplStatus{
		Placements: []*fadeplv1alpha1.FAPlacement{
			{
				Regions: []*fadeplv1alpha1.FARegion{
					{
						RegionRequired: "003-003",
						RegionSelected: "003-003",
						CPU2MIPSMilli:  5,
					},
				},
				Microservice: "driver1",
			},
			{
				Regions: []*fadeplv1alpha1.FARegion{
					{
						RegionRequired: "003-003",
						RegionSelected: "003-003",
						CPU2MIPSMilli:  5,
					},
				},
				Microservice: "driver2",
			},
			{
				Regions: []*fadeplv1alpha1.FARegion{
					{
						RegionSelected: "002-002",
						CPU2MIPSMilli:  30,
					},
				},
				Microservice: "driver3",
			},
			{
				Regions: []*fadeplv1alpha1.FARegion{
					{
						RegionSelected: "002-002",
						CPU2MIPSMilli:  30,
					},
				},
				Microservice: "processor",
			},
			{
				Regions: []*fadeplv1alpha1.FARegion{
					{
						RegionSelected: "002-002",
						CPU2MIPSMilli:  30,
					},
				},
				Microservice: "analytics",
			},
		},
		LinksOccupancy: []*fadeplv1alpha1.FALinkOccupancy{
			{
				LinkID:          "link3",
				BwAllocated:     resource.MustParse("200k"),
				PrevBwAllocated: resource.MustParse("0"),
				IsChanged:       false,
			},
		},
		CurrentStatus: 1,
	}
}

//
// newFADeplHeteroCPU function
//
func newFADeplHeteroCPU(name string, replicas *int32) *fadeplv1alpha1.FADepl {
	return &fadeplv1alpha1.FADepl{
		TypeMeta: metav1.TypeMeta{APIVersion: fadeplv1alpha1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Finalizers: []string{"finalizer.fogatlas.fbk.eu"},
			Namespace:  metav1.NamespaceDefault,
		},
		Spec: fadeplv1alpha1.FADeplSpec{
			ExternalEndpoints: []string{"cam1"},
			Algorithm:         "DAG",
			Microservices: []*fadeplv1alpha1.FADeplMicroservice{
				{
					Name: "driver-mips",
					Regions: []*fadeplv1alpha1.FARegion{
						{
							RegionRequired: "003-003",
						},
					},
					Deployment: newDeployment("driver-mips", replicas, "100m", "100M"),
				},
				{
					Name:         "processor-mips",
					MIPSRequired: resource.MustParse("2000"),
					Deployment:   newDeployment("processor-mips", replicas, "100m", "100M"),
				},
			},
			DataFlows: []*fadeplv1alpha1.FADeplDataFlow{
				{
					BandwidthRequired: resource.MustParse("5M"),
					LatencyRequired:   resource.MustParse("20"),
					SourceID:          "cam1",
					DestinationID:     "driver-mips",
				},
				{
					BandwidthRequired: resource.MustParse("1M"),
					LatencyRequired:   resource.MustParse("500"),
					SourceID:          "driver-mips",
					DestinationID:     "processor-mips",
				},
			},
		},
	}
}

//
// expectedPlacementHeteroCPU function
//
func expectedPlacementHeteroCPU() fadeplv1alpha1.FADeplStatus {
	return fadeplv1alpha1.FADeplStatus{
		Placements: []*fadeplv1alpha1.FAPlacement{
			{
				Regions: []*fadeplv1alpha1.FARegion{
					{
						RegionRequired: "003-003",
						RegionSelected: "003-003",
						CPU2MIPSMilli:  5,
					},
				},
				Microservice: "driver-mips",
			},
			{
				Regions: []*fadeplv1alpha1.FARegion{
					{
						RegionSelected: "002-002",
						CPU2MIPSMilli:  30,
					},
				},
				Microservice: "processor-mips",
			},
		},
		LinksOccupancy: []*fadeplv1alpha1.FALinkOccupancy{
			{
				LinkID:      "link3",
				BwAllocated: resource.MustParse("1M"),
				IsChanged:   false,
			},
		},
		CurrentStatus: 1,
	}
}

//
// int32Ptr function
//
func int32Ptr(i int32) *int32 { return &i }

func getKey(fadepl *fadeplv1alpha1.FADepl) (string, error) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(fadepl)
	if err != nil {
		return "", err
	}
	return key, nil
}

// TestEnd2End tests the entire lifecycle of a FADepl from its submission,
// to the execution of the placement algorithm and finally to the creation of the
// Deployments.
func TestE2E(t *testing.T) {
	f := initialize(t)

	var tests = []struct {
		testName string
		fadepl   *fadeplv1alpha1.FADepl
		expected fadeplv1alpha1.FADeplStatus
	}{
		{
			testName: "PlacementSilly",
			fadepl:   newFADeplSilly("silly", int32Ptr(1)),
			expected: expectedPlacementSilly(),
		},
		{
			testName: "PlacememntDAG",
			fadepl:   newFADeplDAG("dag", int32Ptr(1)),
			expected: expectedPlacementDAG(),
		},
		{
			testName: "PlacememntDAGLeaf",
			fadepl:   newFADeplDAGLeaf("dagleaf", int32Ptr(1)),
			expected: expectedPlacementDAGLeaf(),
		},
		{
			testName: "PlacememntHeteroCPU",
			fadepl:   newFADeplHeteroCPU("dagheterocpu", int32Ptr(1)),
			expected: expectedPlacementHeteroCPU(),
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			// Need to add/create the fadepl to both the Informer and using the clientSet.
			// Otherwise either the subsequent Get or the Update don't work.
			f.fadeplInformerFactory.Fogatlas().V1alpha1().FADepls().Informer().GetIndexer().Add(test.fadepl)
			_, err := f.fadeplClientSet.FogatlasV1alpha1().FADepls(metav1.NamespaceDefault).Create(context.TODO(), test.fadepl, metav1.CreateOptions{})
			if err != nil {
				t.Error(err)
			} else {
				key, err := getKey(test.fadepl)
				if err != nil {
					t.Errorf("Unexpected error getting key for fadepl: %v", err)
				} else {
					f.controller.syncHandler(key)
					actualFadepl, err := f.fadeplClientSet.FogatlasV1alpha1().FADepls(test.fadepl.Namespace).Get(context.TODO(), test.fadepl.Name, metav1.GetOptions{})
					if err != nil {
						t.Error(err)
					} else {
						valid := cmp.Equal(actualFadepl.Status, test.expected, nil)
						if !valid {
							diff := cmp.Diff(actualFadepl.Status, test.expected, nil)
							t.Errorf("Expected and actual differ: %s", diff)
						}
					}
				}
			}
			cleanUp(f, t, test.fadepl)
		})
	}
}
