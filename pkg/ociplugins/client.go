// pkg/ociplugins/client.go
package ociplugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/rs/zerolog/log"
)

// SecurityService defines the interface for signature verification
// This allows the OCI client to use enterprise security features when available
type SecurityService interface {
	VerifySignature(ctx context.Context, ref *OCIReference, pubKeyID string) error
	VerifyBundle(ctx context.Context, ref *OCIReference, issuer, subject string) error
	// Deprecated: cosign policy files are not supported by the in-process
	// verifier; use VerifySignature.
	VerifyWithPolicy(ctx context.Context, ref *OCIReference, policyPath string) error
}

// SignatureCapability is implemented by a SecurityService that may be
// unable to verify signatures. The Community Edition service reports false:
// its VerifySignature accepts everything, so the client still loads plugins
// when signatures are required but records them as unverified (Verified and
// SignatureChecked false), and an Enterprise build sharing the cache
// verifies them again.
type SignatureCapability interface {
	VerifiesSignatures() bool
}

// pluginPuller pulls a plugin's manifest, config and binary from a registry.
// ORASFetcher implements it; tests substitute a fake.
type pluginPuller interface {
	Pull(ctx context.Context, ref *OCIReference, params *OCIPluginParams) (*PullResult, error)
}

// OCIPluginClient handles OCI-based plugin operations
type OCIPluginClient struct {
	config          *OCIConfig
	storage         *ContentStorage
	fetcher         pluginPuller
	securityService SecurityService // verifies signatures; required when signatures are required

	// Background processing
	gcCancel  context.CancelFunc
	gcStopped chan struct{}
}

// NewOCIPluginClient creates a new OCI plugin client
func NewOCIPluginClient(config *OCIConfig) (*OCIPluginClient, error) {
	if config == nil {
		config = DefaultOCIConfig()
	}

	// Initialize storage
	storage, err := NewContentStorage(config.CacheDir, config.MaxCacheSize, config.GCInterval)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Initialize ORAS fetcher
	fetcher, err := NewORASFetcher(config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize ORAS fetcher: %w", err)
	}

	client := &OCIPluginClient{
		config:    config,
		storage:   storage,
		fetcher:   fetcher,
		gcStopped: make(chan struct{}),
	}

	// Start background garbage collection if configured
	if config.GCInterval > 0 {
		ctx, cancel := context.WithCancel(context.Background())
		client.gcCancel = cancel
		go client.startGarbageCollection(ctx)
	}

	return client, nil
}

// SetSecurityService sets the security service that verifies signatures. A
// client with signatures required and no security service refuses every
// fetch ("no signature verifier configured").
func (c *OCIPluginClient) SetSecurityService(service SecurityService) {
	c.securityService = service
	log.Debug().Msg("Enterprise security service configured for OCI plugin client")
}

// SecurityService returns the security service set with SetSecurityService,
// or nil if none was set.
func (c *OCIPluginClient) SecurityService() SecurityService {
	return c.securityService
}

// signaturesEnforced reports whether fetched plugins have their signatures
// checked: signatures are required and the security service can verify them
// (not the Community Edition service, see SignatureCapability).
func (c *OCIPluginClient) signaturesEnforced() bool {
	if !c.config.RequireSignature {
		return false
	}
	if capability, ok := c.securityService.(SignatureCapability); ok && !capability.VerifiesSignatures() {
		return false
	}
	return true
}

