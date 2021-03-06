// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pusher

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/pushsync"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/topology"
)

type Service struct {
	storer            storage.Storer
	pushSyncer        pushsync.PushSyncer
	logger            logging.Logger
	tagg              *tags.Tags
	metrics           metrics
	quit              chan struct{}
	chunksWorkerQuitC chan struct{}
}

var retryInterval = 10 * time.Second // time interval between retries

func New(storer storage.Storer, peerSuggester topology.ClosestPeerer, pushSyncer pushsync.PushSyncer, tagger *tags.Tags, logger logging.Logger) *Service {
	service := &Service{
		storer:            storer,
		pushSyncer:        pushSyncer,
		tagg:              tagger,
		logger:            logger,
		metrics:           newMetrics(),
		quit:              make(chan struct{}),
		chunksWorkerQuitC: make(chan struct{}),
	}
	go service.chunksWorker()
	return service
}

// chunksWorker is a loop that keeps looking for chunks that are locally uploaded ( by monitoring pushIndex )
// and pushes them to the closest peer and get a receipt.
func (s *Service) chunksWorker() {
	var chunks <-chan swarm.Chunk
	var unsubscribe func()
	// timer, initially set to 0 to fall through select case on timer.C for initialisation
	timer := time.NewTimer(0)
	defer timer.Stop()
	defer close(s.chunksWorkerQuitC)
	chunksInBatch := -1
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-s.quit
		cancel()
	}()

	sem := make(chan struct{}, 10)
	inflight := make(map[string]struct{})
	var mtx sync.Mutex

LOOP:
	for {
		select {
		// handle incoming chunks
		case ch, more := <-chunks:
			// if no more, set to nil, reset timer to 0 to finalise batch immediately
			if !more {
				chunks = nil
				var dur time.Duration
				if chunksInBatch == 0 {
					dur = 500 * time.Millisecond
				}
				timer.Reset(dur)
				break
			}

			// postpone a retry only after we've finished processing everything in index
			timer.Reset(retryInterval)
			chunksInBatch++
			s.metrics.TotalChunksToBeSentCounter.Inc()
			select {
			case sem <- struct{}{}:
			case <-s.quit:
				if unsubscribe != nil {
					unsubscribe()
				}
				return
			}
			mtx.Lock()
			if _, ok := inflight[ch.Address().String()]; ok {
				mtx.Unlock()
				<-sem
				continue
			}

			inflight[ch.Address().String()] = struct{}{}
			mtx.Unlock()

			go func(ctx context.Context, ch swarm.Chunk) {
				var err error
				defer func() {
					if err == nil {
						// only print this if there was no error while sending the chunk
						s.logger.Tracef("pusher pushed chunk %s", ch.Address().String())
					}
					mtx.Lock()
					delete(inflight, ch.Address().String())
					mtx.Unlock()
					<-sem
				}()
				// Later when we process receipt, get the receipt and process it
				// for now ignoring the receipt and checking only for error
				_, err = s.pushSyncer.PushChunkToClosest(ctx, ch)
				if err != nil {
					if !errors.Is(err, topology.ErrNotFound) {
						s.logger.Debugf("pusher: error while sending chunk or receiving receipt: %v", err)
					}
					return
				}
				s.setChunkAsSynced(ctx, ch)
			}(ctx, ch)
		case <-timer.C:
			// initially timer is set to go off as well as every time we hit the end of push index
			startTime := time.Now()

			// if subscribe was running, stop it
			if unsubscribe != nil {
				unsubscribe()
			}

			// and start iterating on Push index from the beginning
			chunks, unsubscribe = s.storer.SubscribePush(ctx)

			// reset timer to go off after retryInterval
			timer.Reset(retryInterval)
			s.metrics.MarkAndSweepTimer.Observe(time.Since(startTime).Seconds())

		case <-s.quit:
			if unsubscribe != nil {
				unsubscribe()
			}
			break LOOP
		}
	}

	// wait for all pending push operations to terminate
	closeC := make(chan struct{})
	go func() {
		defer func() { close(closeC) }()
		for i := 0; i < cap(sem); i++ {
			sem <- struct{}{}
		}
	}()

	select {
	case <-closeC:
	case <-time.After(2 * time.Second):
		s.logger.Warning("pusher shutting down with pending operations")
	}
}

func (s *Service) setChunkAsSynced(ctx context.Context, ch swarm.Chunk) {
	if err := s.storer.Set(ctx, storage.ModeSetSyncPush, ch.Address()); err != nil {
		s.logger.Errorf("pusher: error setting chunk as synced: %v", err)
		s.metrics.ErrorSettingChunkToSynced.Inc()
	}
	t, err := s.tagg.Get(ch.TagID())
	if err == nil && t != nil {
		t.Inc(tags.StateSynced)
	}
}

func (s *Service) Close() error {
	close(s.quit)

	// Wait for chunks worker to finish
	select {
	case <-s.chunksWorkerQuitC:
	case <-time.After(3 * time.Second):
	}
	return nil
}
