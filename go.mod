module github.com/fogatlas/fadepl-controller

go 1.13

require (
	github.com/google/go-cmp v0.5.2
	github.com/jessevdk/go-flags v1.4.0
	github.com/onsi/ginkgo v1.14.1
	github.com/onsi/gomega v1.10.2
	github.com/sirupsen/logrus v1.6.0
	github.com/fogatlas/crd-client-go v0.3.0-1.0.1
	k8s.io/api v0.20.4
	k8s.io/apimachinery v0.20.4
	k8s.io/client-go v0.20.4
	k8s.io/sample-controller v0.20.4
)

//replace github.com/fogatlas/crd-client-go => ../crd-client-go
