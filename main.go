// Copyright (c) 2021 Doc.ai and/or its affiliates.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at:
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// +build linux

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/edwarnicke/grpcfd"
	"github.com/kelseyhightower/envconfig"
	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/networkservicemesh/api/pkg/api/networkservice"
	kernelmech "github.com/networkservicemesh/api/pkg/api/networkservice/mechanisms/kernel"
	vfiomech "github.com/networkservicemesh/api/pkg/api/networkservice/mechanisms/vfio"
	"github.com/networkservicemesh/sdk-sriov/pkg/networkservice/common/mechanisms/vfio"
	sriovtoken "github.com/networkservicemesh/sdk-sriov/pkg/networkservice/common/token"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/authorize"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/begin"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/mechanisms"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/mechanisms/kernel"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/mechanisms/sendfd"
	"github.com/networkservicemesh/sdk/pkg/networkservice/common/updatepath"
	"github.com/networkservicemesh/sdk/pkg/networkservice/connectioncontext/dnscontext"
	"github.com/networkservicemesh/sdk/pkg/networkservice/core/chain"
	"github.com/networkservicemesh/sdk/pkg/tools/grpcutils"
	"github.com/networkservicemesh/sdk/pkg/tools/jaeger"
	"github.com/networkservicemesh/sdk/pkg/tools/log"
	"github.com/networkservicemesh/sdk/pkg/tools/log/logruslogger"
	"github.com/networkservicemesh/sdk/pkg/tools/nsurl"
	"github.com/networkservicemesh/sdk/pkg/tools/opentracing"
	"github.com/networkservicemesh/sdk/pkg/tools/spiffejwt"
	"github.com/networkservicemesh/sdk/pkg/tools/token"

	"github.com/networkservicemesh/cmd-nsc-init/internal/config"
)

func main() {
	// ********************************************************************************
	// Configure signal handling context
	// ********************************************************************************
	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		// More Linux signals here
		syscall.SIGHUP,
		syscall.SIGTERM,
		syscall.SIGQUIT,
	)
	defer cancel()

	// ********************************************************************************
	// Setup logger
	// ********************************************************************************
	logrus.Info("Starting NetworkServiceMesh Client ...")
	logrus.SetFormatter(&nested.Formatter{})
	ctx = log.WithFields(ctx, map[string]interface{}{"cmd": os.Args[:1]})
	ctx = log.WithLog(ctx, logruslogger.New(ctx))

	// Enable Jaeger
	log.EnableTracing(true)
	jaegerCloser := jaeger.InitJaeger(ctx, "nsc")
	defer func() { _ = jaegerCloser.Close() }()

	logger := log.FromContext(ctx)

	// ********************************************************************************
	// Get config from environment
	// ********************************************************************************
	rootConf := &config.Config{}
	if err := envconfig.Usage("nsm", rootConf); err != nil {
		logger.Fatal(err)
	}
	if err := envconfig.Process("nsm", rootConf); err != nil {
		logger.Fatalf("error processing rootConf from env: %+v", err)
	}
	setLogLevel(rootConf.LogLevel)
	logger.Infof("rootConf: %+v", rootConf)

	// ********************************************************************************
	// Get a x509Source
	// ********************************************************************************
	source, err := workloadapi.NewX509Source(ctx)
	if err != nil {
		logger.Fatalf("error getting x509 source: %v", err.Error())
	}
	var svid *x509svid.SVID
	svid, err = source.GetX509SVID()
	if err != nil {
		logger.Fatalf("error getting x509 svid: %v", err.Error())
	}
	logger.Infof("sVID: %q", svid.ID)

	// ********************************************************************************
	// Dial to NSManager
	// ********************************************************************************
	dialCtx, cancel := context.WithTimeout(ctx, rootConf.DialTimeout)
	defer cancel()

	logger.Infof("NSC: Connecting to Network Service Manager %v", rootConf.ConnectTo.String())
	cc, err := grpc.DialContext(
		dialCtx,
		grpcutils.URLToTarget(&rootConf.ConnectTo),
		append(opentracing.WithTracingDial(),
			grpcfd.WithChainStreamInterceptor(),
			grpcfd.WithChainUnaryInterceptor(),
			grpc.WithDefaultCallOptions(
				grpc.PerRPCCredentials(token.NewPerRPCCredentials(spiffejwt.TokenGeneratorFunc(source, rootConf.MaxTokenLifetime))),
			),
			grpc.WithTransportCredentials(
				grpcfd.TransportCredentials(
					credentials.NewTLS(
						tlsconfig.MTLSClientConfig(source, source, tlsconfig.AuthorizeAny()),
					),
				),
			))...,
	)
	if err != nil {
		logger.Fatalf("failed dial to NSMgr: %v", err.Error())
	}
	// ********************************************************************************
	// Create Network Service Manager nsmClient
	// ********************************************************************************
	nsmClient := chain.NewNetworkServiceClient(
		updatepath.NewClient(rootConf.Name),
		begin.NewClient(),
		sriovtoken.NewClient(),
		mechanisms.NewClient(map[string]networkservice.NetworkServiceClient{
			vfiomech.MECHANISM:   chain.NewNetworkServiceClient(vfio.NewClient()),
			kernelmech.MECHANISM: chain.NewNetworkServiceClient(kernel.NewClient()),
		}),
		dnscontext.NewClient(dnscontext.WithChainContext(ctx)),
		authorize.NewClient(),
		sendfd.NewClient(),
		networkservice.NewNetworkServiceClient(cc),
	)

	// ********************************************************************************
	// Create Network Service Manager nsmClient
	// ********************************************************************************

	// ********************************************************************************
	// Initiate connections
	// ********************************************************************************
	for i := 0; i < len(rootConf.NetworkServices); i++ {
		// Update network services configs
		u := (*nsurl.NSURL)(&rootConf.NetworkServices[i])

		// Construct a request
		request := &networkservice.NetworkServiceRequest{
			Connection: &networkservice.Connection{
				Id:             fmt.Sprintf("%s-%d", rootConf.Name, i),
				NetworkService: u.NetworkService(),
				Labels:         u.Labels(),
			},
			MechanismPreferences: []*networkservice.Mechanism{
				u.Mechanism(),
			},
		}

		retryCtx, retryCancel := context.WithCancel(ctx)
		defer retryCancel()
		if rootConf.RequestTimeout != 0 {
			retryCtx, retryCancel = context.WithTimeout(ctx, rootConf.RetryTimeout)
			defer retryCancel()
		}

		for {
			requestCtx, cancelRequest := context.WithTimeout(ctx, rootConf.RequestTimeout)
			defer cancelRequest()

			resp, err := nsmClient.Request(requestCtx, request)

			if err != nil {
				logger.Errorf("failed connect to NSMgr: %v", err.Error())
				select {
				case <-retryCtx.Done():
					logger.Fatalf("failed to connect to %s after %d retries")
				default:
					time.Sleep(rootConf.RetryInterval)
				}
				continue
			}

			logger.Infof("successfully connected to %v. Response: %v", u.NetworkService(), resp)
			break
		}
	}
}

func setLogLevel(level string) {
	l, err := logrus.ParseLevel(level)
	if err != nil {
		logrus.Fatalf("invalid log level %s", level)
	}
	logrus.SetLevel(l)
}
