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

package main

import (
	flags "github.com/jessevdk/go-flags"
	"github.com/fogatlas/fadepl-controller/fadepl"
	"k8s.io/sample-controller/pkg/signals"
)

type Options struct {
	Kubeconfig string `long:"kubeconfig" description:"absolute path to the kubeconfig file."`
	MasterURL  string `long:"master" description:"The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster."`
	LogLevel   string `long:"loglevel" description:"The log level." env:"LOGLEVEL"`
}

var opts Options

//
// main function
//
func main() {
	var parser = flags.NewParser(&opts, flags.Default)
	parser.Parse()
	kubeconfig := opts.Kubeconfig
	masterURL := opts.MasterURL
	logLevel := opts.LogLevel
	k8sclient, faclient := fadepl.Initialize(kubeconfig, masterURL, logLevel)
	stopCh := signals.SetupSignalHandler()
	fadepl.Run(k8sclient, faclient, stopCh)
}
