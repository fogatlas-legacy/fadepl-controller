package silly

import (
	"encoding/json"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	log "github.com/sirupsen/logrus"
	fadeplv1alpha1 "github.com/fogatlas/crd-client-go/pkg/apis/fogatlas/v1alpha1"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Silly", func() {
	var sillyAlgo SillyAlgorithm
	BeforeEach(func() {
		log.SetLevel(log.ErrorLevel)
		sillyAlgo.Init("Silly", nil, nil)
	})

	Describe("Placing a FADepl using Silly algorithm", func() {
		var fadepl *fadeplv1alpha1.FADepl
		Context("with 2 microservices and one external endpoint", func() {
			BeforeEach(func() {
				fadepl = newFADeplSilly("test-silly", int32Ptr(1))
			})
			It("should have FADeplStatus equal to the expected one ", func() {
				err := sillyAlgo.CalculatePlacement(fadepl)
				Expect(err).NotTo(HaveOccurred())
				status, err := json.Marshal(fadepl.Status)
				Expect(err).NotTo(HaveOccurred())
				statusStr := string(status)
				Expect(statusStr).To(Equal(expectedPlacementSilly()))
			})
		})
	})
})

//
// newDeployment function
//
func newDeployment(name string, replicas *int32, cpu int64, ram int64) apps.Deployment {
	selector := make(map[string]string)
	selector["name"] = name
	resourceList := make(map[v1.ResourceName]resource.Quantity)
	resourceList[v1.ResourceCPU] = *resource.NewMilliQuantity(cpu, resource.DecimalSI)
	resourceList[v1.ResourceMemory] = *resource.NewQuantity(ram, resource.DecimalSI)
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
// int32Ptr function
//
func int32Ptr(i int32) *int32 { return &i }

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
					Deployment: newDeployment("driver", replicas, 100, 100*1000*1000),
				},
				{
					Name: "processor",
					Regions: []*fadeplv1alpha1.FARegion{
						{
							RegionRequired: "002-002",
						},
					},
					Deployment: newDeployment("processor", replicas, 100, 400*1000*1000),
				},
			},
			DataFlows: []*fadeplv1alpha1.FADeplDataFlow{
				{
					BandwidthRequired: 5000000,
					LatencyRequired:   20,
					SourceId:          "cam1",
					DestinationId:     "driver",
				},
				{
					BandwidthRequired: 100000,
					LatencyRequired:   500,
					SourceId:          "driver",
					DestinationId:     "processor",
				},
			},
		},
	}
}

//
// expectedPlacementSilly function
//
func expectedPlacementSilly() string {
	status := `{"placements":[{"regions":[{"regionrequired":"003-003","regionselected":"003-003"}],"microservice":"driver"},{"regions":[{"regionrequired":"002-002","regionselected":"002-002"}],"microservice":"processor"}],"currentstatus":0}`
	return status
}
