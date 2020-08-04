// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/manifest/mantaray"
)

const (
	contentTypeHeader = "Content-Type"
	contentTypeTar    = "application/x-tar"
)

type toEncryptContextKey struct{}

// dirUploadHandler uploads a directory supplied as a tar in an HTTP request
func (s *server) dirUploadHandler(w http.ResponseWriter, r *http.Request) {
	ctx, err := validateRequest(r)
	if err != nil {
		s.Logger.Errorf("dir upload, validate request")
		s.Logger.Debugf("dir upload, validate request err: %v", err)
		jsonhttp.BadRequest(w, "could not validate request")
		return
	}

	reference, err := storeDir(ctx, r.Body, s.Storer, s.Logger)
	if err != nil {
		s.Logger.Errorf("dir upload, store dir")
		s.Logger.Debugf("dir upload, store dir err: %v", err)
		jsonhttp.InternalServerError(w, "could not store dir")
		return
	}

	jsonhttp.OK(w, fileUploadResponse{
		Reference: reference,
	})
}

// validateRequest validates an HTTP request for a directory to be uploaded
// it returns a context based on the given request
func validateRequest(r *http.Request) (context.Context, error) {
	ctx := r.Context()
	if r.Body == http.NoBody {
		return nil, errors.New("request has no body")
	}
	contentType := r.Header.Get(contentTypeHeader)
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, err
	}
	if mediaType != contentTypeTar {
		return nil, errors.New("content-type not set to tar")
	}
	toEncrypt := strings.ToLower(r.Header.Get(EncryptHeader)) == "true"
	return context.WithValue(ctx, toEncryptContextKey{}, toEncrypt), nil
}

// storeDir stores all files recursively contained in the directory given as a tar
// it returns the hash for the uploaded manifest corresponding to the uploaded dir
func storeDir(ctx context.Context, reader io.ReadCloser, s storage.Storer, logger logging.Logger) (swarm.Address, error) {
	v := ctx.Value(toEncryptContextKey{})
	toEncrypt, _ := v.(bool) // default is false

	ls := &manifestLoadSaver{s, toEncrypt}

	dirManifest := mantaray.New()

	// set up HTTP body reader
	tarReader := tar.NewReader(reader)
	defer reader.Close()

	filesAdded := 0

	// iterate through the files in the supplied tar
	for {
		fileHeader, err := tarReader.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return swarm.ZeroAddress, fmt.Errorf("read tar stream error: %w", err)
		}

		filePath := fileHeader.Name

		// only store regular files
		if !fileHeader.FileInfo().Mode().IsRegular() {
			logger.Warningf("skipping file upload for %s as it is not a regular file", filePath)
			continue
		}

		fileName := fileHeader.FileInfo().Name()
		contentType := mime.TypeByExtension(filepath.Ext(fileHeader.Name))

		// upload file
		fileInfo := &fileUploadInfo{
			name:        fileName,
			size:        fileHeader.FileInfo().Size(),
			contentType: contentType,
			reader:      tarReader,
		}
		fileReference, err := storeFile(ctx, fileInfo, s)
		if err != nil {
			return swarm.ZeroAddress, fmt.Errorf("store dir file error: %w", err)
		}
		logger.Tracef("uploaded dir file %v with reference %v", filePath, fileReference)

		// add file entry to dir manifest
		err = dirManifest.Add([]byte(filePath), fileReference.Bytes(), ls)
		if err != nil {
			return swarm.ZeroAddress, fmt.Errorf("add to manifest error: %w", err)
		}

		filesAdded++
	}

	// check if files were uploaded through the manifest
	if filesAdded == 0 {
		return swarm.ZeroAddress, fmt.Errorf("no files in tar")
	}

	// save manifest
	err := dirManifest.Save(ls)
	if err != nil {
		return swarm.ZeroAddress, fmt.Errorf("store manifest error: %w", err)
	}

	manifestReference := swarm.NewAddress(dirManifest.Reference())

	return manifestReference, nil
}
