package silly

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestSilly(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Silly Suite")
}
