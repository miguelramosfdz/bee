// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/ethersphere/bee/pkg/collection/entry"
	"github.com/ethersphere/bee/pkg/encryption"
	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/manifest/jsonmanifest"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	// ManifestContentType represents content type used for noting that specific
	// file should be processed as manifest
	ManifestContentType = "application/bzz-manifest+json"
)

func (s *server) bzzDownloadHandler(w http.ResponseWriter, r *http.Request) {
	targets := r.URL.Query().Get("targets")
	r = r.WithContext(sctx.SetTargets(r.Context(), targets))
	ctx := r.Context()

	addressHex := mux.Vars(r)["address"]
	path := mux.Vars(r)["path"]

	address, err := swarm.ParseHexAddress(addressHex)
	if err != nil {
		s.Logger.Debugf("bzz download: parse address %s: %v", addressHex, err)
		s.Logger.Error("bzz download: parse address")
		jsonhttp.BadRequest(w, "invalid address")
		return
	}

	toDecrypt := len(address.Bytes()) == (swarm.HashSize + encryption.KeyLength)

	// read manifest entry
	j := joiner.NewSimpleJoiner(s.Storer)
	buf := bytes.NewBuffer(nil)
	_, err = file.JoinReadAll(ctx, j, address, buf, toDecrypt)
	if err != nil {
		s.Logger.Debugf("bzz download: read entry %s: %v", address, err)
		s.Logger.Errorf("bzz download: read entry %s", address)
		jsonhttp.NotFound(w, nil)
		return
	}
	e := &entry.Entry{}
	err = e.UnmarshalBinary(buf.Bytes())
	if err != nil {
		s.Logger.Debugf("bzz download: unmarshal entry %s: %v", address, err)
		s.Logger.Errorf("bzz download: unmarshal entry %s", address)
		jsonhttp.InternalServerError(w, "error unmarshaling entry")
		return
	}

	// read metadata
	buf = bytes.NewBuffer(nil)
	_, err = file.JoinReadAll(ctx, j, e.Metadata(), buf, toDecrypt)
	if err != nil {
		s.Logger.Debugf("bzz download: read metadata %s: %v", address, err)
		s.Logger.Errorf("bzz download: read metadata %s", address)
		jsonhttp.NotFound(w, nil)
		return
	}
	metadata := &entry.Metadata{}
	err = json.Unmarshal(buf.Bytes(), metadata)
	if err != nil {
		s.Logger.Debugf("bzz download: unmarshal metadata %s: %v", address, err)
		s.Logger.Errorf("bzz download: unmarshal metadata %s", address)
		jsonhttp.InternalServerError(w, "error unmarshaling metadata")
		return
	}

	// we are expecting manifest Mime type here
	if ManifestContentType != metadata.MimeType {
		s.Logger.Debugf("bzz download: not manifest %s: %v", address, err)
		s.Logger.Error("bzz download: not manifest")
		jsonhttp.BadRequest(w, "not manifest")
		return
	}

	// read manifest content
	buf = bytes.NewBuffer(nil)
	_, err = file.JoinReadAll(ctx, j, e.Reference(), buf, toDecrypt)
	if err != nil {
		s.Logger.Debugf("bzz download: data join %s: %v", address, err)
		s.Logger.Errorf("bzz download: data join %s", address)
		jsonhttp.NotFound(w, nil)
		return
	}
	manifest := jsonmanifest.NewManifest()
	err = manifest.UnmarshalBinary(buf.Bytes())
	if err != nil {
		s.Logger.Debugf("bzz download: unmarshal manifest %s: %v", address, err)
		s.Logger.Errorf("bzz download: unmarshal manifest %s", address)
		jsonhttp.InternalServerError(w, "error unmarshaling manifest")
		return
	}

	me, err := manifest.Entry(path)
	if err != nil {
		s.Logger.Debugf("bzz download: invalid path %s/%s: %v", address, path, err)
		s.Logger.Error("bzz download: invalid path")
		jsonhttp.BadRequest(w, "invalid path address")
		return
	}

	manifestEntryAddress := me.Reference()

	var additionalHeaders http.Header

	// copy header from manifest
	if me.Header() != nil {
		additionalHeaders = me.Header().Clone()
	} else {
		additionalHeaders = http.Header{}
	}

	// include filename
	if me.Name() != "" {
		additionalHeaders.Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", me.Name()))
	}

	// read file entry
	buf = bytes.NewBuffer(nil)
	_, err = file.JoinReadAll(ctx, j, manifestEntryAddress, buf, toDecrypt)
	if err != nil {
		s.Logger.Debugf("bzz download: read file entry %s: %v", address, err)
		s.Logger.Errorf("bzz download: read file entry %s", address)
		jsonhttp.NotFound(w, nil)
		return
	}
	fe := &entry.Entry{}
	err = fe.UnmarshalBinary(buf.Bytes())
	if err != nil {
		s.Logger.Debugf("bzz download: unmarshal file entry %s: %v", address, err)
		s.Logger.Errorf("bzz download: unmarshal file entry %s", address)
		jsonhttp.InternalServerError(w, "error unmarshaling file entry")
		return
	}

	fileEntryAddress := fe.Reference()

	s.downloadHandler(w, r, fileEntryAddress, additionalHeaders)
}
