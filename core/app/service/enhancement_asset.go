package service

import (
	"bytes"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	// Register the raster decoders the branding upload accepts. SVG is not a
	// raster format and is intentionally absent (rejected before decode); .ico
	// has no stdlib decoder and is therefore never accepted.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"

	"github.com/1Panel-dev/1Panel/core/global"
)

// brandingAsset describes one uploadable branding image. The map key is the
// fixed public asset key. It is simultaneously the multipart form value the
// client sends, the on-disk basename under uploads/theme, the served path
// segment (/api/v2/images/<key>), and the sentinel value stored in the setting
// so the DTO can report presence without ever exposing bytes or paths.
type brandingAsset struct {
	settingKey string // the enhancement setting key / DTO field, e.g. "Logo"
	maxBytes   int    // hard per-asset upload size cap
	requirePNG bool   // favicon: PNG only (stdlib cannot decode .ico)
}

const (
	brandingAssetMaxBytes   = 2 << 20    // 2 MiB for logos / login imagery
	brandingFaviconMaxBytes = 256 << 10  // 256 KiB for the favicon
	brandingMaxImagePixels  = 16_000_000 // reject decompression / pixel bombs (T5)
)

// brandingAssets is the fixed server-side enum of uploadable branding images.
// A client never supplies a filename; the asset key selects the hardcoded
// basename, so path traversal and filename injection are impossible by
// construction (T3).
var brandingAssets = map[string]brandingAsset{
	"logo":            {settingKey: "Logo", maxBytes: brandingAssetMaxBytes},
	"logoWithText":    {settingKey: "LogoWithText", maxBytes: brandingAssetMaxBytes},
	"favicon":         {settingKey: "Favicon", maxBytes: brandingFaviconMaxBytes, requirePNG: true},
	"loginImage":      {settingKey: "LoginImage", maxBytes: brandingAssetMaxBytes},
	"loginBackground": {settingKey: "LoginBackground", maxBytes: brandingAssetMaxBytes},
}

// allowedBrandingImageFormats is the raster whitelist accepted on upload. SVG is
// intentionally absent — it is an active-content format and is rejected before
// decode. .ico is absent — the stdlib cannot decode it, so it can never be
// validated and must not be accepted on MIME or extension alone (T10).
var allowedBrandingImageFormats = map[string]bool{
	"png":  true,
	"jpeg": true,
	"gif":  true,
	"webp": true,
}

func brandingAssetDir() string {
	return filepath.Join(global.CONF.Base.InstallDir, "1panel/uploads/theme")
}

// IsBrandingAssetFileName reports whether name is one of the fixed branding
// asset basenames. The public image-serve route uses this to serve only known
// assets, as defense-in-depth against traversal or stray-file serving (T3/T6).
func IsBrandingAssetFileName(name string) bool {
	_, ok := brandingAssets[name]
	return ok
}

// SaveAsset validates an uploaded branding image and, only if every check
// passes, writes it atomically to its fixed path and records a presence
// sentinel in settings. The value stored is the asset key itself, never a path
// or the bytes, so the (public) DTO can report presence without leaking data.
func (u *EnhancementService) SaveAsset(assetKey string, data []byte) error {
	asset, ok := brandingAssets[assetKey]
	if !ok {
		return fmt.Errorf("unsupported branding asset %q", assetKey)
	}
	if len(data) == 0 {
		return fmt.Errorf("branding asset %q is empty", assetKey)
	}
	if len(data) > asset.maxBytes {
		return fmt.Errorf("branding asset %q exceeds the %d byte limit", assetKey, asset.maxBytes)
	}
	// Reject SVG / XML / active-content payloads before touching a decoder (T1).
	if looksLikeMarkup(data) {
		return fmt.Errorf("svg, xml, and html uploads are not allowed")
	}
	// Bound the dimensions from the header BEFORE a full decode, to stop
	// decompression / pixel bombs (T5).
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("uploaded file is not a valid image: %w", err)
	}
	if !allowedBrandingImageFormats[format] {
		return fmt.Errorf("unsupported image format %q", format)
	}
	if asset.requirePNG && format != "png" {
		return fmt.Errorf("%s must be a PNG image", assetKey)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > brandingMaxImagePixels {
		return fmt.Errorf("image dimensions are out of range")
	}
	// A full decode confirms the header is backed by a complete, well-formed
	// image body, rejecting a valid header over a truncated or corrupt body.
	// (Stdlib decoders ignore bytes after the image terminator, so this does
	// not by itself reject an appended-markup polyglot; the serve route
	// neutralizes that with a fixed raster Content-Type + nosniff — see
	// core/init/router/router.go.)
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return fmt.Errorf("uploaded file failed to decode: %w", err)
	}
	if err := writeBrandingAssetFile(assetKey, data); err != nil {
		return err
	}
	return settingRepo.UpdateOrCreate(asset.settingKey, assetKey)
}

// ResetAsset removes a custom branding image and clears its presence sentinel,
// restoring the built-in default. Cleanup targets the exact fixed basename with
// no glob (T6).
func (u *EnhancementService) ResetAsset(assetKey string) error {
	asset, ok := brandingAssets[assetKey]
	if !ok {
		return fmt.Errorf("unsupported branding asset %q", assetKey)
	}
	dir := brandingAssetDir()
	dstPath := filepath.Join(dir, assetKey)
	if !isWithinDir(dstPath, dir) {
		return fmt.Errorf("resolved asset path escapes the theme directory")
	}
	if err := os.Remove(dstPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove asset: %w", err)
	}
	return settingRepo.UpdateOrCreate(asset.settingKey, "")
}

// looksLikeMarkup scans a bounded, case-folded prefix for SVG/XML/HTML markers.
// It is the upload-side half of the pre-auth stored-XSS control; the serve side
// additionally refuses to emit active-content types with nosniff (T1/T2).
func looksLikeMarkup(data []byte) bool {
	n := len(data)
	if n > 1024 {
		n = 1024
	}
	head := strings.ToLower(strings.TrimSpace(string(data[:n])))
	return strings.Contains(head, "<svg") ||
		strings.HasPrefix(head, "<?xml") ||
		strings.Contains(head, "<!doctype") ||
		strings.Contains(head, "<html")
}

// writeBrandingAssetFile writes data to uploads/theme/<assetKey> atomically via
// a temp file and rename, after asserting the resolved path stays inside the
// theme directory (T3/T6).
func writeBrandingAssetFile(assetKey string, data []byte) error {
	dir := brandingAssetDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create theme dir: %w", err)
	}
	dstPath := filepath.Join(dir, assetKey)
	if !isWithinDir(dstPath, dir) {
		return fmt.Errorf("resolved asset path escapes the theme directory")
	}
	tmp, err := os.CreateTemp(dir, assetKey+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp asset: %w", err)
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp asset: %w", err)
	}
	if err := tmp.Chmod(0644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp asset: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp asset: %w", err)
	}
	if err := os.Rename(tmpPath, dstPath); err != nil {
		return fmt.Errorf("commit asset: %w", err)
	}
	committed = true
	return nil
}

// isWithinDir reports whether path resolves inside dir (not dir's parent and not
// an absolute escape).
func isWithinDir(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return false
	}
	return true
}
