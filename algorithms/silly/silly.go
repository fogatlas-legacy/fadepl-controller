package silly

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	fadeplv1alpha1 "github.com/fogatlas/crd-client-go/pkg/apis/fogatlas/v1alpha1"
	clientset "github.com/fogatlas/crd-client-go/pkg/generated/clientset/versioned"

	"k8s.io/client-go/kubernetes"
)

type SillyAlgorithm struct {
	name            string
	kubeclientset   kubernetes.Interface
	faDeplclientset clientset.Interface
	initialized     bool
}

//
// Init function
//
func (p *SillyAlgorithm) Init(name string, kubeclientset kubernetes.Interface, faDeplclientset clientset.Interface) {
	p.name = name
	p.kubeclientset = kubeclientset
	p.faDeplclientset = faDeplclientset
	p.initialized = true
}

//
// CalculatePlacement method
//
func (p *SillyAlgorithm) CalculatePlacement(depl *fadeplv1alpha1.FADepl) (err error) {
	defer trace("CalculatePlacement()")()
	log.Infof("Going to calculate the placement of %s with algorithm %s", depl.Name, p.name)

	if p.initialized == false {
		log.Errorf("SillyAlgorithm not intitialized. Unable to proceed.")
		return fmt.Errorf("SillyAlgorithm not intitialized. Unable to proceed.")
	}

	fadeplmicroservices := depl.Spec.Microservices
	for _, m := range fadeplmicroservices {
		if len(m.Regions) == 0 {
			log.Errorf("Region required not set for at least one microservice")
			return fmt.Errorf("Region required not set for at least one microservice")
		}

		for _, reg := range m.Regions {
			if reg.RegionRequired == "" {
				log.Errorf("Region required not set for at least one microservice")
				return fmt.Errorf("Region required not set for at least one microservice")
			}
			log.Infof("Selected regions for microservice (%s) is (%s)", m.Name, reg.RegionRequired)
		}
		//update FADepl status
		fillInFADeplStatus(depl, m)
	}

	return nil
}

//
// CalculateUpdate method
//
func (p *SillyAlgorithm) CalculateUpdate(fadepl *fadeplv1alpha1.FADepl) (err error) {
	defer trace("CalculateUpdate()")()

	// Just return
	return nil
}

//
// FillInFADeplStatus function
//
func fillInFADeplStatus(fadepl *fadeplv1alpha1.FADepl, ms *fadeplv1alpha1.FADeplMicroservice) {
	defer trace("FillInFADeplStatus()")()
	//delete previous placement structure for this FADepl
	for i, pl := range fadepl.Status.Placements {
		if pl.Microservice == ms.Name {
			fadepl.Status.Placements[i] = fadepl.Status.Placements[len(fadepl.Status.Placements)-1]
			fadepl.Status.Placements = fadepl.Status.Placements[:len(fadepl.Status.Placements)-1]
			break
		}
	}

	var placement fadeplv1alpha1.FAPlacement
	for _, r := range ms.Regions {
		var replicas int32
		var image string
		if r.Replicas != 0 {
			replicas = r.Replicas
		}
		if r.Image != "" {
			image = r.Image
		}
		region := fadeplv1alpha1.FARegion{RegionRequired: r.RegionRequired,
			Replicas: replicas,
			Image:    image,
			// used only if MIPS are used. In that case this field must be set to
			// the conversion factor from millicore to MIPS for the selected region.
			CPU2MIPSMilli:  0,
			RegionSelected: r.RegionRequired}
		placement.Regions = append(placement.Regions, &region)
	}
	placement.Microservice = ms.Name
	fadepl.Status.Placements = append(fadepl.Status.Placements, &placement)

	// In a more reasonable algorithm, you could also take advantage of the
	// fadepl.status.LinksOccupancy structure to keep into account the link occupancy.
	// The controller will then decrease that amount of bandwidth available from the
	// referenced link.
}

func trace(msg string) func() {
	start := time.Now()
	log.Tracef("enter %s ", msg)
	return func() { log.Tracef("exit %s (%s) ", msg, time.Since(start)) }
}
