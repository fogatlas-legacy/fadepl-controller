module github.com/fogatlas/fadepl-controller

go 1.13

require (
	github.com/jessevdk/go-flags v1.4.0
	github.com/onsi/ginkgo v1.14.1
	github.com/onsi/gomega v1.10.2
	github.com/sirupsen/logrus v1.4.2
	github.com/fogatlas/crd-client-go v0.2.0-0.9.1
	google.golang.org/appengine v1.5.0
	k8s.io/api v0.15.9
	k8s.io/apimachinery v0.15.9
	k8s.io/client-go v0.15.9
	k8s.io/sample-controller v0.15.9
)

//replace github.com/fogatlas/crd-client-go => ../crd-client-go
