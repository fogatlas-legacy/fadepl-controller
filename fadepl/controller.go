/*
Derivative work:
Copyright 2019 FBK
The controller workflow is mainly the original one
This file has been modified by FBK in order to adapt it to the fadepl needs in terms
of handling the FADepl and related resources

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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	appslisters "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	fadeplv1alpha1 "github.com/fogatlas/crd-client-go/pkg/apis/fogatlas/v1alpha1"
	clientset "github.com/fogatlas/crd-client-go/pkg/generated/clientset/versioned"
	fadeplscheme "github.com/fogatlas/crd-client-go/pkg/generated/clientset/versioned/scheme"
	informers "github.com/fogatlas/crd-client-go/pkg/generated/informers/externalversions/fogatlas/v1alpha1"
	listers "github.com/fogatlas/crd-client-go/pkg/generated/listers/fogatlas/v1alpha1"
)

const controllerAgentName = "fadepl"
const deploymentNameSeparator = ".reg."

const (
	// SuccessSynced is used as part of the Event 'reason' when a FADepl is synced
	SuccessSynced = "Synced"
	// ErrResourceExists is used as part of the Event 'reason' when a FADepl fails
	// to sync due to a Deployment of the same name already existing.
	ErrResourceExists = "ErrResourceExists"

	// MessageResourceExists is the message used for Events when a resource
	// fails to sync due to a Deployment already existing
	MessageResourceExists = "Resource %q already exists and is not managed by FADepl"
	// MessageResourceSynced is the message used for an Event fired when a FADepl
	// is synced successfully
	MessageResourceSynced = "FADepl synced successfully"
)

// PlacementAlgorithm provides the specification of a placement algorithm
type PlacementAlgorithm interface {
	Init(name string, kubeclientset kubernetes.Interface, faDeplclientset clientset.Interface)
	CalculatePlacement(fadepl *fadeplv1alpha1.FADepl) (err error)
	CalculateUpdate(fadepl *fadeplv1alpha1.FADepl) (err error)
}

// Controller is the controller implementation for FaDepl resources
type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// faDeplclientset is a clientset for our own API group
	faDeplclientset clientset.Interface

	deploymentsLister appslisters.DeploymentLister
	deploymentsSynced cache.InformerSynced
	faDeplsLister     listers.FADeplLister
	faDeplsSynced     cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
	algoMap  map[string]PlacementAlgorithm
}

//
// RegisterAlgoImpl registers an algorithm to the controller
//
func (c *Controller) RegisterAlgoImpl(p map[string]PlacementAlgorithm) {
	c.algoMap = p
}

//
// NewController function returns a new sample controller
//
func NewController(
	kubeclientset kubernetes.Interface,
	faDeplclientset clientset.Interface,
	deploymentInformer appsinformers.DeploymentInformer,
	faDeplInformer informers.FADeplInformer) *Controller {

	// Create event broadcaster
	// Add fadepl types to the default Kubernetes Scheme so Events can be
	// logged for fadepl types.
	utilruntime.Must(fadeplscheme.AddToScheme(scheme.Scheme))
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("default")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		kubeclientset:     kubeclientset,
		faDeplclientset:   faDeplclientset,
		deploymentsLister: deploymentInformer.Lister(),
		deploymentsSynced: deploymentInformer.Informer().HasSynced,
		faDeplsLister:     faDeplInformer.Lister(),
		faDeplsSynced:     faDeplInformer.Informer().HasSynced,
		workqueue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "FADepls"),
		recorder:          recorder,
	}

	// Set up an event handler for when FADepl resources change
	faDeplInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueFADepl,
		UpdateFunc: func(old, new interface{}) {
			newFADepl := new.(*fadeplv1alpha1.FADepl)
			oldFADepl := old.(*fadeplv1alpha1.FADepl)

			if newFADepl.ResourceVersion == oldFADepl.ResourceVersion {
				// Periodic resync will send update events for all known FADepl.
				// Two different versions of the same FADepl will always have different RVs.
				return
			} else if newFADepl.DeletionTimestamp == nil { // because DeleteFunc is called after deletion
				specOld, err := json.Marshal(oldFADepl.Spec)
				specNew, err1 := json.Marshal(newFADepl.Spec)
				if err == nil && err1 == nil {
					if string(specOld) == string(specNew) {
						return
					}
					newFADepl.Status.CurrentStatus = fadeplv1alpha1.Changed
				}
			}
			controller.enqueueFADepl(new)
		},
	})
	// Set up an event handler for when Deployment resources change. This
	// handler will lookup the owner of the given Deployment, and if it is
	// owned by a FADepl resource will enqueue that FADepl resource for
	// processing. This way, we don't need to implement custom logic for
	// handling Deployment resources. More info on this pattern:
	// https://github.com/kubernetes/community/blob/8cafef897a22026d42f5e5bb3f104febe7e29830/contributors/devel/controllers.md
	// NOTE: indeed this event handler is not exploited in this controller:
	// we didn't implement any business logic to handle the case of a Deployment, belonging to a FADepl,
	// changed, added or deleted. This means that a direct change to a Deployment has no effect
	// apart the fact of producing a misalignement.
	// For this reason, I commented it out.
	/*
		deploymentInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: controller.handleObject,
			UpdateFunc: func(old, new interface{}) {
				newDepl := new.(*appsv1.Deployment)
				oldDepl := old.(*appsv1.Deployment)
				if newDepl.ResourceVersion == oldDepl.ResourceVersion {
					// Periodic resync will send update events for all known Deployments.
					// Two different versions of the same Deployment will always have different RVs.
					return
				}
				controller.handleObject(new)
			},
			DeleteFunc: controller.handleObject,
		})
	*/
	return controller
}

