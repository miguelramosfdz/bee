// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"net/http"

	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/pingpong"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/prometheus/client_golang/prometheus"
)

type Service interface {
	http.Handler
	MustRegisterMetrics(cs ...prometheus.Collector)
}

type server struct {
	Overlay        swarm.Address
	P2P            p2p.DebugService
	Pingpong       pingpong.Interface
	TopologyDriver topology.PeerAdder
	Storer         storage.Storer
	Logger         logging.Logger
	Tracer         *tracing.Tracer
	Tags           *tags.Tags
	Accounting     accounting.Interface
	http.Handler

	metricsRegistry *prometheus.Registry
}

func New(overlay swarm.Address, p2p p2p.DebugService, pingpong pingpong.Interface, topologyDriver topology.PeerAdder, storer storage.Storer, logger logging.Logger, tracer *tracing.Tracer, tags *tags.Tags, accounting accounting.Interface) Service {
	s := &server{
		Overlay:         overlay,
		P2P:             p2p,
		Pingpong:        pingpong,
		TopologyDriver:  topologyDriver,
		Storer:          storer,
		Logger:          logger,
		Tracer:          tracer,
		Tags:            tags,
		Accounting:      accounting,
		metricsRegistry: newMetricsRegistry(),
	}

	s.setupRouting()

	return s
}