// FetchPlugin fetches a plugin by OCI reference, verifies it, and returns local information
func (c *OCIPluginClient) FetchPlugin(ctx context.Context, ref *OCIReference, params *OCIPluginParams) (*LocalPlugin, error) {
	startTime := time.Now()

	log.Info().
		Str("reference", ref.FullReference()).
		Str("architecture", params.Architecture).
		Msg("Fetching OCI plugin")

	// Validate reference against configuration
	if err := ValidateOCIReference(ref, c.config); err != nil {
		return nil, fmt.Errorf("reference validation failed: %w", err)
	}

	// Normalize architecture if not specified
	if params.Architecture == "" {
		params.Architecture = fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
	}

	// Check if plugin already exists in cache
	var unchecked *ErrCacheUnchecked
	if c.HasPlugin(ref.Digest, params.Architecture) {
		log.Debug().
			Str("reference", ref.FullReference()).
			Str("architecture", params.Architecture).
			Msg("Plugin found in cache")

		local, err := c.getLocalPlugin(ref, params)
		if err == nil {
			return local, nil
		}
		// Fetch it again: the pull re-verifies the signature and the store
		// replaces the cached file and records its hash.
		if errors.As(err, &unchecked) {
			log.Info().
				Str("reference", ref.FullReference()).
				Str("reason", unchecked.Reason).
				Msg("Cached OCI plugin has no recorded hash (cached by an earlier release); fetching it again to verify it")
		} else {
			log.Warn().
				Err(err).
				Str("reference", ref.FullReference()).
				Msg("Cached OCI plugin failed its integrity check, fetching it again")
		}
	}

	// Fetch the plugin manifest and layers
	pulled, err := c.fetcher.Pull(ctx, ref, params)
	if err != nil {
		// A copy that merely could not be checked still loads, as it did
		// before hashes were recorded; one that failed its check never does.
		if unchecked != nil {
			log.Warn().
				Err(err).
				Str("reference", ref.FullReference()).
				Msg("Cannot fetch the OCI plugin to verify its cached copy, which has no recorded hash; loading the cached copy unverified. It is verified on the next load that reaches the registry")
			return c.unverifiedLocalPlugin(ref, params, unchecked.Path), nil
		}
		return nil, fmt.Errorf("failed to pull plugin: %w", err)
	}

	pluginConfig, binaryData := pulled.Config, pulled.Binary

	// Tag-based references carry no digest, so pin them to the manifest digest
	// the registry resolved the tag to; the cache is keyed by digest. For a
	// multi-arch tag that is the platform manifest's digest, as it was before
	// signatures were checked against the index, so existing caches stay valid.
	cacheRef := ref
	if cacheRef.Digest == "" && pulled.Manifest != nil {
		resolved := *ref
		resolved.Digest = pulled.Manifest.Digest.String()
		cacheRef = &resolved
	}

	// Verify signature if required.
	var signedDigest string
	if c.config.RequireSignature {
		var err error
		signedDigest, err = c.verifySignature(ctx, ref, pulled, params)
		if err != nil {
			return nil, err
		}
		if !c.signaturesEnforced() {
			log.Warn().
				Str("reference", ref.FullReference()).
				Msg("OCI plugin signature NOT verified: signature verification is not available in this edition; the plugin is recorded as unverified")
		}
	} else {
		log.Debug().
			Str("reference", ref.FullReference()).
			Msg("Signature verification skipped (disabled)")
	}

	// Validate architecture compatibility
	if pluginConfig != nil && pluginConfig.Arch != "" {
		if !isCompatibleArchitecture(pluginConfig.OS+"/"+pluginConfig.Arch, params.Architecture) {
			return nil, &ErrIncompatibleArchitecture{
				PluginArch:  pluginConfig.OS + "/" + pluginConfig.Arch,
				RuntimeArch: params.Architecture,
			}
		}
	}

	// Store the binary data as executable
	execPath, err := c.storage.StoreExecutable(cacheRef.Digest, params.Architecture, binaryData)
	if err != nil {
		return nil, fmt.Errorf("failed to store executable: %w", err)
	}

	// Create and store metadata
	fetchTime := time.Now()
	version := ""
	if pluginConfig != nil && pluginConfig.Version != "" {
		version = pluginConfig.Version
	}

	// Verified means the signature check ran and passed, whichever key it used.
	enforced := c.signaturesEnforced()
	binarySum := sha256.Sum256(binaryData)

	metadata := &PluginMetadata{
		Reference:    cacheRef,
		Params:       params,
		FetchTime:    fetchTime,
		Config:       pluginConfig,
		Verified:     enforced,
		Size:         int64(len(binaryData)),
		LastAccessed: fetchTime,
		Version:      version,
		BinarySHA256: hex.EncodeToString(binarySum[:]),
		// The signature check above returns early on failure, so reaching
		// here with signatures enforced means it ran and passed.
		SignatureChecked: enforced,
		SignatureKey:     params.PublicKey,
	}
	if pulled.Resolved != nil {
		metadata.ResolvedDigest = pulled.Resolved.Digest.String()
	}
	if pulled.Manifest != nil {
		metadata.ManifestDigest = pulled.Manifest.Digest.String()
	}
	if enforced {
		metadata.SignedDigest = signedDigest
	}

	if err := c.storage.StoreMetadata(cacheRef.Digest, params.Architecture, metadata); err != nil {
		log.Warn().Err(err).Msg("Failed to store plugin metadata")
	}

	// Create local plugin info
	localPlugin := &LocalPlugin{
		Reference:      cacheRef,
		Params:         params,
		ExecutablePath: execPath,
		CacheDir:       c.config.CacheDir,
		Verified:       metadata.Verified,
		FetchTime:      fetchTime,
		Config:         pluginConfig,
	}

	log.Info().
		Str("reference", ref.FullReference()).
		Str("executable_path", execPath).
		Dur("fetch_time", time.Since(startTime)).
		Msg("OCI plugin fetched successfully")

	return localPlugin, nil
}

