package config

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config", func() {
	Context("Read config", func() {
		var (
			workDir    string
			configPath string
		)

		writeConfig := func(configString string) {
			var err error
			workDir, err = os.MkdirTemp("", "")
			Expect(err).To(Succeed())

			f, err := os.CreateTemp(workDir, "config-*.yaml")
			Expect(err).To(Succeed())
			configPath = f.Name()

			_, err = f.WriteString(configString)
			Expect(err).To(Succeed())
		}

		AfterEach(func() {
			Expect(os.RemoveAll(workDir)).To(Succeed())
		})

		It("should read config", func() {
			writeConfig("slaPeriod: 5h")
			config, err := ReadConfig(configPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(config.SLAPeriod).To(Equal(time.Hour * 5))
		})
	})
})
