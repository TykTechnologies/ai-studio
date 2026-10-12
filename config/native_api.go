package config

// NativeMaxBodyBytesLimit is the most a native API create body (an Anthropic
// message batch) may be configured to: 256 MiB. Studio, the Microgateway and
// the proxy all clamp NATIVE_API_MAX_BODY_BYTES to it.
const NativeMaxBodyBytesLimit int64 = 256 << 20

// ClampNativeMaxBodyBytes returns n capped at NativeMaxBodyBytesLimit,
// calling warn with the configured value when it was above the cap. Zero
// (the default) and smaller values are returned unchanged.
func ClampNativeMaxBodyBytes(n int64, warn func(configured int64)) int64 {
	if n <= NativeMaxBodyBytesLimit {
		return n
	}
	if warn != nil {
		warn(n)
	}
	return NativeMaxBodyBytesLimit
}