// verifySignature checks the signature of the pulled plugin and returns the
// digest whose signature verified. Every failure is an
// ErrSignatureVerificationFailed.
//
// The signature is checked against the top-level digest the reference
// resolved to: the index for a multi-arch plugin, which publish-plugin.sh
// signs. Plugins signed before that, or by hand, may be signed on the
// platform manifest instead, so when the index's signature does not verify,
// the platform manifest that was pulled is checked with the same key. That
// digest comes from the index content (fetched by the index digest), never
// from the tag or a second resolution, and another platform's manifest is
// never consulted. Both checks use the digests as pulled, not the tag: a tag
// moved between the pull and the check would otherwise get a signed image
// verified and an unsigned one stored.
func (c *OCIPluginClient) verifySignature(ctx context.Context, ref *OCIReference, pulled *PullResult, params *OCIPluginParams) (string, error) {
	fail := func(reason string) error {
		return &ErrSignatureVerificationFailed{Reference: ref.FullReference(), Reason: reason}
	}
	if c.securityService == nil {
		return "", fail("no signature verifier configured")
	}

	var resolved string
	if pulled.Resolved != nil {
		resolved = pulled.Resolved.Digest.String()
	}
	if !isSHA256Digest(resolved) {
		return "", fail(fmt.Sprintf("the reference resolved to no valid sha256 digest (%q)", resolved))
	}
	var platform string
	if isIndexMediaType(pulled.Resolved.MediaType) && pulled.Manifest != nil && pulled.Manifest.Digest.String() != resolved {
		platform = pulled.Manifest.Digest.String()
		if !isSHA256Digest(platform) {
			return "", fail(fmt.Sprintf("the platform manifest has no valid sha256 digest (%q)", platform))
		}
	}

	ctx = WithAuthConfig(ctx, params.AuthConfig)
	firstErr := c.verifyDigest(ctx, ref, resolved, params.PublicKey)
	if firstErr == nil {
		kind := "manifest"
		if platform != "" {
			kind = "index"
		}
		log.Info().
			Str("reference", ref.FullReference()).
			Str("verified_digest", resolved).
			Str("digest_kind", kind).
			Msg("OCI plugin signature check passed")
		return resolved, nil
	}
	if platform == "" {
		return "", fail(firstErr.Error())
	}

	secondErr := c.verifyDigest(ctx, ref, platform, params.PublicKey)
	if secondErr != nil {
		return "", fail(fmt.Sprintf("index %s: %v; platform manifest %s: %v", resolved, firstErr, platform, secondErr))
	}
	log.Info().
		Str("reference", ref.FullReference()).
		Str("verified_digest", platform).
		Str("digest_kind", "platform").
		Str("index_digest", resolved).
		Msg("OCI plugin signature check passed on the platform manifest (the index has no valid signature)")
	return platform, nil
}

