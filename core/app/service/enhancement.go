package service

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/constant"
)

const (
	defaultEnhancementTheme      = "light"
	defaultEnhancementThemeColor = `{"light":"#005eeb","dark":"#F0BE96"}`
	defaultEnhancementWatermark  = `{"lightColor":"rgba(0, 0, 0, 0.15)","darkColor":"rgba(255, 255, 255, 0.15)","fontSize":16,"content":"${nodeName} - ${nodeAddr}","rotate":-22,"gap":100}`
)

var (
	hexColorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$`)
	rgbColorPattern = regexp.MustCompile(`^rgba?\(\s*([0-9]{1,3})\s*,\s*([0-9]{1,3})\s*,\s*([0-9]{1,3})(?:\s*,\s*(0|1|0?\.\d+))?\s*\)$`)
)

type enhancementThemeColor struct {
	Light                string              `json:"light"`
	Dark                 string              `json:"dark"`
	ThemePredefineColors map[string][]string `json:"themePredefineColors,omitempty"`
}

type enhancementWatermark struct {
	LightColor string `json:"lightColor"`
	DarkColor  string `json:"darkColor"`
	FontSize   int    `json:"fontSize"`
	Content    string `json:"content"`
	Rotate     int    `json:"rotate"`
	Gap        int    `json:"gap"`
}

type EnhancementService struct{}

type IEnhancementService interface {
	GetSettingInfo() (*dto.EnhancementSettingInfo, error)
	GetPublicSettingInfo() (*dto.PublicEnhancementSettingInfo, error)
	UpdateSetting(key, value string) error
	SaveAsset(assetKey string, data []byte) error
	ResetAsset(assetKey string) error
}

func NewIEnhancementService() IEnhancementService {
	return &EnhancementService{}
}

func (u *EnhancementService) GetSettingInfo() (*dto.EnhancementSettingInfo, error) {
	return &dto.EnhancementSettingInfo{
		Theme:             loadValidatedEnhancementValue("Theme", defaultEnhancementTheme),
		ThemeColor:        loadValidatedEnhancementValue("ThemeColor", defaultEnhancementThemeColor),
		Watermark:         loadValidatedEnhancementValue("Watermark", defaultEnhancementWatermark),
		WatermarkShow:     loadValidatedEnhancementValue("WatermarkShow", constant.StatusDisable),
		Title:             loadValidatedEnhancementValue("Title", ""),
		MasterAlias:       loadValidatedEnhancementValue("MasterAlias", ""),
		LoginBgType:       loadValidatedEnhancementValue("LoginBgType", ""),
		LoginBackground:   loadValidatedEnhancementValue("LoginBackground", ""),
		LoginBtnLinkColor: loadValidatedEnhancementValue("LoginBtnLinkColor", ""),
		Logo:              loadValidatedEnhancementValue("Logo", ""),
		LogoWithText:      loadValidatedEnhancementValue("LogoWithText", ""),
		Favicon:           loadValidatedEnhancementValue("Favicon", ""),
		LoginImage:        loadValidatedEnhancementValue("LoginImage", ""),
		LoginWelcome:      loadValidatedEnhancementValue("LoginWelcome", ""),
		LoginSubtitle:     loadValidatedEnhancementValue("LoginSubtitle", ""),
		Copyright:         loadValidatedEnhancementValue("Copyright", ""),
	}, nil
}

func (u *EnhancementService) GetPublicSettingInfo() (*dto.PublicEnhancementSettingInfo, error) {
	return &dto.PublicEnhancementSettingInfo{
		Theme:             loadValidatedEnhancementValue("Theme", defaultEnhancementTheme),
		ThemeColor:        loadValidatedEnhancementValue("ThemeColor", defaultEnhancementThemeColor),
		Title:             loadValidatedEnhancementValue("Title", ""),
		MasterAlias:       loadValidatedEnhancementValue("MasterAlias", ""),
		LoginBgType:       loadValidatedEnhancementValue("LoginBgType", ""),
		LoginBackground:   loadValidatedEnhancementValue("LoginBackground", ""),
		LoginBtnLinkColor: loadValidatedEnhancementValue("LoginBtnLinkColor", ""),
		// Presence sentinels only — the images they gate are already anonymously
		// fetchable at /api/v2/images/*, so a sentinel leaks nothing new, while
		// the login page needs them to render custom branding before auth (T8).
		Logo:         loadValidatedEnhancementValue("Logo", ""),
		LogoWithText: loadValidatedEnhancementValue("LogoWithText", ""),
		Favicon:      loadValidatedEnhancementValue("Favicon", ""),
		LoginImage:   loadValidatedEnhancementValue("LoginImage", ""),
		// Login-page text the community login page renders pre-auth (interpolation only).
		LoginWelcome:  loadValidatedEnhancementValue("LoginWelcome", ""),
		LoginSubtitle: loadValidatedEnhancementValue("LoginSubtitle", ""),
		Copyright:     loadValidatedEnhancementValue("Copyright", ""),
	}, nil
}

func (u *EnhancementService) UpdateSetting(key, value string) error {
	if err := validateEnhancementSetting(key, value); err != nil {
		return err
	}
	if err := settingRepo.UpdateOrCreate(key, value); err != nil {
		return err
	}
	if key == "Watermark" {
		return settingRepo.UpdateOrCreate("WatermarkShow", constant.StatusEnable)
	}
	return nil
}

func loadEnhancementValue(key, fallback string) string {
	value, err := settingRepo.GetValueByKey(key)
	if err != nil || strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func loadValidatedEnhancementValue(key, fallback string) string {
	value := loadEnhancementValue(key, fallback)
	if err := validateEnhancementSetting(key, value); err != nil {
		return fallback
	}
	return value
}

func validateEnhancementSetting(key, value string) error {
	switch key {
	case "Theme":
		if value != "light" && value != "dark" && value != "auto" {
			return fmt.Errorf("unsupported theme %q", value)
		}
	case "ThemeColor":
		var colors enhancementThemeColor
		if err := json.Unmarshal([]byte(value), &colors); err != nil {
			return fmt.Errorf("invalid theme color: %w", err)
		}
		if !isSafeCSSColor(colors.Light) || !isSafeCSSColor(colors.Dark) {
			return fmt.Errorf("theme colors must be hex, rgb, or rgba values")
		}
		for theme, predefined := range colors.ThemePredefineColors {
			if theme != "light" && theme != "dark" {
				return fmt.Errorf("unsupported predefined color group %q", theme)
			}
			if len(predefined) > 20 {
				return fmt.Errorf("too many predefined colors for %s", theme)
			}
			for _, color := range predefined {
				if !isSafeCSSColor(color) {
					return fmt.Errorf("invalid predefined color %q", color)
				}
			}
		}
	case "Watermark":
		var watermark enhancementWatermark
		if err := json.Unmarshal([]byte(value), &watermark); err != nil {
			return fmt.Errorf("invalid watermark: %w", err)
		}
		if strings.TrimSpace(watermark.Content) == "" || len([]rune(watermark.Content)) > 128 {
			return fmt.Errorf("watermark content must contain 1 to 128 characters")
		}
		if !isSafeCSSColor(watermark.LightColor) || !isSafeCSSColor(watermark.DarkColor) {
			return fmt.Errorf("watermark colors must be hex, rgb, or rgba values")
		}
		if watermark.FontSize < 12 || watermark.FontSize > 100 {
			return fmt.Errorf("watermark font size must be between 12 and 100")
		}
		if watermark.Rotate < -180 || watermark.Rotate > 180 {
			return fmt.Errorf("watermark rotation must be between -180 and 180")
		}
		if watermark.Gap < 0 || watermark.Gap > 500 {
			return fmt.Errorf("watermark gap must be between 0 and 500")
		}
	case "WatermarkShow":
		if value != constant.StatusEnable && value != constant.StatusDisable {
			return fmt.Errorf("unsupported watermark status %q", value)
		}
	case "Title", "MasterAlias":
		if err := validateBrandingText(value, 64); err != nil {
			return err
		}
	case "LoginWelcome", "LoginSubtitle":
		if err := validateBrandingText(value, 128); err != nil {
			return err
		}
	case "Copyright":
		if err := validateBrandingText(value, 200); err != nil {
			return err
		}
	case "LoginBgType":
		if value != "" && value != "image" && value != "color" {
			return fmt.Errorf("unsupported login background type %q", value)
		}
	case "LoginBackground":
		// Dual-use: a CSS color (bg-type=color) or the image presence sentinel
		// (bg-type=image, set only by the asset upload).
		if value == "" || value == "loginBackground" {
			return nil
		}
		if !isSafeCSSColor(value) {
			return fmt.Errorf("LoginBackground must be empty or a hex, rgb, or rgba value")
		}
	case "LoginBtnLinkColor":
		if value != "" && !isSafeCSSColor(value) {
			return fmt.Errorf("LoginBtnLinkColor must be empty or a hex, rgb, or rgba value")
		}
	case "Logo", "LogoWithText", "Favicon", "LoginImage":
		// Image fields hold only a fixed presence sentinel (the asset key). The
		// bytes live under uploads/theme and are written solely by the asset
		// upload service, never through the text/update path.
		return validateBrandingAssetSentinel(key, value)
	default:
		return fmt.Errorf("unsupported enhancement setting %q", key)
	}
	return nil
}

// validateBrandingAssetSentinel accepts only empty (unset) or the exact
// lower-camel presence sentinel matching the image setting key (e.g. "Logo" ->
// "logo"). It guarantees a stored image reference can never carry markup, a
// path, or an unexpected value.
func validateBrandingAssetSentinel(key, value string) error {
	if value == "" {
		return nil
	}
	if value != lowerFirstRune(key) {
		return fmt.Errorf("unsupported %s value %q", key, value)
	}
	return nil
}

func lowerFirstRune(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

// validateBrandingText bounds an operator-authored branding string that renders
// on the login page before authentication. It rejects angle brackets and
// control characters (so the value can never be persisted as markup, the
// server-side half of the pre-auth XSS control) and caps the rune length. An
// empty value is allowed and means "unset — use the frontend default".
func validateBrandingText(value string, maxRunes int) error {
	if value == "" {
		return nil
	}
	if strings.ContainsAny(value, "<>") {
		return fmt.Errorf("branding text must not contain angle brackets")
	}
	for _, r := range value {
		// Reject C0/C1 control chars and Unicode bidirectional formatting
		// characters (RLO/LRO/isolates/marks, U+202A-202E, U+2066-2069, ...),
		// which can visually reorder this pre-auth text (Trojan-Source style).
		if unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r) {
			return fmt.Errorf("branding text must not contain control or bidirectional formatting characters")
		}
	}
	if len([]rune(value)) > maxRunes {
		return fmt.Errorf("branding text must be at most %d characters", maxRunes)
	}
	return nil
}

func isSafeCSSColor(value string) bool {
	value = strings.TrimSpace(value)
	if hexColorPattern.MatchString(value) {
		return true
	}
	matches := rgbColorPattern.FindStringSubmatch(value)
	if len(matches) == 0 {
		return false
	}
	for _, channel := range matches[1:4] {
		channelValue, err := strconv.Atoi(channel)
		if err != nil || channelValue > 255 {
			return false
		}
	}
	if matches[4] == "" {
		return true
	}
	alpha, err := strconv.ParseFloat(matches[4], 64)
	return err == nil && alpha >= 0 && alpha <= 1
}
