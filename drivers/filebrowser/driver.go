package filebrowser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/OpenListTeam/OpenList/v4/drivers/base"
	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/errs"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
)

type FileBrowser struct {
	model.Storage
	Addition
}

func (d *FileBrowser) Config() driver.Config          { return config }
func (d *FileBrowser) GetAddition() driver.Additional { return &d.Addition }

func (d *FileBrowser) Init(ctx context.Context) error {
	d.Address = strings.TrimRight(strings.TrimSpace(d.Address), "/")
	u, err := url.Parse(d.Address)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return errors.New("address must be an HTTP(S) site URL without credentials, query or fragment")
	}
	if d.Auth == "" || d.Source == "" {
		return errors.New("auth and source are required")
	}
	switch d.Checksum {
	case "", "none", "md5", "sha1", "sha256":
	default:
		return errors.New("unsupported FileBrowser checksum algorithm")
	}
	if d.RootFolderPath == "" {
		d.RootFolderPath = "/"
	}
	d.RootFolderPath = path.Clean("/" + d.RootFolderPath)
	_, err = d.List(ctx, &model.Object{Path: d.RootFolderPath}, model.ListArgs{})
	return err
}

func (d *FileBrowser) Drop(ctx context.Context) error { return nil }

type item struct {
	Name       string            `json:"name"`
	Size       int64             `json:"size"`
	Modified   time.Time         `json:"modified"`
	Created    time.Time         `json:"created"`
	Type       string            `json:"type"`
	HasPreview bool              `json:"hasPreview"`
	Checksums  map[string]string `json:"checksums"`
}

func (d *FileBrowser) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	var listing struct {
		Type    string `json:"type"`
		Files   []item `json:"files"`
		Folders []item `json:"folders"`
	}
	err := d.request(ctx, http.MethodGet, "/api/resources", url.Values{"path": {dir.GetPath()}, "content": {"false"}, "metadata": {"false"}}, nil, 0, "", &listing)
	if err != nil {
		return nil, err
	}
	if listing.Type != "directory" {
		return nil, errors.New("resource is not a directory")
	}
	objects := make([]model.Obj, 0, len(listing.Folders)+len(listing.Files))
	appendItems := func(items []item, isDir bool) {
		for _, f := range items {
			if checkName(f.Name) != nil {
				continue
			}
			objects = append(objects, d.object(f, path.Join(dir.GetPath(), f.Name), isDir))
		}
	}
	appendItems(listing.Folders, true)
	appendItems(listing.Files, false)
	return objects, nil
}

func (d *FileBrowser) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := d.checkPath(file.GetPath()); err != nil {
		return nil, err
	}
	return &model.Link{URL: d.apiURL("/api/resources/download", url.Values{"file": {file.GetPath()}})}, nil
}

func (d *FileBrowser) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	if err := checkName(dirName); err != nil {
		return err
	}
	return d.request(ctx, http.MethodPost, "/api/resources", url.Values{"path": {path.Join(parentDir.GetPath(), dirName)}, "isDir": {"true"}}, nil, 0, "", nil)
}

func (d *FileBrowser) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	if err := checkName(srcObj.GetName()); err != nil {
		return err
	}
	return d.patch(ctx, "move", srcObj.GetPath(), path.Join(dstDir.GetPath(), srcObj.GetName()))
}

func (d *FileBrowser) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	if err := checkName(newName); err != nil {
		return err
	}
	return d.patch(ctx, "rename", srcObj.GetPath(), path.Join(path.Dir(srcObj.GetPath()), newName))
}

func (d *FileBrowser) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	if err := checkName(srcObj.GetName()); err != nil {
		return err
	}
	return d.patch(ctx, "copy", srcObj.GetPath(), path.Join(dstDir.GetPath(), srcObj.GetName()))
}

func (d *FileBrowser) Remove(ctx context.Context, obj model.Obj) error {
	if path.Clean(obj.GetPath()) == d.RootFolderPath {
		return errors.New("cannot remove the mounted root")
	}
	return d.request(ctx, http.MethodDelete, "/api/resources", url.Values{"path": {obj.GetPath()}}, nil, 0, "", nil)
}

func (d *FileBrowser) Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress) error {
	if err := checkName(file.GetName()); err != nil {
		return err
	}
	if file.GetSize() < 0 {
		return errors.New("FileBrowser upload requires a known file size")
	}
	if up == nil {
		up = func(float64) {}
	}
	var reader io.Reader = http.NoBody
	if file.GetSize() != 0 {
		reader = driver.NewLimitedUploadStream(ctx, &driver.ReaderUpdatingProgress{Reader: file, UpdateProgress: up})
	}
	err := d.request(ctx, http.MethodPost, "/api/resources", url.Values{"path": {path.Join(dstDir.GetPath(), file.GetName())}, "override": {"true"}}, reader, file.GetSize(), "application/octet-stream", nil)
	if err == nil {
		up(100)
	}
	return err
}

