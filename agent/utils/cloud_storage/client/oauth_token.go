package client

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
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
