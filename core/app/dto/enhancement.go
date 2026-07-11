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
	Theme      string `json:"theme"`
	ThemeColor string `json:"themeColor"`
}

type EnhancementSettingUpdate struct {
	Key   string `json:"key" validate:"required,oneof=Theme ThemeColor Watermark WatermarkShow"`
	Value string `json:"value" validate:"max=8192"`
}
