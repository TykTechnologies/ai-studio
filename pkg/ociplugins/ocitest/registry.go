// Package ocitest provides an in-memory OCI distribution registry for tests
// that exercise the real ORAS fetcher (and signature verifiers) end to end.
//
// It serves the read side of the distribution API over plain HTTP on
// 127.0.0.1, which ociplugins treats as an insecure (HTTP) registry:
// manifests and blobs (GET and HEAD) and the referrers API.
package ocitest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// PluginArtifactType is the artifact type publish-plugin.sh gives plugin
// manifests; the fetcher does not check it.
const PluginArtifactType = "application/vnd.tyk.plugin.v1"

// PluginLayerMediaType is the media type of a plugin binary layer.
const PluginLayerMediaType = "application/vnd.tyk.plugin.layer.v1+binary"

type manifestEntry struct {
	mediaType string
	data      []byte
}

// Registry is an in-memory OCI registry served by an httptest server.
type Registry struct {
	server *httptest.Server

	mu        sync.Mutex
	blobs     map[digest.Digest][]byte
	manifests map[string]map[string]manifestEntry // repo -> tag or digest -> manifest
	// Requests counts requests by "METHOD kind" (kind is manifests, blobs or
	// referrers), so tests can assert which endpoints were used.
	requests map[string]int
	// username and password, when set, are required as basic auth.
	username, password string
	// noReferrersAPI makes the referrers endpoint answer 404, as registries
	// without the OCI 1.1 referrers API do; clients then fall back to the
	// referrers tag schema (an index tagged sha256-<hex>).
	noReferrersAPI bool
}

// RequireBasicAuth makes every request after it need these basic-auth
// credentials, answering 401 with a Basic challenge otherwise.
func (r *Registry) RequireBasicAuth(username, password string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.username, r.password = username, password
}

// DisableReferrersAPI makes the registry behave like one without the
// referrers API.
func (r *Registry) DisableReferrersAPI() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.noReferrersAPI = true
}

// NewRegistry starts a registry that is shut down when the test ends. It
// listens on loopback, an internal address, so it sets
// ALLOW_INTERNAL_NETWORK_ACCESS=true for the test, as a deployment with an
// internal registry must.
func NewRegistry(t testing.TB) *Registry {
	t.Helper()
	t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "true")
	r := &Registry{
		blobs:     make(map[digest.Digest][]byte),
		manifests: make(map[string]map[string]manifestEntry),
		requests:  make(map[string]int),
	}
	r.server = httptest.NewServer(http.HandlerFunc(r.serve))
	t.Cleanup(r.server.Close)
	return r
}

// Host returns the registry's host:port, for use in oci:// references.
func (r *Registry) Host() string {
	return strings.TrimPrefix(r.server.URL, "http://")
}

// Requests returns how many requests of the given method reached the given
// endpoint kind ("manifests", "blobs" or "referrers").
func (r *Registry) Requests(method, kind string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.requests[method+" "+kind]
}

// PushBlob stores a blob and returns its descriptor.
func (r *Registry) PushBlob(mediaType string, data []byte) ocispec.Descriptor {
	d := digest.FromBytes(data)
	r.mu.Lock()
	r.blobs[d] = append([]byte(nil), data...)
	r.mu.Unlock()
	return ocispec.Descriptor{MediaType: mediaType, Digest: d, Size: int64(len(data))}
}

// PushManifest stores a manifest (or index) under its digest and, when tag
// is non-empty, under the tag. It returns the manifest's descriptor.
func (r *Registry) PushManifest(repo, tag, mediaType string, data []byte) ocispec.Descriptor {
	d := digest.FromBytes(data)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.manifests[repo] == nil {
		r.manifests[repo] = make(map[string]manifestEntry)
	}
	entry := manifestEntry{mediaType: mediaType, data: append([]byte(nil), data...)}
	r.manifests[repo][d.String()] = entry
	if tag != "" {
		r.manifests[repo][tag] = entry
	}
	return ocispec.Descriptor{MediaType: mediaType, Digest: d, Size: int64(len(data))}
}

// PushPlugin pushes a single-platform plugin manifest whose first layer is
// binary, tagged with tag (if non-empty), and returns its descriptor with
// the platform set. platform is "os/arch".
func (r *Registry) PushPlugin(repo, tag, platform string, binary []byte) ocispec.Descriptor {
	osName, arch, _ := strings.Cut(platform, "/")
	config := r.PushBlob(ocispec.MediaTypeImageConfig,
		[]byte(fmt.Sprintf(`{"os":%q,"arch":%q,"version":"1.0.0"}`, osName, arch)))
	layer := r.PushBlob(PluginLayerMediaType, binary)
	manifest := ocispec.Manifest{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: PluginArtifactType,
		Config:       config,
		Layers:       []ocispec.Descriptor{layer},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		panic(err)
	}
	desc := r.PushManifest(repo, tag, ocispec.MediaTypeImageManifest, data)
	desc.Platform = &ocispec.Platform{OS: osName, Architecture: arch}
	return desc
}

