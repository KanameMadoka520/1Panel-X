package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/1Panel-dev/1Panel/agent/utils/files"
)

const oneDriveGlobalBaseURL = "https://graph.microsoft.com/v1.0/"
const oneDriveChinaBaseURL = "https://microsoftgraph.chinacloudapi.cn/v1.0/"
const oneDriveErrorBodyLimit = int64(64 << 10)

const (
	oneDriveMetadataTimeout = 30 * time.Second
	oneDriveTransferTimeout = 2 * time.Hour
	oneDriveChunkTimeout    = 10 * time.Minute
	oneDriveOAuthTimeout    = 15 * time.Second
)

type oneDriveClient struct {
	client  *http.Client
	baseURL *url.URL
	token   string
}

func NewOneDriveClient(vars map[string]interface{}) (*oneDriveClient, error) {
	return NewOneDriveClientWithContext(context.Background(), vars)
}

func NewOneDriveClientWithContext(ctx context.Context, vars map[string]interface{}) (*oneDriveClient, error) {
	token, err := RefreshTokenWithContext(ctx, "refresh_token", "accessToken", vars)
	if err != nil {
		return nil, err
	}

	baseURL := oneDriveGlobalBaseURL
	if loadParamFromVars("isCN", vars) == "true" {
		baseURL = oneDriveChinaBaseURL
	}
	parsedBaseURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse OneDrive base URL failed: %w", err)
	}
	return &oneDriveClient{client: newOneDriveHTTPClient(), baseURL: parsedBaseURL, token: token}, nil
}

func newOneDriveHTTPClient() *http.Client {
	return &http.Client{Timeout: oneDriveTransferTimeout, CheckRedirect: oneDriveRedirectPolicy}
}

func oneDriveRedirectPolicy(_ *http.Request, _ []*http.Request) error {
	return errors.New("OneDrive redirect refused")
}

func (o oneDriveClient) ListBuckets() ([]interface{}, error) { return nil, nil }

func (o oneDriveClient) Exist(itemPath string) (bool, error) {
	_, err := o.loadIDByPath(normalizeDrivePath(itemPath))
	if errors.Is(err, errCloudStorageObjectNotFound) {
		return false, nil
	}
	return err == nil, err
}

func (o oneDriveClient) Size(itemPath string) (int64, error) {
	var item DriveItem
	if err := o.getDriveItem(context.Background(), normalizeDrivePath(itemPath), &item); err != nil {
		return 0, err
	}
	return item.Size, nil
}

func (o oneDriveClient) Delete(itemPath string) (bool, error) {
	itemPath = normalizeDrivePath(itemPath)
	ctx, cancel := context.WithTimeout(context.Background(), oneDriveMetadataTimeout)
	defer cancel()
	if err := o.doJSON(ctx, http.MethodDelete, "me/drive/root:"+escapeDrivePath(itemPath), nil, nil); err != nil {
		return false, fmt.Errorf("delete OneDrive file failed: %w", err)
	}
	return true, nil
}

func (o oneDriveClient) Upload(ctx context.Context, src, target string) (bool, error) {
	ctx, cancel := oneDriveContextWithTimeout(ctx, oneDriveTransferTimeout)
	defer cancel()
	target = normalizeDrivePath(target)
	targetName := path.Base(target)
	if targetName == "" || targetName == "." || targetName == ".." || targetName == "/" {
		return false, errors.New("OneDrive upload target name is invalid")
	}
	parentPath := path.Dir(target)
	if _, err := o.loadIDByPathWithContext(ctx, parentPath); err != nil {
		if !errors.Is(err, errCloudStorageObjectNotFound) {
			return false, err
		}
		if err := o.createFolder(ctx, parentPath); err != nil {
			return false, fmt.Errorf("create directory before upload failed: %w", err)
		}
	}
	folderID, err := o.loadIDByPathWithContext(ctx, parentPath)
	if err != nil {
		return false, err
	}
	fileInfo, err := os.Stat(src)
	if err != nil {
		return false, err
	}
	if fileInfo.IsDir() {
		return false, errors.New("only file is allowed to be uploaded here")
	}
	if fileInfo.Size() < 4*1024*1024 {
		return o.upSmall(ctx, src, targetName, folderID, fileInfo.Size())
	}
	return o.upBig(ctx, src, targetName, folderID, fileInfo.Size())
}

