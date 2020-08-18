// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package encryption

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"golang.org/x/crypto/sha3"
)

func hashFunc() hash.Hash {
	return sha3.NewLegacyKeccak256()
}

type decryptingGetter struct {
	storage.Getter
}

func NewDecryptingGetter(s storage.Getter) storage.Getter {
	return &decryptingGetter{s}
}

func (s *decryptingGetter) Get(ctx context.Context, mode storage.ModeGet, addr swarm.Address) (ch swarm.Chunk, err error) {
	switch l := len(addr.Bytes()); l {
	case 32:
		// normal, unencrypted content
		fmt.Println("short ref")
		return s.Getter.Get(ctx, mode, addr)

	case 64:
		// encrypted reference
		fmt.Println("long ref")
		ref := addr.Bytes()
		address := swarm.NewAddress(ref[:32])
		ch, err := s.Getter.Get(ctx, mode, address)
		if err != nil {
			return nil, err
		}

		decryptionKey := make([]byte, KeyLength)
		copy(decryptionKey, ref[32:])
		if len(ref[32:]) != 32 {
			panic(1)
		}
		d, err := decryptChunkData(ch.Data(), decryptionKey)
		if err != nil {
			return nil, err
		}
		return swarm.NewChunk(address, d), nil

	default:
		return nil, storage.ErrReferenceLength
	}
}

type Encrypting struct {
}

func NewEncrypting() *Encrypting {
	return &Encrypting{}
}

func (s *Encrypting) Encrypt(b []byte) ([]byte, Key, error) {
	return encryptChunkData(b)
}

func encryptChunkData(chunkData []byte) ([]byte, Key, error) {
	if len(chunkData) < 8 {
		return nil, nil, fmt.Errorf("invalid data, min length 8 got %v", len(chunkData))
	}

	key, encryptedSpan, encryptedData, err := encrypt(chunkData)
	if err != nil {
		return nil, nil, err
	}
	c := make([]byte, len(encryptedSpan)+len(encryptedData))
	copy(c[:8], encryptedSpan)
	copy(c[8:], encryptedData)
	return c, key, nil
}

func encrypt(chunkData []byte) (Key, []byte, []byte, error) {
	key := GenerateRandomKey(KeyLength)
	encryptedSpan, err := newSpanEncryption(key).Encrypt(chunkData[:8])
	if err != nil {
		return nil, nil, nil, err
	}
	encryptedData, err := newDataEncryption(key).Encrypt(chunkData[8:])
	if err != nil {
		return nil, nil, nil, err
	}
	return key, encryptedSpan, encryptedData, nil
}

func decryptChunkData(chunkData []byte, encryptionKey Key) ([]byte, error) {
	if len(chunkData) < 8 {
		return nil, fmt.Errorf("invalid ChunkData, min length 8 got %v", len(chunkData))
	}

	decryptedSpan, decryptedData, err := decrypt(chunkData, encryptionKey)
	if err != nil {
		return nil, err
	}

	// removing extra bytes which were just added for padding
	length := binary.LittleEndian.Uint64(decryptedSpan)
	refSize := int64(swarm.HashSize + KeyLength)
	for length > swarm.ChunkSize {
		length = length + (swarm.ChunkSize - 1)
		length = length / swarm.ChunkSize
		length *= uint64(refSize)
	}

	c := make([]byte, length+8)
	copy(c[:8], decryptedSpan)
	copy(c[8:], decryptedData[:length])
	fmt.Println("decrypted span", binary.LittleEndian.Uint64(decryptedSpan))

	return c, nil
}

func decrypt(chunkData []byte, key Key) ([]byte, []byte, error) {
	encryptedSpan, err := newSpanEncryption(key).Encrypt(chunkData[:8])
	if err != nil {
		return nil, nil, err
	}
	encryptedData, err := newDataEncryption(key).Encrypt(chunkData[8:])
	if err != nil {
		return nil, nil, err
	}
	return encryptedSpan, encryptedData, nil
}

func newSpanEncryption(key Key) *Encryption {
	refSize := int64(swarm.HashSize + KeyLength)
	return New(key, 0, uint32(swarm.ChunkSize/refSize), sha3.NewLegacyKeccak256)
}

func newDataEncryption(key Key) *Encryption {
	return New(key, int(swarm.ChunkSize), 0, sha3.NewLegacyKeccak256)
}
