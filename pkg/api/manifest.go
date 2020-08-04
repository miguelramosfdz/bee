// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"context"

	"github.com/ethersphere/bee/pkg/collection/entry"
	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	// ManifestJSONContentType represents content type used for noting that
	// specific file should be processed as JSON manifest
	ManifestJSONContentType = "application/bzz-manifest+json"

	// ManifestBinaryContentType represents content type used for noting that
	// specific file should be processed as binary manifest
	ManifestBinaryContentType = "application/bzz-manifest+octet-stream"
)

type manifestLoadSaver struct {
	Storer    storage.Storer
	Encrypted bool
}

func (ls *manifestLoadSaver) Load(ref []byte) ([]byte, error) {
	ctx := context.Background()

	j := joiner.NewSimpleJoiner(ls.Storer)

	buf := bytes.NewBuffer(nil)
	_, err := file.JoinReadAll(ctx, j, swarm.NewAddress(ref), buf, ls.Encrypted)
	if err != nil {
		return nil, err
	}

	e := &entry.Entry{}
	err = e.UnmarshalBinary(buf.Bytes())
	if err != nil {
		return nil, err
	}

	buf = bytes.NewBuffer(nil)
	_, err = file.JoinReadAll(ctx, j, e.Reference(), buf, ls.Encrypted)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (ls *manifestLoadSaver) Save(content []byte) ([]byte, error) {
	ctx := context.Background()

	fileInfo := &fileUploadInfo{
		size:        int64(len(content)),
		contentType: ManifestBinaryContentType,
		reader:      bytes.NewReader(content),
	}

	fileReference, err := storeFile(ctx, fileInfo, ls.Storer)
	if err != nil {
		return nil, err
	}

	return fileReference.Bytes(), nil
}