// verifyDigest checks the signature of the artifact with the given digest in
// ref's repository.
func (c *OCIPluginClient) verifyDigest(ctx context.Context, ref *OCIReference, digest, publicKey string) error {
	pinned := *ref
	pinned.Digest = digest
	return c.securityService.VerifySignature(ctx, &pinned, publicKey)
}

// isSHA256Digest reports whether d is a well-formed "sha256:<64 hex>" digest.
func isSHA256Digest(d string) bool {
	parsed := digest.Digest(d)
	return parsed.Validate() == nil && parsed.Algorithm() == digest.SHA256
}

// isIndexMediaType reports whether mediaType is a multi-platform index.
func isIndexMediaType(mediaType string) bool {
	return mediaType == ocispec.MediaTypeImageIndex || mediaType == "application/vnd.docker.distribution.manifest.list.v2+json"
}

// HasPlugin checks if a plugin exists in the cache
func (c *OCIPluginClient) HasPlugin(digest, arch string) bool {
	return c.storage.HasExecutable(digest, arch)
}

// GetPlugin returns a local plugin if it exists in cache
func (c *OCIPluginClient) GetPlugin(ref *OCIReference, params *OCIPluginParams) (*LocalPlugin, error) {
	if !c.HasPlugin(ref.Digest, params.Architecture) {
		return nil, &ErrPluginNotFound{Reference: ref.FullReference()}
	}

	return c.getLocalPlugin(ref, params)
}

// getLocalPlugin returns a cached plugin after checking the executable
// against the SHA-256 recorded when it was fetched. A mismatch is refused
// (ErrCacheCorrupted); FetchPlugin then fetches the plugin again. An entry
// with no recorded hash (no metadata, or cached by an older release) cannot
// be checked: it is reported as ErrCacheUnchecked, which FetchPlugin answers
// by fetching it again to record its hash (loading the cached copy
// unverified only when the registry cannot be reached), or as
// ErrCacheCorrupted when signatures are required. So is an entry cached
// while signatures were not required, or verified with a different public
// key than the one now asked for.
func (c *OCIPluginClient) getLocalPlugin(ref *OCIReference, params *OCIPluginParams) (*LocalPlugin, error) {
	execPath := c.storage.GetExecutablePath(ref.Digest, params.Architecture)

	metadata, err := c.storage.LoadMetadata(ref.Digest, params.Architecture)
	if err != nil || metadata.BinarySHA256 == "" {
		reason := "no recorded binary hash"
		if err != nil {
			reason = fmt.Sprintf("no readable metadata: %v", err)
		}
		if c.signaturesEnforced() {
			return nil, &ErrCacheCorrupted{Path: execPath, Reason: reason + "; signatures are required, so the plugin must be fetched and verified again"}
		}
		return nil, &ErrCacheUnchecked{Path: execPath, Reason: reason}
	}

	if err := checkFileSHA256(execPath, metadata.BinarySHA256); err != nil {
		return nil, &ErrCacheCorrupted{Path: execPath, Reason: err.Error()}
	}
	if c.signaturesEnforced() && !metadata.SignatureChecked {
		return nil, &ErrCacheCorrupted{Path: execPath, Reason: "cached without signature verification, which is now required"}
	}
	if c.signaturesEnforced() && metadata.SignatureKey != params.PublicKey {
		return nil, &ErrCacheCorrupted{Path: execPath, Reason: "signature was verified with a different public key"}
	}
	// Serve the entry only if a fresh fetch of ref.Digest would verify the
	// same way: it was resolved from this digest, and the digest whose
	// signature passed is that one or its platform manifest. A tag fetch
	// caches a multi-arch plugin under the platform digest but resolved the
	// index, so a later pin to the platform digest is verified afresh. An
	// entry with no recorded digests (an earlier release, which may not have
	// verified anything) is verified again once.
	if c.signaturesEnforced() {
		if metadata.ResolvedDigest != ref.Digest {
			return nil, &ErrCacheCorrupted{Path: execPath, Reason: "cached from a different resolution of this reference, or by an earlier release; verifying it again"}
		}
		if metadata.SignedDigest == "" || (metadata.SignedDigest != metadata.ResolvedDigest && metadata.SignedDigest != metadata.ManifestDigest) {
			return nil, &ErrCacheCorrupted{Path: execPath, Reason: "no record of which digest's signature verified; verifying it again"}
		}
	}

	if err := c.storage.UpdateLastAccessed(ref.Digest, params.Architecture); err != nil {
		log.Warn().Err(err).Msg("Failed to update last accessed time")
	}

	return &LocalPlugin{
		Reference:      ref,
		Params:         params,
		ExecutablePath: execPath,
		CacheDir:       c.config.CacheDir,
		Verified:       metadata.Verified,
		FetchTime:      metadata.FetchTime,
		Config:         metadata.Config,
	}, nil
}

