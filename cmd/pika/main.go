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

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	ngn2v1alpha1 "github.com/NVIDIA/pika/api/v1alpha1"
	"github.com/NVIDIA/pika/controllers"

	"github.com/open-policy-agent/cert-controller/pkg/rotator"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	//+kubebuilder:scaffold:imports
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	caName             = "nidavellir-ca"
	caOrganization     = "nidavellir"
	metricsServiceName = "pika-controller-manager-metrics-service"

	secretName            = "pika-server-cert" // #nosec G101
	webhookServiceName    = "pika-controller-manager-webhook-service"
	validatingWebhookName = "pika-validating-webhook-configuration"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(ngn2v1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var (
		metricsPort          int
		enableLeaderElection bool
		probeAddr            string
		certDir              string
		syncPeriod           time.Duration
	)
	flag.IntVar(&metricsPort, "metrics-port", 8383, "Port where the metrics server is running on.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.StringVar(&certDir, "cert-dir", "/certs", "The directory where certs are stored, defaults to /certs")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.DurationVar(&syncPeriod, "sync-period", 10*time.Hour, "Determines how often we should do reconciliation of watched resources")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		CertDir:                certDir,
		Scheme:                 scheme,
		MetricsBindAddress:     "0",
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "a316e27f.nvidia.com",
		SyncPeriod:             pointer.Duration(syncPeriod),
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.NotifyMaintenanceReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Log:    logf.Log,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NotifyMaintenance")
		os.Exit(1)
	}

	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupFinished := make(chan struct{})
	namespace := os.Getenv("NAMESPACE")
	if err := rotator.AddRotator(mgr, &rotator.CertRotator{
		SecretKey: types.NamespacedName{
			Namespace: namespace,
			Name:      secretName,
		},
		CertDir:        certDir,
		CAName:         caName,
		CAOrganization: caOrganization,
		DNSName:        fmt.Sprintf("%s.%s.svc", webhookServiceName, namespace),
		ExtraDNSNames:  []string{fmt.Sprintf("%s.%s.svc", metricsServiceName, namespace)},
		IsReady:        setupFinished,
		Webhooks: []rotator.WebhookInfo{
			{Name: validatingWebhookName, Type: rotator.Validating},
		},
		ExtKeyUsages: &[]x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}); err != nil {
		setupLog.Error(err, "unable to set up cert rotation")
		os.Exit(1)
	}

	// Setup webhook server in parallel but only once the rotator completes setup.
	go func() {
		<-setupFinished

		// Start custom Metrics Server.
		go runPrometheusServer(metricsPort, certDir)

		err = (&ngn2v1alpha1.NotifyMaintenance{}).SetupWebhookWithManager(mgr)
		if err != nil {
			setupLog.Error(err, "failed to build webhook for NotifyMaintenance")
			os.Exit(1)
		}

		// this needs to be in a go-routine to make sure mgr.Start is run before client is usable
		start := mgr.GetCache().WaitForCacheSync(context.Background())
		if !start {
			setupLog.Info("cound not start cache")
			return
		}
	}()

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func runPrometheusServer(metricsPort int, certDir string) {
	certWatcher, err := certwatcher.New(filepath.Join(certDir, "tls.crt"), filepath.Join(certDir, "tls.key"))
	if err != nil {
		setupLog.Error(err, "unable to set up cert rotation watcher")
		os.Exit(1)
	}

	go func() {
		if err := certWatcher.Start(context.TODO()); err != nil {
			setupLog.Error(err, "certificate watcher error")
		}
	}()

	handler := promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{
		ErrorHandling: promhttp.HTTPErrorOnError,
	})
	mux := http.NewServeMux()
	mux.Handle("/metrics", handler)
	server := &http.Server{
		Handler:           mux,
		Addr:              fmt.Sprintf(":%d", metricsPort),
		ReadHeaderTimeout: 5 * time.Second,
		TLSConfig: &tls.Config{
			MinVersion:     tls.VersionTLS13,
			GetCertificate: certWatcher.GetCertificate,
		},
	}
	if err = server.ListenAndServeTLS("", ""); err != nil {
		setupLog.Error(err, "unable to server metrics")
		os.Exit(1)
	}
}
