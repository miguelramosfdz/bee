// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package manifest

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/manifest/mantaray"
)

const (
	// ManifestMantarayContentType represents content type used for noting that
	// specific file should be processed as mantaray manifest.
	ManifestMantarayContentType = "application/bzz-manifest-mantaray+octet-stream"
)

type mantarayManifest struct {
	trie *mantaray.Node

	loadSaver mantaray.LoadSaver
}

// NewMantarayManifest creates a new mantaray-based manifest.
func NewMantarayManifest(
	ctx context.Context,
	encrypted bool,
	storer storage.Storer,
) (Interface, error) {
	return &mantarayManifest{
		trie:      mantaray.New(),
		loadSaver: newMantarayLoadSaver(ctx, encrypted, storer),
	}, nil
}

// NewMantarayManifestReference loads existing mantaray-based manifest.
func NewMantarayManifestReference(
	ctx context.Context,
	reference swarm.Address,
	encrypted bool,
	storer storage.Storer,
) (Interface, error) {
	return &mantarayManifest{
		trie:      mantaray.NewNodeRef(reference.Bytes()),
		loadSaver: newMantarayLoadSaver(ctx, encrypted, storer),
	}, nil
}

func (m *mantarayManifest) Type() string {
	return ManifestMantarayContentType
}

func (m *mantarayManifest) Add(path string, entry Entry) error {
	p := []byte(path)
	e := entry.Reference().Bytes()

	return m.trie.Add(p, e, m.loadSaver)
}

func (m *mantarayManifest) Remove(path string) error {
	p := []byte(path)

	err := m.trie.Remove(p, m.loadSaver)
	if err != nil {
		if errors.Is(err, mantaray.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	return nil
}

func (m *mantarayManifest) Lookup(path string) (Entry, error) {
	p := []byte(path)

	ref, err := m.trie.Lookup(p, m.loadSaver)
	if err != nil {
		return nil, ErrNotFound
	}

	address := swarm.NewAddress(ref)

	entry := NewEntry(address)

	return entry, nil
}

func (m *mantarayManifest) Store() (swarm.Address, error) {

	err := m.trie.Save(m.loadSaver)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("manifest save error: %w", err)
	}

	address := swarm.NewAddress(m.trie.Reference())

	return address, nil
}

// mantarayLoadSaver implements required interface 'mantaray.LoadSaver'
type mantarayLoadSaver struct {
	ctx       context.Context
	encrypted bool
	storer    storage.Storer
}

func newMantarayLoadSaver(
	ctx context.Context,
	encrypted bool,
	storer storage.Storer,
) *mantarayLoadSaver {
	return &mantarayLoadSaver{
		ctx:       ctx,
		encrypted: encrypted,
		storer:    storer,
	}
}

func (ls *mantarayLoadSaver) Load(ref []byte) ([]byte, error) {
	ctx := ls.ctx

	j := joiner.NewSimpleJoiner(ls.storer)

	buf := bytes.NewBuffer(nil)
	_, err := file.JoinReadAll(ctx, j, swarm.NewAddress(ref), buf, ls.encrypted)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (ls *mantarayLoadSaver) Save(data []byte) ([]byte, error) {
	ctx := ls.ctx

	sp := splitter.NewSimpleSplitter(ls.storer)

	address, err := file.SplitWriteAll(ctx, sp, bytes.NewReader(data), int64(len(data)), ls.encrypted)
	if err != nil {
		return swarm.ZeroAddress.Bytes(), err
	}

	return address.Bytes(), nil
}