//
// Run method will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
//
func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	if ok := cache.WaitForCacheSync(stopCh, c.deploymentsSynced, c.faDeplsSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	// Launch two workers to process fadepl resources
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	log.Info("Started workers")
	<-stopCh
	log.Info("Shutting down workers")

	return nil
}

//
// runWorker method is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
//
func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

//
// processNextWorkItem method will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
//
func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

	before := time.Now()
	log.Infof("<PERF> Before processEvent: time is %v", before.UnixNano())

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.
		if key, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// FADepl resource to be synced.
		if err := c.syncHandler(key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		log.Info("Successfully synced: ", key)
		return nil
	}(obj)

	after := time.Now()
	log.Infof("<PERF> After processEvent: time is %v", after.UnixNano())
	log.Infof("<PERF> Elapsed time for processEvent is %v", after.Sub(before))

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

//
// syncHandler method compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the FADepl resource
// with the current status of the resource.
// ***NOTE: this is the function where you have to put the business logic ***
//
func (c *Controller) syncHandler(key string) error {
	log.Tracef("key is %s ", key)
	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the FADepl resource with this namespace/name
	fadepl, err := c.faDeplsLister.FADepls(namespace).Get(name)
	if err != nil {
		// The FADepl resource may no longer exist, in which case we stop
		// processing.
		if errors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("fadepl '%s' in work queue no longer exists", key))
			return nil
		}
		return err
	}
	//Check if deletion is needed
	if fadepl.DeletionTimestamp != nil {
		log.Infof("Going to purge and delete the object (%s)", fadepl.DeletionTimestamp)
		finalizers := fadepl.ObjectMeta.GetFinalizers()
		if finalizers != nil {
			// Call the needed business logic
			c.handleCleanup(fadepl)
			faDeplCopy := fadepl.DeepCopy()
			faDeplCopy.ObjectMeta.Finalizers = nil
			_, err1 := c.faDeplclientset.FogatlasV1alpha1().FADepls(fadepl.Namespace).Update(context.TODO(), faDeplCopy, metav1.UpdateOptions{})
			return err1
		}
	}
	if fadepl.Status.CurrentStatus == fadeplv1alpha1.Synced || fadepl.Status.CurrentStatus == fadeplv1alpha1.Failed {
		log.Trace("No need to do anything. Returning ...")
		return nil
	}
	// Just for tracing purposes, print the fadepl
	str, err := json.Marshal(fadepl)
	if err != nil {
		log.Errorf("Error marshalling fadepl: (%s)", err.Error())
	} else {
		log.Tracef("Received following fadepl (%s)", str)
	}
	// If it is an update of FADepl then call handleUpdate() function that in turn calls
	// algo.CalculateUpdate()
	// For example you could need to do some operations before re-placement
	if fadepl.Status.CurrentStatus == fadeplv1alpha1.Changed {
		c.handleUpdate(fadepl)
	}
	// Handle placement of the fadepl.
	before := time.Now()
	log.Infof("<PERF> Before handlePlacement: time is %v", before.UnixNano())
	err = c.handlePlacement(fadepl)
	after := time.Now()
	log.Infof("<PERF> After handlePlacement: time is %v", after.UnixNano())
	log.Infof("<PERF> Elapsed time for handlePlacement is %v", after.Sub(before))
	if err != nil {
		fadepl.Status.CurrentStatus = fadeplv1alpha1.Failed
		// We choose to absorb the error here as the worker would requeue the
		// resource otherwise. Instead, the next time the resource is updated
		// the resource will be queued again.
		err = nil
	}
	for _, m := range fadepl.Spec.Microservices {
		if fadepl.Status.CurrentStatus == fadeplv1alpha1.Failed {
			break
		}
		pl := c.getPlacement(m.Name, fadepl)
		if pl == nil {
			fadepl.Status.CurrentStatus = fadeplv1alpha1.Failed
			log.Errorf("Unable to find placement for ms (%s).", m.Name)
			break
		} else {
			//lists the regions involved in this FADepl
			var involvedRegions []string
			var dName string //name of this deployment
			for _, r := range pl.Regions {
				involvedRegions = append(involvedRegions, r.RegionSelected)
				nodeSelectorLabels := make(map[string]string)
				inputSelectorLabels := m.Deployment.Spec.Template.Spec.NodeSelector
				if inputSelectorLabels != nil {
					for k, v := range inputSelectorLabels {
						nodeSelectorLabels[k] = v
					}
				}
				nodeSelectorLabels["region"] = r.RegionSelected
				depl := m.Deployment.DeepCopy()
				labels := make(map[string]string)
				inputMetaLabels := m.Deployment.ObjectMeta.Labels
				if inputMetaLabels != nil {
					for k, v := range inputMetaLabels {
						labels[k] = v
					}
				}
				labels["fadepl"] = fadepl.Name
				depl.ObjectMeta.SetLabels(labels)
				depl.Spec.Template.Spec.NodeSelector = nodeSelectorLabels
				if r.Replicas > 0 {
					depl.Spec.Replicas = &r.Replicas
				}
				//Assuming just one container per pod in case of overriding
				if r.Image != "" {
					if len(depl.Spec.Template.Spec.Containers) == 1 {
						depl.Spec.Template.Spec.Containers[0].Image = r.Image
					} else {
						log.Warnf("Image override with more than one container per pod. Ignoring it")
					}
				}
				// If CPU resources are specified as MIPS, convert them to Quantities.
				// We always assume to have one container per Pod. If more than one, we skip
				// the usage of MIPSRequired.
				// Similarly this work only of CPU2MIPS is set. If equal to 0, we skip it
				if m.MIPSRequired.CmpInt64(0) == 1 {
					if len(depl.Spec.Template.Spec.Containers) == 1 {
						if r.CPU2MIPSMilli == 0 {
							log.Warnf("MIPSRequired used but CPU2MIPS conversion factor not set. Did you select the right algorithm? Ignoring it.")
						} else {
							newCPU := int64(m.MIPSRequired.Value() / r.CPU2MIPSMilli)
							depl.Spec.Template.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU] = *resource.NewMilliQuantity(newCPU, resource.DecimalSI)
						}
					} else {
						log.Warnf("MIPSRequired used with more than one container per pod. Ignoring it.")
					}
				}
				references := []metav1.OwnerReference{*metav1.NewControllerRef(fadepl, fadeplv1alpha1.SchemeGroupVersion.WithKind("FADepl"))}
				depl.SetOwnerReferences(references)

				// Get the deployment with the name specified in FADepl.spec with RegionSelected as suffix
				deplName := m.Name + deploymentNameSeparator + r.RegionSelected
				depl.Name = deplName
				dName = m.Name
				deployment, err1 := c.deploymentsLister.Deployments(fadepl.Namespace).Get(depl.Name)
				// If the resource doesn't exist, we'll create it
				if errors.IsNotFound(err1) {
					log.Trace("Creating new deployment")
					before := time.Now()
					log.Infof("<PERF> Before createDeployment: time is %v", before.UnixNano())
					deployment, err1 = c.kubeclientset.AppsV1().Deployments(fadepl.Namespace).Create(context.TODO(), depl, metav1.CreateOptions{})
					after := time.Now()
					log.Infof("<PERF> After createDeployment: time is %v", after.UnixNano())
					log.Infof("<PERF> Elapsed time for createDeployment is %v", after.Sub(before))
				} else {
					log.Trace("Updating deployment")
					deployment, err1 = c.kubeclientset.AppsV1().Deployments(fadepl.Namespace).Update(context.TODO(), depl, metav1.UpdateOptions{})
				}

				// If an error occurs during Get/Create, we won't requeue the item. User
				// has to re-submit it
				if err1 != nil {
					log.Errorf("Unable to create/update deployment (%s)", err1)
					fadepl.Status.CurrentStatus = fadeplv1alpha1.Failed
					break
				}

				// If the Deployment is not controlled by this FADepl resource, we should log
				// a warning to the event recorder.
				if !metav1.IsControlledBy(deployment, fadepl) {
					fadepl.Status.CurrentStatus = fadeplv1alpha1.Failed
					msg := fmt.Sprintf(MessageResourceExists, deployment.Name)
					c.recorder.Event(fadepl, corev1.EventTypeWarning, ErrResourceExists, msg)
					log.Errorf("Deployment is not controlled by this FADepl (%s)", msg)
					break
				}
			}
			// Check if there are deployments, related with this fadepl, but not involved in this update that should be purged.
			label := "fadepl=" + fadepl.Name
			deployments, err1 := c.kubeclientset.AppsV1().Deployments(fadepl.Namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: label})
			if err1 != nil {
				fadepl.Status.CurrentStatus = fadeplv1alpha1.Failed
				log.Errorf("Error when searching for deployments with label (%s) (%s)", label, err1)
			} else {
				for _, d := range deployments.Items {
					//pieces[0] is ms name, pieces[1] is region name
					pieces := strings.Split(d.Name, deploymentNameSeparator)
					if pieces[0] != dName {
						continue
					}
					found := false
					for _, k := range involvedRegions {
						if strings.Contains(d.Name, k) {
							found = true
							break
						}
					}
					if !found {
						log.Infof("Found a deployment to be purged (%s)", d.Name)
						c.kubeclientset.AppsV1().Deployments(fadepl.Namespace).Delete(context.TODO(), d.Name, metav1.DeleteOptions{})
					}
				}
			}
		}
	}
	if fadepl.Status.CurrentStatus != fadeplv1alpha1.Failed {
		fadepl.Status.CurrentStatus = fadeplv1alpha1.Synced
	}

	// Finally, we update the status block of the Link resources to keep into account the
	// allocated bandwidth and the status block of the FADepl resource to reflect the
	// current state of the world
	if fadepl.Status.CurrentStatus == fadeplv1alpha1.Synced {
		err = c.updateLinksStatus(fadepl, true)
		if err != nil {
			fadepl.Status.CurrentStatus = fadeplv1alpha1.Failed
			log.Errorf("Error updating links (%s)", err)
		}
	}
	err = c.updateFADeplStatus(fadepl)
	// Apparently we have to slow down the process in order to be sure that the status
	// is correctly updated.
	// [20200424] Guess it is no more needed. Commenting it out.
	//time.Sleep(200 * time.Millisecond)
	if err != nil {
		return err
	}
	
	if fadepl.Status.CurrentStatus == fadeplv1alpha1.Synced {
		c.recorder.Event(fadepl, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	} else {
		c.recorder.Event(fadepl, corev1.EventTypeWarning, "Failed", "FADepl is in failed status")
	}
	return nil
}

