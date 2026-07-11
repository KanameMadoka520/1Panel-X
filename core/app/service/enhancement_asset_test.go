package service

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/1Panel-dev/1Panel/core/global"
)

// setupAssetTest wires an in-memory settings DB and an isolated install dir so
// SaveAsset/ResetAsset can write real files under a temp uploads/theme.
func setupAssetTest(t *testing.T) string {
	t.Helper()
	setupEnhancementTestDB(t)
	oldInstall := global.CONF.Base.InstallDir
	dir := t.TempDir()
	global.CONF.Base.InstallDir = dir
	t.Cleanup(func() { global.CONF.Base.InstallDir = oldInstall })
	return dir
}

func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func encodeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

func encodeGIF(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewPaletted(image.Rect(0, 0, w, h), color.Palette{color.Black, color.White})
	var buf bytes.Buffer
	if err := gif.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode gif: %v", err)
	}
	return buf.Bytes()
}

// craftHugePNG builds a tiny but structurally valid PNG whose IHDR declares
// enormous dimensions. image.DecodeConfig reads these from the header without
// allocating pixels, so it exercises the pre-decode dimension cap (T5) with a
// few-byte file — the essence of a decompression / pixel bomb.
func craftHugePNG(w, h uint32) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a})
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], w)
	binary.BigEndian.PutUint32(ihdr[4:8], h)
	ihdr[8] = 8  // bit depth
	ihdr[9] = 6  // color type RGBA (non-paletted -> DecodeConfig returns after IHDR)
	ihdr[10] = 0 // compression
	ihdr[11] = 0 // filter
	ihdr[12] = 0 // interlace
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(ihdr)))
	buf.Write(length[:])
	typeAndData := append([]byte("IHDR"), ihdr...)
	buf.Write(typeAndData)
	var crc [4]byte
	binary.BigEndian.PutUint32(crc[:], crc32.ChecksumIEEE(typeAndData))
	buf.Write(crc[:])
	return buf.Bytes()
}

func TestIsBrandingAssetFileName(t *testing.T) {
	for _, name := range []string{"logo", "logoWithText", "favicon", "loginImage", "loginBackground"} {
		if !IsBrandingAssetFileName(name) {
			t.Fatalf("expected %q to be a known branding asset", name)
		}
	}
	for _, name := range []string{"", "..", "passwd", "logo.png", "Logo", "evil"} {
		if IsBrandingAssetFileName(name) {
			t.Fatalf("did not expect %q to be a known branding asset", name)
		}
	}
}

