package agenttrace

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"
)

type exporter struct {
	StaticFingerprinter
}

func (e exporter) EdgeKey() (byte, []byte) { return e.ID, e.Key }

func TestEdgeKey(t *testing.T) {
	t.Cleanup(func() { SetFingerprinter(nil) })
	key := bytes.Repeat([]byte{9}, 32)

	SetFingerprinter(nil)
	if _, _, ok := EdgeKey(); ok {
		t.Fatal("no Fingerprinter, no edge key")
	}
	SetFingerprinter(StaticFingerprinter{ID: 3, Key: key})
	if _, _, ok := EdgeKey(); ok {
		t.Fatal("a Fingerprinter that exports nothing gives no edge key")
	}
	SetFingerprinter(exporter{StaticFingerprinter{ID: 0, Key: key}})
	if _, _, ok := EdgeKey(); ok {
		t.Fatal("key id 0 is not a key")
	}
	SetFingerprinter(exporter{StaticFingerprinter{ID: 3, Key: key}})
	if id, k, ok := EdgeKey(); !ok || id != 3 || !bytes.Equal(k, key) {
		t.Fatalf("EdgeKey = %d %x %v", id, k, ok)
	}
}

func TestInstallEdgeKey(t *testing.T) {
	t.Cleanup(func() { SetFingerprinter(nil) })
	key := bytes.Repeat([]byte{0x5A}, 32)
	InstallEdgeKey(11, key)
	f := CurrentFingerprinter()
	if f == nil || f.KeyID() != 11 {
		t.Fatal("key not installed")
	}
	want := StaticFingerprinter{ID: 11, Key: key}.Fingerprint([]byte("x"))
	if !bytes.Equal(f.Fingerprint([]byte("x")), want) {
		t.Fatal("edge fingerprint differs from the hub's")
	}
	key[0] = 0 // the installed key is a copy
	if !bytes.Equal(f.Fingerprint([]byte("x")), want) {
		t.Fatal("the installed key aliases the caller's slice")
	}
	for _, s := range []string{fmt.Sprint(f), fmt.Sprintf("%+v %#v", f, f)} {
		if bytes.Contains([]byte(s), bytes.Repeat([]byte{0x5A}, 4)) || bytes.Contains([]byte(s), []byte(hex.EncodeToString([]byte{0x5A, 0x5A}))) {
			t.Fatalf("key material in %q", s)
		}
	}

	InstallEdgeKey(0, key)
	if CurrentFingerprinter() != nil {
		t.Fatal("id 0 removes the key")
	}
	InstallEdgeKey(11, nil)
	if CurrentFingerprinter() != nil {
		t.Fatal("an empty key removes it")
	}
}
