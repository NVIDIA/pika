/*
Copyright 2024.

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

package controllers

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	ngn2v1alpha1 "github.com/NVIDIA/pika/api/v1alpha1"
	"github.com/NVIDIA/pika/pkg/config"
	"github.com/NVIDIA/pika/pkg/notify"

	//+kubebuilder:scaffold:imports
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
	err       error

	mockCtrl *gomock.Controller
	notifier *notify.MockNotifier
	nmRec    *NotifyMaintenanceReconciler

	scheme = runtime.NewScheme()

	// test k8s obj
	namespaceName  types.NamespacedName
	namespaceName2 types.NamespacedName
	object         string
	nm             *ngn2v1alpha1.NotifyMaintenance
	nm2            *ngn2v1alpha1.NotifyMaintenance
	testNode       *k8sv1.Node
)

const (
	testNamespace    = "notify-ns"
	maintenanceID    = "maintenance-1"
	newMaintenanceID = "maintenance-2"

	timeout  = time.Second * 10
	interval = time.Second * 1
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	f := false
	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		UseExistingCluster:    &f,
	}

	cfg, err = testEnv.Start()

	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	// +kubebuilder:scaffold:scheme

	SetupNotifyController(cfg)

	Expect(k8sClient.Create(context.TODO(), newNamespace(testNamespace))).Should(Succeed())

})

func SetupNotifyController(cfg *rest.Config) {
	time.Sleep(1 * time.Second)
	scheme = runtime.NewScheme()
	err = clientgoscheme.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	err = ngn2v1alpha1.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: "0",
	})
	Expect(err).NotTo(HaveOccurred())

	k8sClient = mgr.GetClient()
	Expect(k8sClient).NotTo(BeNil())

	mockCtrl = gomock.NewController(GinkgoT())
	notifier = notify.NewMockNotifier(mockCtrl)

	tmpDir := GinkgoT().TempDir()
	config.DefaultConfigPath = filepath.Join(tmpDir, "config.yaml")
	data, err := yaml.Marshal(config.Config{
		SLAPeriod: 5 * time.Hour,
	})
	Expect(err).NotTo(HaveOccurred())
	err = os.WriteFile(config.DefaultConfigPath, data, 0600)
	Expect(err).NotTo(HaveOccurred())

	nmRec = &NotifyMaintenanceReconciler{
		Client: mgr.GetClient(),
		Log:    logf.Log,
		Scheme: mgr.GetScheme(),
		Notifiers: []notify.NewNotifierFunc{
			func(_ config.Config, _ logr.Logger) (notify.Notifier, error) {
				return notifier, nil
			},
		},
	}
	err = nmRec.SetupWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = mgr.Start(ctx)
		Expect(err).NotTo(HaveOccurred())
	}()
}

func newNamespace(ns string) *k8sv1.Namespace {
	return &k8sv1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ns,
		},
	}
}

var _ = AfterSuite(func() {
	cancel()

	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
