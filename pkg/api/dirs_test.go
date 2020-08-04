// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"path"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/manifest/mantaray"
)

func TestDirs(t *testing.T) {
	var (
		dirUploadResource    = "/dirs"
		fileDownloadResource = func(addr string) string { return "/files/" + addr }
		storer               = mock.NewStorer()
		client               = newTestServer(t, testServerOptions{
			Storer: storer,
			Tags:   tags.NewTags(),
			Logger: logging.New(ioutil.Discard, 5),
		})
	)

	t.Run("empty request body", func(t *testing.T) {
		jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, dirUploadResource, bytes.NewReader(nil), http.StatusBadRequest, jsonhttp.StatusResponse{
			Message: "could not validate request",
			Code:    http.StatusBadRequest,
		}, http.Header{
			"Content-Type": {api.ContentTypeTar},
		})
	})

	t.Run("non tar file", func(t *testing.T) {
		file := bytes.NewReader([]byte("some data"))

		jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, dirUploadResource, file, http.StatusInternalServerError, jsonhttp.StatusResponse{
			Message: "could not store dir",
			Code:    http.StatusInternalServerError,
		}, http.Header{
			"Content-Type": {api.ContentTypeTar},
		})
	})

	t.Run("wrong content type", func(t *testing.T) {
		tarReader := tarFiles(t, []f{{
			data: []byte("some data"),
			name: "binary-file",
		}})

		// submit valid tar, but with wrong content-type
		jsonhttptest.ResponseDirectSendHeadersAndReceiveHeaders(t, client, http.MethodPost, dirUploadResource, tarReader, http.StatusBadRequest, jsonhttp.StatusResponse{
			Message: "could not validate request",
			Code:    http.StatusBadRequest,
		}, http.Header{
			"Content-Type": {"other"},
		})
	})

	// valid tars
	for _, tc := range []struct {
		name         string
		expectedHash string
		files        []f // files in dir for test case
	}{
		{
			name:         "non-nested files without extension",
			expectedHash: "3609d0521d34469ecbffc1d2401ce7a34c7c54bb63e8d23933ef0073015aa9e7",
			files: []f{
				{
					data:      []byte("first file data"),
					name:      "file1",
					dir:       "",
					reference: swarm.MustParseHexAddress("3c07cd2cf5c46208d69d554b038f4dce203f53ac02cb8a313a0fe1e3fe6cc3cf"),
					header: http.Header{
						"Content-Type": {""},
					},
				},
				{
					data:      []byte("second file data"),
					name:      "file2",
					dir:       "",
					reference: swarm.MustParseHexAddress("47e1a2a8f16e02da187fac791d57e6794f3e9b5d2400edd00235da749ad36683"),
					header: http.Header{
						"Content-Type": {""},
					},
				},
			},
		},
		{
			name:         "nested files with extension",
			expectedHash: "983869d469f0eab1f1bb6c2daeac1fdf476968246410b3001e59e9f2e0236da0",
			files: []f{
				{
					data:      []byte("robots text"),
					name:      "robots.txt",
					dir:       "",
					reference: swarm.MustParseHexAddress("17b96d0a800edca59aaf7e40c6053f7c4c0fb80dd2eb3f8663d51876bf350b12"),
					header: http.Header{
						"Content-Type": {"text/plain; charset=utf-8"},
					},
				},
				{
					data:      []byte("image 1"),
					name:      "1.png",
					dir:       "img",
					reference: swarm.MustParseHexAddress("3c1b3fc640e67f0595d9c1db23f10c7a2b0bdc9843b0e27c53e2ac2a2d6c4674"),
					header: http.Header{
						"Content-Type": {"image/png"},
					},
				},
				{
					data:      []byte("image 2"),
					name:      "2.png",
					dir:       "img",
					reference: swarm.MustParseHexAddress("b234ea7954cab7b2ccc5e07fe8487e932df11b2275db6b55afcbb7bad0be73fb"),
					header: http.Header{
						"Content-Type": {"image/png"},
					},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// tar all the test case files
			tarReader := tarFiles(t, tc.files)

			// verify directory tar upload response
			_, respBytes := jsonhttptest.ResponseDirectSendHeadersAndDontCheckResponse(t, client, http.MethodPost, dirUploadResource, tarReader, http.StatusOK, http.Header{
				"Content-Type": {api.ContentTypeTar},
			})
			read := bytes.NewReader(respBytes)

			// get the reference as everytime it will change because of random encryption key
			var resp api.FileUploadResponse
			err := json.NewDecoder(read).Decode(&resp)
			if err != nil {
				t.Fatal(err)
			}

			ls := &api.ManifestLoadSaver{storer, false}

			// verify manifest content
			verifyManifest := mantaray.NewNodeRef(resp.Reference.Bytes())

			// check if each file can be located and read
			for _, file := range tc.files {
				filePath := path.Join(file.dir, file.name)

				entry, err := verifyManifest.Lookup([]byte(filePath), ls)
				if err != nil {
					t.Fatal(err)
				}

				fileReference := swarm.NewAddress(entry)

				if !bytes.Equal(file.reference.Bytes(), fileReference.Bytes()) {
					t.Fatalf("expected file reference to match %x, got %x", file.reference, fileReference)
				}

				fileRespHeaders, fileRespBytes := jsonhttptest.ResponseDirectSendHeadersAndDontCheckResponse(t, client, http.MethodGet, fileDownloadResource(fileReference.String()), nil, http.StatusOK, nil)
				if !bytes.Equal(file.data, fileRespBytes) {
					t.Fatalf("expected file data to match %x, got %x", file.data, fileRespBytes)
				}

				for k := range file.header {
					v := file.header.Get(k)
					if v != "" {
						rhv := fileRespHeaders.Get(k)

						if v != rhv {
							t.Fatalf("expected file header to match %s, got %s", v, rhv)
						}
					}
				}
			}
		})
	}
}

// tarFiles receives an array of test case files and creates a new tar with those files as a collection
// it returns a bytes.Buffer which can be used to read the created tar
func tarFiles(t *testing.T, files []f) *bytes.Buffer {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	for _, file := range files {
		// create tar header and write it
		hdr := &tar.Header{
			Name: path.Join(file.dir, file.name),
			Mode: 0600,
			Size: int64(len(file.data)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}

		// write the file data to the tar
		if _, err := tw.Write(file.data); err != nil {
			t.Fatal(err)
		}
	}

	// finally close the tar writer
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	return &buf
}

// struct for dir files for test cases
type f struct {
	data      []byte
	name      string
	dir       string
	reference swarm.Address
	header    http.Header
}
