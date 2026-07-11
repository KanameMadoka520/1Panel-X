package dto

type EnhancementSettingInfo struct {
	Theme             string `json:"theme"`
	ThemeColor        string `json:"themeColor"`
	Watermark         string `json:"watermark"`
	WatermarkShow     string `json:"watermarkShow"`
	Title             string `json:"title"`
	Logo              string `json:"logo"`
	LogoWithText      string `json:"logoWithText"`
	Favicon           string `json:"favicon"`
	LoginImage        string `json:"loginImage"`
	LoginBgType       string `json:"loginBgType"`
	LoginBackground   string `json:"loginBackground"`
	LoginBtnLinkColor string `json:"loginBtnLinkColor"`
	MasterAlias       string `json:"masterAlias"`
}

type PublicEnhancementSettingInfo struct {
	Theme             string `json:"theme"`
	ThemeColor        string `json:"themeColor"`
	Title             string `json:"title"`
	MasterAlias       string `json:"masterAlias"`
	LoginBgType       string `json:"loginBgType"`
	LoginBackground   string `json:"loginBackground"`
	LoginBtnLinkColor string `json:"loginBtnLinkColor"`
	// Presence sentinels for anonymously-served branding images (never bytes or
	// paths); the login page and sidebar consume them before authentication.
	Logo         string `json:"logo"`
	LogoWithText string `json:"logoWithText"`
	Favicon      string `json:"favicon"`
	LoginImage   string `json:"loginImage"`
}

type EnhancementSettingUpdate struct {
	Key   string `json:"key" validate:"required,oneof=Theme ThemeColor Watermark WatermarkShow Title MasterAlias LoginBgType LoginBackground LoginBtnLinkColor"`
	Value string `json:"value" validate:"max=8192"`
}

// EnhancementAssetReset clears a single uploaded branding image. The key is the
// fixed asset enum; image bytes are only ever written by the upload endpoint.
type EnhancementAssetReset struct {
	Key string `json:"key" validate:"required,oneof=logo logoWithText favicon loginImage loginBackground"`
}