func (o oneDriveClient) Download(src, target string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), oneDriveTransferTimeout)
	defer cancel()
	var item DriveItem
	if err := o.getDriveItem(ctx, normalizeDrivePath(src), &item); err != nil {
		return false, err
	}
	if item.DownloadURL == "" {
		return false, errors.New("OneDrive download URL is missing")
	}
	if err := validateOneDriveDataURL(item.DownloadURL); err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, item.DownloadURL, nil)
	if err != nil {
		return false, errors.New("create OneDrive download request failed")
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return false, newOneDriveTransportError(ctx)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return false, newOneDriveHTTPError(resp)
	}
	if err := writeOneDriveDownloadAtomically(target, resp.Body); err != nil {
		return false, err
	}
	return true, nil
}

func (o *oneDriveClient) ListObjects(prefix string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), oneDriveMetadataTimeout)
	defer cancel()
	folderID, err := o.loadIDByPathWithContext(ctx, normalizeDrivePath(prefix))
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("me/drive/items/%s/children", url.PathEscape(folderID))
	result := make([]string, 0)
	seenPages := make(map[string]struct{})
	for endpoint != "" {
		if _, seen := seenPages[endpoint]; seen {
			return nil, errors.New("list OneDrive files failed: pagination cycle detected")
		}
		seenPages[endpoint] = struct{}{}
		var items oneDriveItemsResponse
		if err := o.doJSON(ctx, http.MethodGet, endpoint, nil, &items); err != nil {
			return nil, fmt.Errorf("list OneDrive files failed: %w", err)
		}
		for _, item := range items.Value {
			result = append(result, item.Name)
		}
		endpoint = items.NextLink
	}
	return result, nil
}

func (o *oneDriveClient) loadIDByPath(itemPath string) (string, error) {
	return o.loadIDByPathWithContext(context.Background(), itemPath)
}

func (o *oneDriveClient) loadIDByPathWithContext(ctx context.Context, itemPath string) (string, error) {
	var item DriveItem
	if err := o.getDriveItem(ctx, itemPath, &item); err != nil {
		return "", err
	}
	if item.ID == "" {
		return "", errors.New("OneDrive item response is missing an ID")
	}
	return item.ID, nil
}

func (o *oneDriveClient) getDriveItem(ctx context.Context, itemPath string, result *DriveItem) error {
	ctx, cancel := oneDriveContextWithTimeout(ctx, oneDriveMetadataTimeout)
	defer cancel()
	endpoint := "me/drive/root"
	if itemPath != "/" {
		endpoint += ":" + escapeDrivePath(itemPath)
	}
	if err := o.doJSON(ctx, http.MethodGet, endpoint, nil, result); err != nil {
		if isOneDriveNotFound(err) {
			return fmt.Errorf("get OneDrive item failed: %w", errCloudStorageObjectNotFound)
		}
		return fmt.Errorf("get OneDrive item failed: %w", err)
	}
	return nil
}

func (o *oneDriveClient) createFolder(ctx context.Context, parent string) error {
	if parent == "/" {
		return nil
	}
	parentID, err := o.loadIDByPathWithContext(ctx, path.Dir(parent))
	if err != nil {
		if !errors.Is(err, errCloudStorageObjectNotFound) {
			return err
		}
		if err := o.createFolder(ctx, path.Dir(parent)); err != nil {
			return err
		}
		parentID, err = o.loadIDByPathWithContext(ctx, path.Dir(parent))
		if err != nil {
			return err
		}
	}
	body := struct {
		Name   string                 `json:"name"`
		Folder map[string]interface{} `json:"folder"`
	}{Name: path.Base(parent), Folder: map[string]interface{}{}}
	endpoint := fmt.Sprintf("me/drive/items/%s/children", url.PathEscape(parentID))
	requestCtx, cancel := oneDriveContextWithTimeout(ctx, oneDriveMetadataTimeout)
	defer cancel()
	return o.doJSON(requestCtx, http.MethodPost, endpoint, body, nil)
}

