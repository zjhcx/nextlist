package webdav

import (
	"context"
	"fmt"
	"net/http"
	neturl "net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/OpenListTeam/OpenList/v4/drivers/base"
	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/errs"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/pkg/cron"
	"github.com/OpenListTeam/OpenList/v4/pkg/gowebdav"
	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
)

type WebDav struct {
	model.Storage
	Addition
	client   *gowebdav.Client
	cron     *cron.Cron
	clientMu sync.RWMutex
}

func (d *WebDav) Config() driver.Config {
	return config
}

func (d *WebDav) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *WebDav) Init(ctx context.Context) error {
	err := d.setClient()
	if err == nil {
		d.cron = cron.NewCron(time.Hour * 12)
		d.cron.Do(func() {
			_ = d.setClient()
		})
	}
	return err
}

func (d *WebDav) Drop(ctx context.Context) error {
	if d.cron != nil {
		d.cron.Stop()
	}
	return nil
}

func (d *WebDav) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	files, err := d.requestClient(ctx).ReadDir(dir.GetPath())
	if err != nil {
		return nil, err
	}
	return utils.SliceConvert(files, func(src os.FileInfo) (model.Obj, error) {
		return &model.Object{
			Path:     path.Join(dir.GetPath(), src.Name()),
			Name:     src.Name(),
			Size:     src.Size(),
			Modified: src.ModTime(),
			IsFolder: src.IsDir(),
		}, nil
	})
}

func (d *WebDav) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	url, header, err := d.requestClient(ctx).Link(file.GetPath())
	if err != nil {
		return nil, err
	}
	if args.Redirect {
		// get the url after redirect
		req := base.NoRedirectClient.R().SetContext(ctx)
		req.Header = header
		req.SetDoNotParseResponse(true)
		res, err := req.Get(url)
		if err != nil {
			return nil, err
		}
		_ = res.RawResponse.Body.Close()
		if (res.StatusCode() == 302 || res.StatusCode() == 307 || res.StatusCode() == 308) && res.Header().Get("location") != "" {
			origin, err := neturl.Parse(url)
			if err != nil {
				return nil, err
			}
			target, err := origin.Parse(res.Header().Get("location"))
			if err != nil {
				return nil, err
			}
			if (target.Scheme != "http" && target.Scheme != "https") || target.User != nil {
				return nil, fmt.Errorf("invalid WebDAV redirect")
			}
			if !strings.EqualFold(origin.Host, target.Host) || !strings.EqualFold(origin.Scheme, target.Scheme) {
				header = header.Clone()
				header.Del("Authorization")
				header.Del("Cookie")
			}
			url = target.String()
		} else {
			return nil, fmt.Errorf("redirect failed, status: %d", res.StatusCode())
		}
	}
	return &model.Link{
		URL:    url,
		Header: header,
	}, nil
}

func (d *WebDav) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	return d.requestClient(ctx).MkdirAll(path.Join(parentDir.GetPath(), dirName), 0644)
}

func (d *WebDav) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	return d.requestClient(ctx).Rename(getPath(srcObj), path.Join(dstDir.GetPath(), srcObj.GetName()), true)
}

func (d *WebDav) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	return d.requestClient(ctx).Rename(getPath(srcObj), path.Join(path.Dir(srcObj.GetPath()), newName), true)
}

func (d *WebDav) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	return d.requestClient(ctx).Copy(getPath(srcObj), path.Join(dstDir.GetPath(), srcObj.GetName()), true)
}

func (d *WebDav) Remove(ctx context.Context, obj model.Obj) error {
	return d.requestClient(ctx).RemoveAll(getPath(obj))
}

func (d *WebDav) Put(ctx context.Context, dstDir model.Obj, s model.FileStreamer, up driver.UpdateProgress) error {
	callback := func(r *http.Request) {
		r.Header.Set("Content-Type", s.GetMimetype())
		r.ContentLength = s.GetSize()
	}
	reader := driver.NewLimitedUploadStream(ctx, &driver.ReaderUpdatingProgress{
		Reader:         s,
		UpdateProgress: up,
	})
	err := d.requestClient(ctx).WriteStream(path.Join(dstDir.GetPath(), s.GetName()), reader, 0644, callback)
	return err
}

// implements driver.Getter interface
func (d *WebDav) Get(ctx context.Context, _path string) (model.Obj, error) {
	_path = path.Join(d.GetRootPath(), _path)
	info, err := d.requestClient(ctx).Stat(_path)
	if err != nil && !gowebdav.IsErrNotFound(err) && !strings.HasSuffix(_path, "/") {
		// As in RFC 4918 Section 5.2, retry failed slash-less collection PROPFINDs
		// once with the canonical trailing-slash URL
		info, err = d.requestClient(ctx).Stat(_path + "/")
	}
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return nil, errs.ObjectNotFound
		}
		return nil, err
	}

	return &model.Object{
		Name:     info.Name(),
		Size:     info.Size(),
		Modified: info.ModTime(),
		IsFolder: info.IsDir(),
		Path:     _path,
	}, nil
}

var _ driver.Driver = (*WebDav)(nil)
var _ driver.Getter = (*WebDav)(nil)
