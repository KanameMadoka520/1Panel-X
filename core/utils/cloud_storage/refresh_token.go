package cloud_storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/core/global"
	"github.com/go-resty/resty/v2"
)

const oauthTokenResponseLimit = int64(1 << 20)

var oauthTokenHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return errors.New("OAuth token redirect refused")
	},
}

type oauthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Error        string `json:"error"`
}

func loadParamFromVars(key string, vars map[string]interface{}) string {
	if _, ok := vars[key]; !ok {
		if key != "bucket" && key != "port" && key != "authMode" && key != "passPhrase" {
			global.LOG.Errorf("load param %s from vars failed, err: not exist!", key)
		}
		return ""
	}

	return fmt.Sprintf("%v", vars[key])
}

type aliTokenResp struct {
	RefreshToken string `json:"refresh_token"`
	AccessToken  string `json:"access_token"`
}

func RefreshALIToken(varMap map[string]interface{}) (string, error) {
	refresh_token := loadParamFromVars("refresh_token", varMap)
	if len(refresh_token) == 0 {
		return "", errors.New("no such refresh token find in db")
	}
	client := resty.New().SetTimeout(30 * time.Second)
	client.SetRedirectPolicy(resty.NoRedirectPolicy())
	data := map[string]interface{}{
		"grant_type":    "refresh_token",
		"refresh_token": refresh_token,
	}

	url := "https://api.aliyundrive.com/token/refresh"
	resp, err := client.R().
		SetBody(data).
		Post(url)

	if err != nil {
		return "", fmt.Errorf("load account token failed, err: %v", err)
	}
	if resp.StatusCode() != 200 {
		return "", fmt.Errorf("load account token failed, code: %v", resp.StatusCode())
	}
	var respItem aliTokenResp
	if err := json.Unmarshal(resp.Body(), &respItem); err != nil {
		return "", errors.New("Aliyun token response invalid")
	}
	if respItem.RefreshToken == "" {
		return refresh_token, nil
	}
	return respItem.RefreshToken, nil
}

func RefreshToken(grantType string, tokenType string, varMap map[string]interface{}) (string, error) {
	data := url.Values{}
	isCN := loadParamFromVars("isCN", varMap)
	data.Set("client_id", loadParamFromVars("client_id", varMap))
	data.Set("client_secret", loadParamFromVars("client_secret", varMap))
	if grantType == "refresh_token" {
		data.Set("grant_type", "refresh_token")
		data.Set("refresh_token", loadParamFromVars("refresh_token", varMap))
	} else {
		data.Set("grant_type", "authorization_code")
		data.Set("code", loadParamFromVars("code", varMap))
	}
	data.Set("redirect_uri", loadParamFromVars("redirect_uri", varMap))
	tokenURL := "https://login.microsoftonline.com/common/oauth2/v2.0/token"
	if isCN == "true" {
		tokenURL = "https://login.chinacloudapi.cn/common/oauth2/v2.0/token"
	}
	token, err := requestOAuthToken(tokenURL, data)
	if err != nil {
		return "", err
	}
	if tokenType == "accessToken" {
		return token.AccessToken, nil
	}
	if token.RefreshToken != "" {
		return token.RefreshToken, nil
	}
	if grantType == "refresh_token" {
		refreshToken := loadParamFromVars("refresh_token", varMap)
		if refreshToken != "" {
			return refreshToken, nil
		}
	}
	return "", errors.New("OAuth refresh token missing")
}

func RefreshGoogleToken(grantType string, tokenType string, varMap map[string]interface{}) (string, error) {
	data := url.Values{}
	data.Set("client_id", loadParamFromVars("client_id", varMap))
	data.Set("client_secret", loadParamFromVars("client_secret", varMap))
	data.Set("redirect_uri", loadParamFromVars("redirect_uri", varMap))
	if grantType == "refresh_token" {
		data.Set("grant_type", "refresh_token")
		data.Set("refresh_token", loadParamFromVars("refresh_token", varMap))
	} else {
		data.Set("grant_type", "authorization_code")
		data.Set("code", loadParamFromVars("code", varMap))
	}
	token, err := requestOAuthToken("https://oauth2.googleapis.com/token", data)
	if err != nil {
		return "", err
	}
	if tokenType == "accessToken" {
		return token.AccessToken, nil
	}
	if token.RefreshToken != "" {
		return token.RefreshToken, nil
	}
	if grantType == "refresh_token" {
		refreshToken := loadParamFromVars("refresh_token", varMap)
		if refreshToken != "" {
			return refreshToken, nil
		}
	}
	return "", errors.New("OAuth refresh token missing")
}

func requestOAuthToken(endpoint string, form url.Values) (oauthTokenResponse, error) {
	request, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return oauthTokenResponse{}, errors.New("OAuth token request failed")
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response, err := oauthTokenHTTPClient.Do(request)
	if err != nil {
		return oauthTokenResponse{}, errors.New("OAuth token request failed")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, oauthTokenResponseLimit+1))
	if err != nil || int64(len(body)) > oauthTokenResponseLimit {
		return oauthTokenResponse{}, errors.New("OAuth token response invalid")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return oauthTokenResponse{}, errors.New("OAuth token request rejected")
	}
	var token oauthTokenResponse
	if err := json.Unmarshal(body, &token); err != nil || token.Error != "" || token.AccessToken == "" {
		return oauthTokenResponse{}, errors.New("OAuth token response invalid")
	}
	return token, nil
}