func (o *oneDriveClient) upSmall(ctx context.Context, srcPath, targetName, folderID string, fileSize int64) (bool, error) {
	ctx, cancel := oneDriveContextWithTimeout(ctx, oneDriveChunkTimeout)
	defer cancel()
	file, err := os.Open(srcPath)
	if err != nil {
		return false, err
	}
	defer file.Close()
	endpoint := fmt.Sprintf("me/drive/items/%s:/%s:/content?@microsoft.graph.conflictBehavior=fail", url.PathEscape(folderID), url.PathEscape(targetName))
	req, err := o.newGraphRequest(ctx, http.MethodPut, endpoint, file)
	if err != nil {
		return false, err
	}
	req.ContentLength = fileSize
	req.Header.Set("Content-Length", strconv.FormatInt(fileSize, 10))
	req.Header.Set("Content-Type", files.GetMimeType(srcPath))
	if err := o.do(req, nil); err != nil {
		return false, fmt.Errorf("upload OneDrive file failed: %w", err)
	}
	return true, nil
}

func (o *oneDriveClient) upBig(ctx context.Context, srcPath, targetName, folderID string, fileSize int64) (bool, error) {
	file, err := os.Open(srcPath)
	if err != nil {
		return false, err
	}
	defer file.Close()
	body := struct {
		Item struct {
			ConflictBehavior string `json:"@microsoft.graph.conflictBehavior"`
		} `json:"item"`
	}{}
	body.Item.ConflictBehavior = "fail"
	var session oneDriveUploadSession
	endpoint := fmt.Sprintf("me/drive/items/%s:/%s:/createUploadSession", url.PathEscape(folderID), url.PathEscape(targetName))
	sessionCtx, cancel := oneDriveContextWithTimeout(ctx, oneDriveMetadataTimeout)
	err = o.doJSON(sessionCtx, http.MethodPost, endpoint, body, &session)
	cancel()
	if err != nil {
		return false, fmt.Errorf("create OneDrive upload session failed: %w", err)
	}
	if session.UploadURL == "" {
		return false, errors.New("OneDrive upload session response is missing an upload URL")
	}
	if err := validateOneDriveDataURL(session.UploadURL); err != nil {
		return false, err
	}

	const chunkSize int64 = 5 * 1024 * 1024
	reader := bufio.NewReader(file)
	buffer := make([]byte, chunkSize)
	for offset := int64(0); offset < fileSize; {
		length, readErr := io.ReadFull(reader, buffer)
		if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) && !errors.Is(readErr, io.EOF) {
			return false, readErr
		}
		if length == 0 {
			return false, io.ErrUnexpectedEOF
		}
		if err := o.uploadChunk(ctx, session.UploadURL, offset, fileSize, buffer[:length]); err != nil {
			return false, err
		}
		offset += int64(length)
	}
	return true, nil
}

func (o *oneDriveClient) uploadChunk(ctx context.Context, uploadURL string, offset, total int64, chunk []byte) error {
	ctx, cancel := oneDriveContextWithTimeout(ctx, oneDriveChunkTimeout)
	defer cancel()
	if err := validateOneDriveDataURL(uploadURL); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(chunk))
	if err != nil {
		return errors.New("create OneDrive upload request failed")
	}
	req.Header.Set("Content-Length", strconv.Itoa(len(chunk)))
	req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, offset+int64(len(chunk))-1, total))
	resp, err := o.client.Do(req)
	if err != nil {
		return newOneDriveTransportError(ctx)
	}
	defer resp.Body.Close()
	isFinalChunk := offset+int64(len(chunk)) == total
	validStatus := (!isFinalChunk && resp.StatusCode == http.StatusAccepted) ||
		(isFinalChunk && (resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated))
	if !validStatus {
		return newOneDriveHTTPError(resp)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, oneDriveErrorBodyLimit))
	return nil
}

func (o *oneDriveClient) doJSON(ctx context.Context, method, endpoint string, body, result interface{}) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := o.newGraphRequest(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return o.do(req, result)
}

func (o *oneDriveClient) newGraphRequest(ctx context.Context, method, endpoint string, body io.Reader) (*http.Request, error) {
	apiURL, err := o.resolveGraphURL(endpoint)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, apiURL.String(), body)
	if err != nil {
		return nil, errors.New("create OneDrive API request failed")
	}
	req.Header.Set("Authorization", "Bearer "+o.token)
	return req, nil
}