//
// updateFADeplStatus method
//
func (c *Controller) updateFADeplStatus(fadepl *fadeplv1alpha1.FADepl) error {
	// NEVER modify objects from the store. It's a read-only, local cache.
	// You can use DeepCopy() to make a deep copy of original object and modify this copy
	// Or create a copy manually for better performance
	faDeplCopy := fadepl.DeepCopy()
	// If the CustomResourceSubresources feature gate is not enabled,
	// we must use Update instead of UpdateStatus to update the Status block of the FaDepl resource.
	// UpdateStatus will not allow changes to the Spec of the resource,
	// which is ideal for ensuring nothing other than resource status has been updated.
	_, err := c.faDeplclientset.FogatlasV1alpha1().FADepls(fadepl.Namespace).UpdateStatus(context.TODO(), faDeplCopy, metav1.UpdateOptions{})
	if err != nil {
		log.Errorf("Error updating FADepl status (%s)", err)
	}
	return err
}

//
// updateLinksStatus method
//
func (c *Controller) updateLinksStatus(fadepl *fadeplv1alpha1.FADepl, deploy bool) error {
	// NEVER modify objects from the store. It's a read-only, local cache.
	// You can use DeepCopy() to make a deep copy of original object and modify this copy
	// Or create a copy manually for better performance
	links, err := c.faDeplclientset.FogatlasV1alpha1().Links("default").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Errorf("Error: (%s)", err.Error())
		return err
	}
	var lCopy *fadeplv1alpha1.Link
	for _, link := range links.Items {
		for _, lo := range fadepl.Status.LinksOccupancy {
			if lo.LinkID == link.Spec.ID {
				if deploy == true && lo.IsChanged == true {
					link.Status.BwAllocated.Sub(lo.PrevBwAllocated)
					link.Status.BwAllocated.Add(lo.BwAllocated)
				} else if deploy == false || lo.IsChanged == false {
					link.Status.BwAllocated.Sub(lo.BwAllocated)
					lo.BwAllocated.Set(0)
				}
				lo.IsChanged = false
				lCopy = link.DeepCopy()
				_, err := c.faDeplclientset.FogatlasV1alpha1().Links("default").UpdateStatus(context.TODO(), lCopy, metav1.UpdateOptions{})
				if err != nil {
					log.Errorf("Error updating Link status (%s)", err)
					return err
				}
				break
			}
		}
	}

	return nil
}