// unverifiedLocalPlugin returns the cached executable at execPath, which
// could not be integrity-checked, as an unverified plugin.
func (c *OCIPluginClient) unverifiedLocalPlugin(ref *OCIReference, params *OCIPluginParams, execPath string) *LocalPlugin {
	local := &LocalPlugin{
		Reference:      ref,
		Params:         params,
		ExecutablePath: execPath,
		CacheDir:       c.config.CacheDir,
		Verified:       false,
		FetchTime:      time.Now(), // Unknown fetch time
	}
	if metadata, err := c.storage.LoadMetadata(ref.Digest, params.Architecture); err == nil {
		local.FetchTime, local.Config = metadata.FetchTime, metadata.Config
	}
	return local
}

// checkFileSHA256 reports an error unless the file's SHA-256 is wantHex.
func checkFileSHA256(path, wantHex string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot read executable: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("cannot read executable: %w", err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != wantHex {
		return fmt.Errorf("executable SHA-256 %s does not match the %s recorded at fetch time", got, wantHex)
	}
	return nil
}

// SetActivePlugin creates a symlink to mark a plugin as the active version
func (c *OCIPluginClient) SetActivePlugin(name string, ref *OCIReference, params *OCIPluginParams) error {
	return c.storage.CreateActiveLink(name, ref.Digest, params.Architecture)
}

// GetActivePlugin returns the path to the active version of a plugin
func (c *OCIPluginClient) GetActivePlugin(name string) (string, error) {
	return c.storage.GetActivePath(name)
}

// ListCached returns information about all cached plugins
func (c *OCIPluginClient) ListCached() ([]*LocalPlugin, error) {
	// Get all metadata from storage
	allMetadata, err := c.storage.ListAllMetadata()
	if err != nil {
		return nil, fmt.Errorf("failed to list metadata: %w", err)
	}

	var localPlugins []*LocalPlugin
	for _, metadata := range allMetadata {
		// Verify executable still exists
		execPath := c.storage.GetExecutablePath(metadata.Reference.Digest, metadata.Params.Architecture)
		if _, err := os.Stat(execPath); err != nil {
			// Executable missing, skip this entry
			continue
		}

		localPlugin := &LocalPlugin{
			Reference:      metadata.Reference,
			Params:         metadata.Params,
			ExecutablePath: execPath,
			CacheDir:       c.config.CacheDir,
			Verified:       metadata.Verified,
			FetchTime:      metadata.FetchTime,
			Config:         metadata.Config,
		}

		localPlugins = append(localPlugins, localPlugin)
	}

	return localPlugins, nil
}

// ListCachedByRegistry returns cached plugins filtered by registry
func (c *OCIPluginClient) ListCachedByRegistry(registry string) ([]*LocalPlugin, error) {
	allPlugins, err := c.ListCached()
	if err != nil {
		return nil, err
	}

	var filtered []*LocalPlugin
	for _, plugin := range allPlugins {
		if plugin.Reference.Registry == registry {
			filtered = append(filtered, plugin)
		}
	}

	return filtered, nil
}

// ListCachedByArchitecture returns cached plugins filtered by architecture
func (c *OCIPluginClient) ListCachedByArchitecture(arch string) ([]*LocalPlugin, error) {
	allPlugins, err := c.ListCached()
	if err != nil {
		return nil, err
	}

	var filtered []*LocalPlugin
	for _, plugin := range allPlugins {
		if plugin.Params.Architecture == arch {
			filtered = append(filtered, plugin)
		}
	}

	return filtered, nil
}

// GarbageCollect removes old plugin versions from cache
func (c *OCIPluginClient) GarbageCollect(ctx context.Context, keepVersions int) error {
	log.Info().
		Int("keep_versions", keepVersions).
		Msg("Starting plugin cache garbage collection")

	if err := c.storage.GarbageCollect(keepVersions); err != nil {
		return fmt.Errorf("garbage collection failed: %w", err)
	}

	// Calculate cache size after cleanup
	size, err := c.storage.CalculateSize()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to calculate cache size after GC")
	} else {
		log.Info().
			Int64("cache_size_bytes", size).
			Msg("Garbage collection completed")
	}

	return nil
}