func TestSaveAssetHappyPathWritesFileAndSentinel(t *testing.T) {
	dir := setupAssetTest(t)
	svc := NewIEnhancementService()

	if err := svc.SaveAsset("logo", encodePNG(t, 4, 4)); err != nil {
		t.Fatalf("save logo: %v", err)
	}
	// The file lands at the fixed basename with no extension, matching the
	// serve route, and no temp residue remains.
	assetPath := filepath.Join(dir, "1panel/uploads/theme", "logo")
	if _, err := os.Stat(assetPath); err != nil {
		t.Fatalf("logo file not written: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "1panel/uploads/theme"))
	if err != nil {
		t.Fatalf("read theme dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "logo" {
		t.Fatalf("unexpected theme dir contents: %v", entries)
	}
	// The stored setting is the presence sentinel and survives read-path
	// re-validation (proves the validator accepts it).
	if got := loadValidatedEnhancementValue("Logo", ""); got != "logo" {
		t.Fatalf("Logo sentinel not persisted, got %q", got)
	}
	info, err := svc.GetSettingInfo()
	if err != nil {
		t.Fatalf("get setting info: %v", err)
	}
	if info.Logo != "logo" {
		t.Fatalf("authed DTO logo presence not set: %q", info.Logo)
	}
}

func TestSaveAssetAcceptsJPEGAndWEBPFallbackFormats(t *testing.T) {
	setupAssetTest(t)
	svc := NewIEnhancementService()
	if err := svc.SaveAsset("logo", encodeJPEG(t, 8, 8)); err != nil {
		t.Fatalf("jpeg logo rejected: %v", err)
	}
	if err := svc.SaveAsset("loginImage", encodeGIF(t, 8, 8)); err != nil {
		t.Fatalf("gif loginImage rejected: %v", err)
	}
}

func TestSaveAssetLoginBackgroundSentinelSurvivesReadPath(t *testing.T) {
	setupAssetTest(t)
	svc := NewIEnhancementService()
	if err := svc.SaveAsset("loginBackground", encodePNG(t, 4, 4)); err != nil {
		t.Fatalf("save loginBackground: %v", err)
	}
	// LoginBackground is dual-use (color OR image sentinel); the sentinel must
	// not be stripped by the color-oriented validator on read.
	if got := loadValidatedEnhancementValue("LoginBackground", ""); got != "loginBackground" {
		t.Fatalf("LoginBackground image sentinel did not survive: %q", got)
	}
}

func TestSaveAssetRejectsSVGAndMarkup(t *testing.T) {
	setupAssetTest(t)
	svc := NewIEnhancementService()
	payloads := map[string][]byte{
		"bare svg":       []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"leading ws svg": []byte("  \n<svg onload=alert(1)></svg>"),
		"xml decl":       []byte(`<?xml version="1.0"?><svg></svg>`),
		"html":           []byte("<!DOCTYPE html><html><body>x</body></html>"),
	}
	for name, data := range payloads {
		if err := svc.SaveAsset("logo", data); err == nil {
			t.Fatalf("%s: expected markup upload to be rejected", name)
		}
	}
	// A PNG whose trailing bytes contain <svg (polyglot) is still a valid PNG
	// and is allowed to be stored, but the serve side (nosniff + allowlisted
	// Content-Type) guarantees it is never rendered as SVG.
	if err := svc.SaveAsset("logo", encodePNG(t, 4, 4)); err != nil {
		t.Fatalf("valid png rejected: %v", err)
	}
}

func TestSaveAssetRejectsNonImage(t *testing.T) {
	setupAssetTest(t)
	svc := NewIEnhancementService()
	if err := svc.SaveAsset("logo", []byte("this is definitely not an image")); err == nil {
		t.Fatal("expected non-image upload to be rejected")
	}
}

func TestSaveAssetRejectsPixelBomb(t *testing.T) {
	setupAssetTest(t)
	svc := NewIEnhancementService()
	bomb := craftHugePNG(20000, 20000) // 400M pixels, a few dozen bytes on disk
	if _, _, err := image.DecodeConfig(bytes.NewReader(bomb)); err != nil {
		t.Fatalf("crafted PNG header is not decodable, test is invalid: %v", err)
	}
	if err := svc.SaveAsset("logo", bomb); err == nil {
		t.Fatal("expected oversized-dimension image to be rejected before full decode")
	}
	// Nothing should have been written for a rejected upload.
	if _, err := os.Stat(filepath.Join(global.CONF.Base.InstallDir, "1panel/uploads/theme", "logo")); !os.IsNotExist(err) {
		t.Fatalf("rejected pixel bomb must not write a file: %v", err)
	}
}

func TestSaveAssetRejectsOversizeBytes(t *testing.T) {
	setupAssetTest(t)
	svc := NewIEnhancementService()
	// Larger than the 2 MiB general cap; rejected on size before any decode.
	if err := svc.SaveAsset("logo", make([]byte, brandingAssetMaxBytes+1)); err == nil {
		t.Fatal("expected oversize upload to be rejected")
	}
	// Favicon has the tighter 256 KiB cap.
	if err := svc.SaveAsset("favicon", make([]byte, brandingFaviconMaxBytes+1)); err == nil {
		t.Fatal("expected oversize favicon to be rejected")
	}
}

func TestSaveAssetFaviconRequiresPNG(t *testing.T) {
	setupAssetTest(t)
	svc := NewIEnhancementService()
	if err := svc.SaveAsset("favicon", encodeJPEG(t, 8, 8)); err == nil {
		t.Fatal("expected non-PNG favicon to be rejected (no stdlib .ico decoder)")
	}
	if err := svc.SaveAsset("favicon", encodePNG(t, 16, 16)); err != nil {
		t.Fatalf("png favicon rejected: %v", err)
	}
}

func TestSaveAssetRejectsUnknownAssetKey(t *testing.T) {
	setupAssetTest(t)
	svc := NewIEnhancementService()
	for _, key := range []string{"", "Logo", "../logo", "background", "evil"} {
		if err := svc.SaveAsset(key, encodePNG(t, 4, 4)); err == nil {
			t.Fatalf("expected unknown asset key %q to be rejected", key)
		}
	}
}

func TestSaveAssetRejectsEmpty(t *testing.T) {
	setupAssetTest(t)
	if err := NewIEnhancementService().SaveAsset("logo", nil); err == nil {
		t.Fatal("expected empty upload to be rejected")
	}
}

func TestResetAssetRemovesFileAndClearsSentinel(t *testing.T) {
	dir := setupAssetTest(t)
	svc := NewIEnhancementService()
	if err := svc.SaveAsset("favicon", encodePNG(t, 16, 16)); err != nil {
		t.Fatalf("save favicon: %v", err)
	}
	assetPath := filepath.Join(dir, "1panel/uploads/theme", "favicon")
	if _, err := os.Stat(assetPath); err != nil {
		t.Fatalf("favicon not written: %v", err)
	}
	if err := svc.ResetAsset("favicon"); err != nil {
		t.Fatalf("reset favicon: %v", err)
	}
	if _, err := os.Stat(assetPath); !os.IsNotExist(err) {
		t.Fatalf("favicon file should be removed: %v", err)
	}
	if got := loadValidatedEnhancementValue("Favicon", ""); got != "" {
		t.Fatalf("Favicon sentinel should be cleared, got %q", got)
	}
	// Reset is idempotent even when nothing is present.
	if err := svc.ResetAsset("favicon"); err != nil {
		t.Fatalf("reset on absent asset should be a no-op: %v", err)
	}
	if err := svc.ResetAsset("nope"); err == nil {
		t.Fatal("expected reset with unknown key to be rejected")
	}
}
