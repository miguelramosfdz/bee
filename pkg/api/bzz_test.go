// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mime"
	"net/http"
	"strings"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/collection/entry"
	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/logging"
	smock "github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/manifest/mantaray"
)

func TestBzz(t *testing.T) {
	var (
		bzzDownloadResource = func(addr, path string) string { return "/bzz/" + addr + "/" + path }
		storer              = smock.NewStorer()
		sp                  = splitter.NewSimpleSplitter(storer)
		ls                  = &api.ManifestLoadSaver{storer, false}
		client              = newTestServer(t, testServerOptions{
			Storer: storer,
			Tags:   tags.NewTags(),
			Logger: logging.New(ioutil.Discard, 5),
		})
	)

	t.Run("download-file-by-path", func(t *testing.T) {
		fileName := "sample.html"
		filePath := "test/" + fileName
		missingFilePath := "test/missing"
		sampleHtml := `<!DOCTYPE html>
		<html>
		<body>
	
		<h1>My First Heading</h1>
	
		<p>My first paragraph.</p>
	
		</body>
		</html>`

		var err error
		var fileContentReference swarm.Address
		var fileReference swarm.Address
		var manifestFileReference swarm.Address

		// save file

		fileContentReference, err = file.SplitWriteAll(context.Background(), sp, strings.NewReader(sampleHtml), int64(len(sampleHtml)), false)
		if err != nil {
			t.Fatal(err)
		}

		fileMetadata := entry.NewMetadata(fileName)
		fileMetadata.MimeType = "text/html; charset=utf-8"
		fileMetadataBytes, err := json.Marshal(fileMetadata)
		if err != nil {
			t.Fatal(err)
		}

		fileMetadataReference, err := file.SplitWriteAll(context.Background(), sp, bytes.NewReader(fileMetadataBytes), int64(len(fileMetadataBytes)), false)
		if err != nil {
			t.Fatal(err)
		}

		fe := entry.New(fileContentReference, fileMetadataReference)
		fileEntryBytes, err := fe.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		fileReference, err = file.SplitWriteAll(context.Background(), sp, bytes.NewReader(fileEntryBytes), int64(len(fileEntryBytes)), false)
		if err != nil {
			t.Fatal(err)
		}

		// save manifest

		manifest := mantaray.New()

		err = manifest.Add([]byte(filePath), fileReference.Bytes(), ls)
		if err != nil {
			t.Fatal(err)
		}

		err = manifest.Save(ls)
		if err != nil {
			t.Fatal(err)
		}

		fr := swarm.NewAddress(manifest.Reference())

		m := entry.NewMetadata(fileName)
		m.MimeType = api.ManifestBinaryContentType
		metadataBytes, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}

		mr, err := file.SplitWriteAll(context.Background(), sp, bytes.NewReader(metadataBytes), int64(len(metadataBytes)), false)
		if err != nil {
			t.Fatal(err)
		}

		// now join both references (mr,fr) to create an entry and store it.
		newEntry := entry.New(fr, mr)
		manifestFileEntryBytes, err := newEntry.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}

		manifestFileReference, err = file.SplitWriteAll(context.Background(), sp, bytes.NewReader(manifestFileEntryBytes), int64(len(manifestFileEntryBytes)), false)
		if err != nil {
			t.Fatal(err)
		}

		// read file from manifest path

		rcvdHeader := jsonhttptest.ResponseDirectCheckBinaryResponse(t, client, http.MethodGet, bzzDownloadResource(manifestFileReference.String(), filePath), nil, http.StatusOK, []byte(sampleHtml), nil)
		cd := rcvdHeader.Get("Content-Disposition")
		_, params, err := mime.ParseMediaType(cd)
		if err != nil {
			t.Fatal(err)
		}
		if params["filename"] != fileName {
			t.Fatal("Invalid file name detected")
		}
		if rcvdHeader.Get("ETag") != fmt.Sprintf("%q", fileContentReference) {
			t.Fatal("Invalid ETags header received")
		}
		if rcvdHeader.Get("Content-Type") != "text/html; charset=utf-8" {
			t.Fatal("Invalid content type detected")
		}

		// check on invalid path

		jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodGet, bzzDownloadResource(manifestFileReference.String(), missingFilePath), nil, http.StatusNotFound, jsonhttp.StatusResponse{
			Message: "path address not found",
			Code:    http.StatusNotFound,
		}, nil)

	})

}