type moveCopyItem struct {
	FromSource string `json:"fromSource"`
	FromPath   string `json:"fromPath"`
	ToSource   string `json:"toSource"`
	ToPath     string `json:"toPath"`
	Message    string `json:"message,omitempty"`
}

func (d *FileBrowser) patch(ctx context.Context, action, src, dst string) error {
	if err := d.checkPath(src); err != nil {
		return err
	}
	if err := d.checkPath(dst); err != nil {
		return err
	}
	if path.Clean(src) == d.RootFolderPath {
		return errors.New("cannot move, rename or copy the mounted root")
	}
	body, err := json.Marshal(struct {
		Action string         `json:"action"`
		Items  []moveCopyItem `json:"items"`
	}{Action: action, Items: []moveCopyItem{{FromSource: d.Source, FromPath: src, ToSource: d.Source, ToPath: dst}}})
	if err != nil {
		return err
	}
	var result struct {
		Failed    []moveCopyItem `json:"failed"`
		Succeeded []moveCopyItem `json:"succeeded"`
	}
	err = d.request(ctx, http.MethodPatch, "/api/resources", nil, bytes.NewReader(body), int64(len(body)), "application/json", &result)
	if err != nil {
		return err
	}
	if len(result.Failed) != 0 || len(result.Succeeded) != 1 {
		return fmt.Errorf("FileBrowser %s failed", action)
	}
	return nil
}

func (d *FileBrowser) apiURL(endpoint string, query url.Values) string {
	if query == nil {
		query = make(url.Values)
	}
	query.Set("auth", d.Auth)
	query.Set("source", d.Source)
	return d.Address + endpoint + "?" + query.Encode()
}

func (d *FileBrowser) request(ctx context.Context, method, endpoint string, query url.Values, body io.Reader, size int64, contentType string, result any) error {
	for _, key := range []string{"path", "file"} {
		for _, p := range query[key] {
			if err := d.checkPath(p); err != nil {
				return err
			}
		}
	}
	if method == http.MethodGet {
		var timeoutCancel context.CancelFunc
		ctx, timeoutCancel = context.WithTimeout(ctx, time.Minute)
		defer timeoutCancel()
	}
	req, err := http.NewRequestWithContext(ctx, method, d.apiURL(endpoint, query), body)
	if err != nil {
		return errors.New("invalid FileBrowser request URL")
	}
	req.ContentLength = size
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if contentType == "application/octet-stream" {
		// Quantum stages and verifies content uploads before replacing the target.
		// OpenList multipart Sessions supply a non-seekable stream of known size;
		// read it once and let their manager handle cancellation and retries.
		req.Header.Set("X-File-Total-Size", strconv.FormatInt(size, 10))
	}
	client := *base.HttpClient
	client.Jar = nil // Quantum cookies take precedence over the configured auth token.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := client.Do(req)
	if err != nil {
		// URL errors include the auth query parameter; keep the token out of logs.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return fmt.Errorf("FileBrowser request failed: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 || res.StatusCode == http.StatusAccepted {
		switch res.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden, http.StatusAccepted:
			return fmt.Errorf("FileBrowser %s failed: HTTP %d: %w", method, res.StatusCode, errs.PermissionDenied)
		case http.StatusNotFound:
			return errs.ObjectNotFound
		}
		return fmt.Errorf("FileBrowser %s failed: HTTP %d", method, res.StatusCode)
	}
	if result != nil {
		if err := json.NewDecoder(io.LimitReader(res.Body, 64<<20)).Decode(result); err != nil {
			return fmt.Errorf("invalid FileBrowser response: %w", err)
		}
	} else {
		_, err = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
	}
	return err
}

var _ driver.Driver = (*FileBrowser)(nil)

func checkName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return fmt.Errorf("invalid FileBrowser file name: %w", errs.PermissionDenied)
	}
	return nil
}

func (d *FileBrowser) checkPath(p string) error {
	if !strings.HasPrefix(p, "/") || strings.ContainsAny(p, "\\\x00") {
		return fmt.Errorf("invalid FileBrowser path: %w", errs.PermissionDenied)
	}
	clean := path.Clean(p)
	root := d.RootFolderPath
	if root != "/" && clean != root && !strings.HasPrefix(clean, root+"/") {
		return fmt.Errorf("FileBrowser path is outside the mounted root: %w", errs.PermissionDenied)
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return fmt.Errorf("invalid FileBrowser path traversal: %w", errs.PermissionDenied)
		}
	}
	return nil
}
