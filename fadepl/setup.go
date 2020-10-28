/*
Derivative work:
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
	"time"

	log "github.com/sirupsen/logrus"

	clientset "github.com/fogatlas/crd-client-go/pkg/generated/clientset/versioned"
	informers "github.com/fogatlas/crd-client-go/pkg/generated/informers/externalversions"
	"github.com/fogatlas/fadepl-controller/algorithms/silly"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

//
// Initialize function
//
func Initialize(kubeconfig string, masterURL string, logLevel string) (kubeclient *kubernetes.Clientset,
	fogatlasclient *clientset.Clientset) {
	configureLog(logLevel)
	var cfg *rest.Config
	var err error

	if kubeconfig != "" {
		cfg, err = clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	} else {
		log.Infof("Loading K8S in-cluster configuration")
		cfg, err = rest.InClusterConfig()
	}
	if err != nil {
		log.Fatalf("Unable to get kubeconfig (%s)", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("Error building kubernetes clientset (%s)", err.Error())
	}

	fogatlasClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("Error building example clientset (%s)", err.Error())
	}
	return kubeClient, fogatlasClient
}

//
// Run function
//
func Run(kubeClient *kubernetes.Clientset, fogatlasClient *clientset.Clientset, stopCh <-chan struct{}) {

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*300)
	fogatlasInformerFactory := informers.NewSharedInformerFactory(fogatlasClient, time.Second*300)

	controller := NewController(kubeClient, fogatlasClient,
		kubeInformerFactory.Apps().V1().Deployments(),
		fogatlasInformerFactory.Fogatlas().V1alpha1().FADepls())

	// Register all the algorithms defined
	registerAlgorithms(controller, kubeClient, fogatlasClient)
	// notice that there is no need to run Start methods in a separate goroutine. (i.e. go kubeInformerFactory.Start(stopCh)
	// Start method is non-blocking and runs all registered informers in a dedicated goroutine.
	kubeInformerFactory.Start(stopCh)
	fogatlasInformerFactory.Start(stopCh)

	if err := controller.Run(2, stopCh); err != nil {
		log.Fatalf("Error running controller (%s)", err.Error())
	}

}

//
// configureLog function
//
func configureLog(logLevel string) {
	var loggerLevel log.Level
	switch logLevel {
	case "trace":
		loggerLevel = log.TraceLevel
	case "debug":
		loggerLevel = log.DebugLevel
	case "info":
		loggerLevel = log.InfoLevel
	case "warn":
		loggerLevel = log.WarnLevel
	case "error":
		loggerLevel = log.ErrorLevel
	case "fatal":
		loggerLevel = log.FatalLevel
	case "panic":
		loggerLevel = log.PanicLevel
	default:
		loggerLevel = log.InfoLevel
	}

	log.SetLevel(loggerLevel)
	log.SetFormatter(&log.TextFormatter{
		DisableColors: false,
		FullTimestamp: true,
	})

}

//
// registerAlgorithms function
//
func registerAlgorithms(controller *Controller,
	kubeclientset kubernetes.Interface,
	faDeplclientset clientset.Interface) {
	m := make(map[string]PlacementAlgorithm)
	sillyAlgo := new(silly.SillyAlgorithm)
	sillyAlgo.Init("Silly", kubeclientset, faDeplclientset)
	m["Silly"] = sillyAlgo
	controller.RegisterAlgoImpl(m)
}
