package filebrowser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/errs"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
)

func (d *FileBrowser) object(f item, filePath string, isDir bool) model.Obj {
	hashes := make(map[*utils.HashType]string)
	for name, value := range f.Checksums {
		if ht, ok := utils.GetHashByName(name); ok {
			hashes[ht] = value
		}
	}
	o := model.Object{Name: f.Name, Path: filePath, Size: f.Size, Modified: f.Modified, Ctime: f.Created, IsFolder: isDir, HashInfo: utils.NewHashInfoByMap(hashes)}
	if d.Thumbnail && f.HasPreview && !isDir {
		return &model.ObjThumb{Object: o, Thumbnail: model.Thumbnail{Thumbnail: d.apiURL("/api/resources/preview", url.Values{"path": {filePath}, "size": {"small"}})}}
	}
	return &o
}

// Get avoids listing the parent directory to resolve a single object.
func (d *FileBrowser) Get(ctx context.Context, filePath string) (model.Obj, error) {
	// Getter receives a path relative to the mount, unlike List and Link.
	if !strings.HasPrefix(filePath, "/") || strings.ContainsAny(filePath, "\\\x00") {
		return nil, errs.RelativePath
	}
	for _, part := range strings.Split(filePath, "/") {
		if part == ".." {
			return nil, errs.PermissionDenied
		}
	}
	filePath = path.Join(d.RootFolderPath, filePath)
	query := url.Values{"path": {filePath}, "content": {"false"}, "metadata": {"false"}}
	if d.Checksum != "" && d.Checksum != "none" {
		query.Set("checksum", d.Checksum)
	}
	var f item
	if err := d.request(ctx, http.MethodGet, "/api/resources", query, nil, 0, "", &f); err != nil {
		return nil, err
	}
	if f.Type == "" {
		return nil, errors.New("invalid FileBrowser resource metadata")
	}
	// The source root can have an empty display name.
	f.Name = path.Base(filePath)
	return d.object(f, filePath, f.Type == "directory"), nil
}

func (d *FileBrowser) GetDirectUploadTools() []string {
	if !d.EnableDirectUpload {
		return nil
	}
	return []string{"HttpDirect"}
}

func (d *FileBrowser) GetDirectUploadInfo(ctx context.Context, tool string, dstDir model.Obj, fileName string, fileSize int64) (any, error) {
	if !d.EnableDirectUpload || tool != "HttpDirect" {
		return nil, errs.NotImplement
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := checkName(fileName); err != nil {
		return nil, err
	}
	if fileSize < 0 {
		return nil, errors.New("FileBrowser upload requires a known file size")
	}
	destination := path.Join(dstDir.GetPath(), fileName)
	if err := d.checkPath(destination); err != nil {
		return nil, err
	}
	return &model.HttpDirectUploadInfo{
		UploadURL: d.apiURL("/api/resources", url.Values{"path": {destination}, "override": {"true"}}),
		Method:    http.MethodPost,
		Headers:   map[string]string{"Content-Type": "application/octet-stream", "X-File-Total-Size": strconv.FormatInt(fileSize, 10)},
	}, nil
}

func (d *FileBrowser) ArchiveDecompress(ctx context.Context, srcObj, dstDir model.Obj, args model.ArchiveDecompressArgs) error {
	// Quantum always overwrites matching entries and has no password or
	// inner-path extraction options. Preserve OpenList's semantics via fallback.
	if !d.NativeArchive || !args.Overwrite || args.Password != "" || (args.InnerPath != "" && args.InnerPath != "/") {
		return errs.NotImplement
	}
	name := srcObj.GetName()
	lower := strings.ToLower(name)
	if !strings.HasSuffix(lower, ".zip") && !strings.HasSuffix(lower, ".tar.gz") && !strings.HasSuffix(lower, ".tgz") {
		return errs.NotImplement
	}
	if err := d.checkPath(srcObj.GetPath()); err != nil {
		return err
	}
	destination := dstDir.GetPath()
	if err := d.checkPath(destination); err != nil {
		return err
	}
	if args.PutIntoNewDir {
		// Match OpenList's directory naming and update-hook convention.
		name = strings.TrimSuffix(name, path.Ext(name))
		if err := checkName(name); err != nil {
			return err
		}
		if err := d.MakeDir(ctx, dstDir, name); err != nil {
			return err
		}
		destination = path.Join(destination, name)
	}
	body, err := json.Marshal(struct {
		FromSource  string `json:"fromSource"`
		ToSource    string `json:"toSource"`
		Path        string `json:"path"`
		Destination string `json:"destination"`
	}{d.Source, d.Source, srcObj.GetPath(), destination})
	if err != nil {
		return err
	}
	return d.request(ctx, http.MethodPost, "/api/resources/unarchive", nil, bytes.NewReader(body), int64(len(body)), "application/json", nil)
}

var (
	_ driver.Getter            = (*FileBrowser)(nil)
	_ driver.DirectUploader    = (*FileBrowser)(nil)
	_ driver.ArchiveDecompress = (*FileBrowser)(nil)
)