//
// enqueueFADepl method takes a FADepl resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than FADepl.
//
func (c *Controller) enqueueFADepl(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.Add(key)
}

//
// handleObject  method will take any resource implementing metav1.Object and attempt
// to find the FADepl resource that 'owns' it. It does this by looking at the
// objects metadata.ownerReferences field for an appropriate OwnerReference.
// It then enqueues that FADepl resource to be processed. If the object does not
// have an appropriate OwnerReference, it will simply be skipped.
//
func (c *Controller) handleObject(obj interface{}) {
	var object metav1.Object
	var ok bool
	if object, ok = obj.(metav1.Object); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}
	}
	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		// If this object is not owned by a FADepl, we should not do anything more
		// with it.
		if ownerRef.Kind != "FADepl" {
			return
		}

		fadepl, err := c.faDeplsLister.FADepls(object.GetNamespace()).Get(ownerRef.Name)
		if err != nil {
			return
		}

		c.enqueueFADepl(fadepl)
		return
	}
}

//
// getPlacement method
//
func (c *Controller) getPlacement(msName string, fadepl *fadeplv1alpha1.FADepl) (pl *fadeplv1alpha1.FAPlacement) {
	placements := fadepl.Status.Placements
	for _, pl := range placements {
		if pl.Microservice == msName {
			return pl
		}
	}
	return nil
}

