package ociplugins

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// fakeFetcher serves blobs from memory, standing in for a remote repository.
type fakeFetcher struct {
	blobs map[digest.Digest][]byte
}

func (f *fakeFetcher) Fetch(_ context.Context, target ocispec.Descriptor) (io.ReadCloser, error) {
	data, ok := f.blobs[target.Digest]
	if !ok {
		return nil, errors.New("blob not found")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func blobDescriptor(data []byte) ocispec.Descriptor {
	sum := sha256.Sum256(data)
	return ocispec.Descriptor{
		MediaType: "application/vnd.tyk.plugin.layer.v1",
		Digest:    digest.NewDigestFromBytes(digest.SHA256, sum[:]),
		Size:      int64(len(data)),
	}
}

// content.FetchAll rejects anything over 32MB since oras-go v2.6.0, but plugin
// binaries are routinely larger (advanced-llm-cache is 37MB).
func TestFetchLayerAcceptsBinaryOverFetchAllCap(t *testing.T) {
	data := bytes.Repeat([]byte("tyk-plugin-binary-"), 2_500_000) // ~45MB
	if len(data) <= 32*1024*1024 {
		t.Fatalf("test blob must exceed the 32MB FetchAll cap, got %d bytes", len(data))
	}
	desc := blobDescriptor(data)
	f := &ORASFetcher{config: &OCIConfig{}}
	fetcher := &fakeFetcher{blobs: map[digest.Digest][]byte{desc.Digest: data}}

	got, err := f.fetchLayer(context.Background(), fetcher, desc)
	if err != nil {
		t.Fatalf("fetchLayer: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Error("fetched layer does not match the stored blob")
	}
}

func TestFetchLayerEnforcesSizeLimit(t *testing.T) {
	data := []byte("small-binary")
	desc := blobDescriptor(data)
	f := &ORASFetcher{config: &OCIConfig{MaxPluginSize: 4}}
	fetcher := &fakeFetcher{blobs: map[digest.Digest][]byte{desc.Digest: data}}

	if _, err := f.fetchLayer(context.Background(), fetcher, desc); err == nil {
		t.Error("layer exceeding MaxPluginSize must be rejected")
	}
}

// Lifting the cap must not lift verification: a registry that serves content
// not matching the descriptor is still rejected.
func TestFetchLayerVerifiesContent(t *testing.T) {
	desc := blobDescriptor([]byte("expected-binary"))
	f := &ORASFetcher{config: &OCIConfig{}}

	t.Run("mismatched digest", func(t *testing.T) {
		fetcher := &fakeFetcher{blobs: map[digest.Digest][]byte{desc.Digest: []byte("tampered-binary")}}
		if _, err := f.fetchLayer(context.Background(), fetcher, desc); err == nil {
			t.Error("content not matching the descriptor digest must be rejected")
		}
	})

	t.Run("truncated content", func(t *testing.T) {
		fetcher := &fakeFetcher{blobs: map[digest.Digest][]byte{desc.Digest: []byte("expected")}}
		if _, err := f.fetchLayer(context.Background(), fetcher, desc); err == nil {
			t.Error("content shorter than the descriptor size must be rejected")
		}
	})
}