func (o *oneDriveClient) do(req *http.Request, result interface{}) error {
	resp, err := o.client.Do(req)
	if err != nil {
		return newOneDriveTransportError(req.Context())
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return newOneDriveHTTPError(resp)
	}
	if result == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, oneDriveErrorBodyLimit))
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(result)
}

type DriveItem struct {
	Name        string `json:"name"`
	ID          string `json:"id"`
	DownloadURL string `json:"@microsoft.graph.downloadUrl"`
	Size        int64  `json:"size"`
}

type oneDriveItemsResponse struct {
	Value    []DriveItem `json:"value"`
	NextLink string      `json:"@odata.nextLink"`
}

type oneDriveUploadSession struct {
	UploadURL string `json:"uploadUrl"`
}

type oneDriveError struct {
	Details struct {
		Code string `json:"code"`
	} `json:"error"`
	statusCode int
}

func (e *oneDriveError) Error() string {
	return fmt.Sprintf("OneDrive API returned HTTP %d", e.statusCode)
}

func newOneDriveHTTPError(resp *http.Response) error {
	apiErr := &oneDriveError{statusCode: resp.StatusCode}
	_ = json.NewDecoder(io.LimitReader(resp.Body, oneDriveErrorBodyLimit)).Decode(apiErr)
	return apiErr
}

func isOneDriveNotFound(err error) bool {
	var apiErr *oneDriveError
	return errors.As(err, &apiErr) && apiErr.statusCode == http.StatusNotFound && apiErr.Details.Code == "itemNotFound"
}

func normalizeDrivePath(itemPath string) string { return "/" + strings.TrimPrefix(itemPath, "/") }

func escapeDrivePath(itemPath string) string {
	parts := strings.Split(strings.TrimPrefix(itemPath, "/"), "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return "/" + strings.Join(parts, "/")
}

func validateOneDriveDataURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" || parsed.User != nil {
		return errors.New("OneDrive data URL is invalid")
	}
	return nil
}

func (o *oneDriveClient) resolveGraphURL(endpoint string) (*url.URL, error) {
	if o.baseURL == nil {
		return nil, errors.New("OneDrive API URL is invalid")
	}
	reference, err := url.Parse(endpoint)
	if err != nil {
		return nil, errors.New("OneDrive API URL is invalid")
	}
	apiURL := o.baseURL.ResolveReference(reference)
	if apiURL.User != nil || !strings.EqualFold(apiURL.Scheme, o.baseURL.Scheme) || !strings.EqualFold(apiURL.Host, o.baseURL.Host) {
		return nil, errors.New("OneDrive API URL is invalid")
	}
	return apiURL, nil
}

func newOneDriveTransportError(ctx context.Context) error {
	if ctx != nil && ctx.Err() != nil {
		return fmt.Errorf("OneDrive request failed: %w", ctx.Err())
	}
	return errors.New("OneDrive request failed")
}

func writeOneDriveDownloadAtomically(target string, source io.Reader) error {
	if target == "" {
		return errors.New("OneDrive download target is invalid")
	}
	tempFile, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	keepTemp := true
	defer func() {
		_ = tempFile.Close()
		if keepTemp {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := io.CopyBuffer(tempFile, source, make([]byte, 2*1024*1024)); err != nil {
		return err
	}
	if err := tempFile.Sync(); err != nil {
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, target); err != nil {
		return err
	}
	keepTemp = false
	return nil
}

func oneDriveContextWithTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, timeout)
}

func RefreshToken(grantType string, tokenType string, varMap map[string]interface{}) (string, error) {
	return RefreshTokenWithContext(context.Background(), grantType, tokenType, varMap)
}

func RefreshTokenWithContext(ctx context.Context, grantType string, tokenType string, varMap map[string]interface{}) (string, error) {
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
	token, err := requestOneDriveOAuthToken(ctx, tokenURL, data)
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

func requestOneDriveOAuthToken(ctx context.Context, endpoint string, form url.Values) (oauthTokenResponse, error) {
	requestCtx, cancel := oneDriveContextWithTimeout(ctx, oneDriveOAuthTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return oauthTokenResponse{}, errors.New("OAuth token request failed")
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")

	response, err := oauthTokenHTTPClient.Do(request)
	if err != nil {
		if requestCtx.Err() != nil {
			return oauthTokenResponse{}, fmt.Errorf("OAuth token request failed: %w", requestCtx.Err())
		}
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