//
// handlePlacement method
//
func (c *Controller) handlePlacement(fadepl *fadeplv1alpha1.FADepl) (err error) {
	defer trace("handlePlacement()")()

	algo := fadepl.Spec.Algorithm
	chosenAlgorithm := c.algoMap[algo]
	if chosenAlgorithm == nil {
		log.Error("Unable to determine algorithm")
		return fmt.Errorf("Unable to determine algorithm")
	}
	err = chosenAlgorithm.CalculatePlacement(fadepl)
	if err != nil {
		return err
	}

	return nil
}

//
// handleUpdate method
//
func (c *Controller) handleUpdate(fadepl *fadeplv1alpha1.FADepl) {
	defer trace("handleUpdate()")()

	if fadepl.Status.CurrentStatus != fadeplv1alpha1.Synced && fadepl.Status.CurrentStatus != fadeplv1alpha1.Changed {
		return
	}
	//call algorithm logic
	algo := fadepl.Spec.Algorithm
	chosenAlgorithm := c.algoMap[algo]
	if chosenAlgorithm == nil {
		log.Error("Unable to determine algorithm")
		return
	}
	err := chosenAlgorithm.CalculateUpdate(fadepl)
	if err != nil {
		log.Error("Unable to calculate update")
		return
	}
}

//
// handleCleanup method
//
func (c *Controller) handleCleanup(fadepl *fadeplv1alpha1.FADepl) {
	defer trace("handleCleanup()")()

	err := c.updateLinksStatus(fadepl, false)
	if err != nil {
		fadepl.Status.CurrentStatus = fadeplv1alpha1.Failed
		log.Errorf("Error updating links (%s)", err)
	}
}

//
// trace function
//
func trace(msg string) func() {
	start := time.Now()
	log.Tracef("enter %s", msg)
	return func() { log.Tracef("exit %s (%s)", msg, time.Since(start)) }
}