// PushIndex pushes an image index over the given platform manifests,
// tagged with tag (if non-empty), and returns its descriptor.
func (r *Registry) PushIndex(repo, tag string, manifests ...ocispec.Descriptor) ocispec.Descriptor {
	index := ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: manifests,
	}
	data, err := json.Marshal(index)
	if err != nil {
		panic(err)
	}
	return r.PushManifest(repo, tag, ocispec.MediaTypeImageIndex, data)
}

func (r *Registry) serve(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	username, password := r.username, r.password
	r.mu.Unlock()
	if username != "" {
		if u, p, ok := req.BasicAuth(); !ok || u != username || p != password {
			w.Header().Set("WWW-Authenticate", `Basic realm="ocitest"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}
	if req.URL.Path == "/v2/" || req.URL.Path == "/v2" {
		w.WriteHeader(http.StatusOK)
		return
	}
	path := strings.TrimPrefix(req.URL.Path, "/v2/")
	for _, kind := range []string{"manifests", "blobs", "referrers"} {
		sep := "/" + kind + "/"
		i := strings.LastIndex(path, sep)
		if i <= 0 {
			continue
		}
		repo, ref := path[:i], path[i+len(sep):]
		r.mu.Lock()
		r.requests[req.Method+" "+kind]++
		r.mu.Unlock()
		switch kind {
		case "manifests":
			r.serveManifest(w, req, repo, ref)
		case "blobs":
			r.serveBlob(w, req, ref)
		case "referrers":
			r.serveReferrers(w, req, repo, ref)
		}
		return
	}
	http.NotFound(w, req)
}

func (r *Registry) serveManifest(w http.ResponseWriter, req *http.Request, repo, ref string) {
	r.mu.Lock()
	entry, ok := r.manifests[repo][ref]
	r.mu.Unlock()
	if !ok {
		http.NotFound(w, req)
		return
	}
	writeContent(w, req, entry.mediaType, entry.data)
}

func (r *Registry) serveBlob(w http.ResponseWriter, req *http.Request, ref string) {
	r.mu.Lock()
	data, ok := r.blobs[digest.Digest(ref)]
	r.mu.Unlock()
	if !ok {
		http.NotFound(w, req)
		return
	}
	writeContent(w, req, "application/octet-stream", data)
}

// serveReferrers answers the referrers API from the manifests whose subject
// is the given digest, filtered by the artifactType query parameter.
func (r *Registry) serveReferrers(w http.ResponseWriter, req *http.Request, repo, subject string) {
	r.mu.Lock()
	noAPI := r.noReferrersAPI
	r.mu.Unlock()
	if noAPI {
		http.NotFound(w, req)
		return
	}
	filter := req.URL.Query().Get("artifactType")
	index := ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: []ocispec.Descriptor{},
	}
	r.mu.Lock()
	for ref, entry := range r.manifests[repo] {
		if !strings.HasPrefix(ref, "sha256:") {
			continue // tags alias digests already listed
		}
		var m struct {
			ArtifactType string              `json:"artifactType"`
			Config       ocispec.Descriptor  `json:"config"`
			Subject      *ocispec.Descriptor `json:"subject"`
			Annotations  map[string]string   `json:"annotations"`
		}
		if json.Unmarshal(entry.data, &m) != nil || m.Subject == nil || m.Subject.Digest.String() != subject {
			continue
		}
		artifactType := m.ArtifactType
		if artifactType == "" {
			artifactType = m.Config.MediaType
		}
		if filter != "" && artifactType != filter {
			continue
		}
		index.Manifests = append(index.Manifests, ocispec.Descriptor{
			MediaType:    entry.mediaType,
			ArtifactType: artifactType,
			Digest:       digest.Digest(ref),
			Size:         int64(len(entry.data)),
			Annotations:  m.Annotations,
		})
	}
	r.mu.Unlock()
	data, err := json.Marshal(index)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if filter != "" {
		w.Header().Set("OCI-Filters-Applied", "artifactType")
	}
	writeContent(w, req, ocispec.MediaTypeImageIndex, data)
}

func writeContent(w http.ResponseWriter, req *http.Request, mediaType string, data []byte) {
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Docker-Content-Digest", digest.FromBytes(data).String())
	w.Header().Set("Content-Length", fmt.Sprint(len(data)))
	if req.Method == http.MethodHead {
		return
	}
	_, _ = bytes.NewReader(data).WriteTo(w)
}
