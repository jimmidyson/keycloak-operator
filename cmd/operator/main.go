//  Copyright 2016 Red Hat, Inc.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/jimmidyson/keycloak-operator/pkg/keycloak"

	"github.com/go-kit/kit/log"
)

var (
	cfg              keycloak.Config
	analyticsEnabled bool
)

func init() {
	flagset := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	flagset.StringVar(&cfg.Host, "apiserver", "", "API Server addr, e.g. ' - NOT RECOMMENDED FOR PRODUCTION - http://127.0.0.1:8080'. Omit parameter to run in on-cluster mode and utilize the service account token.")
	flagset.StringVar(&cfg.TLSConfig.CertFile, "cert-file", "", " - NOT RECOMMENDED FOR PRODUCTION - Path to public TLS certificate file.")
	flagset.StringVar(&cfg.TLSConfig.KeyFile, "key-file", "", "- NOT RECOMMENDED FOR PRODUCTION - Path to private TLS certificate file.")
	flagset.StringVar(&cfg.TLSConfig.CAFile, "ca-file", "", "- NOT RECOMMENDED FOR PRODUCTION - Path to TLS CA file.")
	flagset.BoolVar(&cfg.TLSInsecure, "tls-insecure", false, "- NOT RECOMMENDED FOR PRODUCTION - Don't verify API server's CA certificate.")
	flagset.BoolVar(&analyticsEnabled, "analytics", true, "Send analytical event (Cluster Created/Deleted etc.) to Google Analytics")

	flagset.Parse(os.Args[1:])
}

func Main() int {
	logger := log.NewContext(log.NewLogfmtLogger(os.Stdout)).
		With("ts", log.DefaultTimestampUTC, "caller", log.DefaultCaller)

	ko, err := keycloak.New(cfg, logger.With("operator", "keycloak"))
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		return 1
	}

	stopc := make(chan struct{})
	errc := make(chan error)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		if err := ko.Run(stopc); err != nil {
			errc <- err
		}
		wg.Done()
	}()

	term := make(chan os.Signal)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)
	select {
	case <-term:
		fmt.Fprint(os.Stdout, "Received SIGTERM, exiting gracefully...")
		close(stopc)
		wg.Wait()
	case <-errc:
		fmt.Fprintf(os.Stderr, "Unhandled error received. Exiting...")
		close(stopc)
		wg.Wait()
		return 1
	}

	return 0
}

func main() {
	os.Exit(Main())
}
