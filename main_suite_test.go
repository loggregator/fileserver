package main_test

import (
	"fmt"
	"os"

	"github.com/cloudfoundry-incubator/inigo/fake_cc"
	"github.com/cloudfoundry/storeadapter/storerunner/etcdstorerunner"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"

	"testing"
)

var fileServerBinary string
var etcdRunner *etcdstorerunner.ETCDClusterRunner
var fakeCC *fake_cc.FakeCC
var fakeCCProcess ifrit.Process

func TestFileServer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "File Server Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	fileServerPath, err := gexec.Build("github.com/cloudfoundry-incubator/file-server")
	Ω(err).ShouldNot(HaveOccurred())
	return []byte(fileServerPath)
}, func(fileServerPath []byte) {
	fakeCCAddress := fmt.Sprintf("127.0.0.1:%d", 6767+GinkgoParallelNode())
	fakeCC = fake_cc.New(fakeCCAddress)

	etcdRunner = etcdstorerunner.NewETCDClusterRunner(5001+GinkgoParallelNode(), 1)

	fileServerBinary = string(fileServerPath)
})

var _ = SynchronizedAfterSuite(func() {
	etcdRunner.Stop()
}, func() {
	gexec.CleanupBuildArtifacts()
})

var _ = BeforeEach(func() {
	etcdRunner.Start()
	fakeCCProcess = ifrit.Envoke(fakeCC)
})

var _ = AfterEach(func() {
	etcdRunner.Stop()
	fakeCCProcess.Signal(os.Kill)
	Eventually(fakeCCProcess.Wait()).Should(Receive(BeNil()))
})