// GetCacheSize returns the current size of the plugin cache
func (c *OCIPluginClient) GetCacheSize() (int64, error) {
	return c.storage.CalculateSize()
}

// startGarbageCollection runs periodic garbage collection in the background
func (c *OCIPluginClient) startGarbageCollection(ctx context.Context) {
	ticker := time.NewTicker(c.config.GCInterval)
	defer ticker.Stop()
	defer close(c.gcStopped)

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Stopping background garbage collection")
			return
		case <-ticker.C:
			gcCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			if err := c.GarbageCollect(gcCtx, c.config.KeepVersions); err != nil {
				log.Error().Err(err).Msg("Background garbage collection failed")
			}
			cancel()
		}
	}
}

// Close shuts down the OCI client and cleans up resources
func (c *OCIPluginClient) Close() error {
	// Stop background garbage collection if running
	if c.gcCancel != nil {
		log.Info().Msg("Stopping OCI plugin client...")
		c.gcCancel()

		// Wait for garbage collection to stop (with timeout)
		select {
		case <-c.gcStopped:
			log.Info().Msg("Background garbage collection stopped")
		case <-time.After(5 * time.Second):
			log.Warn().Msg("Timed out waiting for garbage collection to stop")
		}
	}

	return nil
}

// isCompatibleArchitecture checks if plugin architecture is compatible with runtime architecture
func isCompatibleArchitecture(pluginArch, runtimeArch string) bool {
	// Exact match is always compatible
	if pluginArch == runtimeArch {
		return true
	}

	// Split architectures into OS/ARCH components
	pluginParts := strings.Split(pluginArch, "/")
	runtimeParts := strings.Split(runtimeArch, "/")

	if len(pluginParts) != 2 || len(runtimeParts) != 2 {
		return false
	}

	pluginOS, pluginCPU := pluginParts[0], pluginParts[1]
	runtimeOS, runtimeCPU := runtimeParts[0], runtimeParts[1]

	// OS must match (no cross-OS compatibility)
	if pluginOS != runtimeOS {
		return false
	}

	// Architecture compatibility matrix
	compatMatrix := map[string][]string{
		// arm64 can run amd64 binaries via emulation on many systems
		"arm64": {"amd64", "arm64"},
		// amd64 can only run amd64 binaries reliably
		"amd64": {"amd64"},
		// arm can run arm binaries
		"arm": {"arm"},
		// 386 can run on amd64 with compatibility layer
		"386": {"386"},
	}

	// Check if runtime CPU can execute plugin CPU
	compatibleCPUs, exists := compatMatrix[runtimeCPU]
	if !exists {
		return false
	}

	for _, compatCPU := range compatibleCPUs {
		if pluginCPU == compatCPU {
			return true
		}
	}

	return false
}