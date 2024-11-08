package notify

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestNotify(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Notify Suite")
}
