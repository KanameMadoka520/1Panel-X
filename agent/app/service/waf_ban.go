package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/1Panel-dev/1Panel/agent/app/dto/response"
)

const (
	wafAdminBase        = "http://127.0.0.1:9000"
	wafAdminTokenHeader = "X-Waf-Admin-Token"
)

// IWafBanService reads and lifts the gateway's temporary IP bans.
//
// Live bans are held in the gateway process, not in the panel database, so they
// are read over the gateway's loopback management API. That is also why they do
// not survive a gateway container restart — a real behaviour of our own
// architecture, which the UI states plainly instead of borrowing another
// product's wording.
type IWafBanService interface {
	List() (response.WafBanState, error)
	Release(ip string) (response.WafBanState, error)
}

type WafBanService struct {
	client *http.Client
}

func NewIWafBanService() IWafBanService {
	return &WafBanService{client: &http.Client{Timeout: 3 * time.Second}}
}

func (s *WafBanService) List() (response.WafBanState, error) {
	var state response.WafBanState
	if err := s.call(http.MethodGet, "/admin/state", nil, &state); err != nil {
		return response.WafBanState{}, err
	}
	if state.Bans == nil {
		state.Bans = []response.WafBan{}
	}
	return state, nil
}

func (s *WafBanService) Release(ip string) (response.WafBanState, error) {
	body, err := json.Marshal(map[string]string{"ip": ip})
	if err != nil {
		return response.WafBanState{}, err
	}
	var out struct {
		Released bool `json:"released"`
	}
	if err := s.call(http.MethodPost, "/admin/bans/release", body, &out); err != nil {
		return response.WafBanState{}, err
	}
	if !out.Released {
		// Reported honestly rather than as a success: the address was not banned,
		// so nothing was lifted.
		return response.WafBanState{}, fmt.Errorf("no active ban found for %s", ip)
	}
	return s.List()
}

func (s *WafBanService) call(method, path string, body []byte, out any) error {
	token, err := ensureWafAdminToken()
	if err != nil {
		return err
	}
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, wafAdminBase+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set(wafAdminTokenHeader, token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return errors.New("the WAF gateway is not reachable; temporary bans are held in the gateway and are unavailable while it is down")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return errors.New("the WAF gateway management API is not enabled; restart the WAF gateway to pick up its access token")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("WAF gateway management request failed with status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
