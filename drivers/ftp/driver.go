package ftp

import (
	"context"
	"errors"
	"io"
	"net/textproto"
	stdpath "path"
	"strings"
	"sync"

	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/errs"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/stream"
	"github.com/OpenListTeam/OpenList/v4/pkg/http_range"
	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
	"github.com/jlaffaye/ftp"
)

type FTP struct {
	model.Storage
	Addition
	conn       *ftp.ServerConn
	mu         sync.Mutex
	connCancel context.CancelFunc

	ctx    context.Context
	cancel context.CancelFunc
}

func (d *FTP) Config() driver.Config {
	return config
}

func (d *FTP) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *FTP) Init(ctx context.Context) error {
	if err := d.validate(); err != nil {
		return err
	}
	d.ctx, d.cancel = context.WithCancel(context.Background())
	release, err := d.acquire(ctx)
	if err != nil {
		d.cancel()
		return err
	}
	release()
	return nil
}

func (d *FTP) Drop(ctx context.Context) error {
	if d.cancel != nil {
		d.cancel()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.connCancel != nil {
		d.connCancel()
		d.connCancel = nil
	}
	if d.conn != nil {
		_ = d.conn.Quit()
		d.conn = nil
	}
	return nil
}

func (d *FTP) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	release, err := d.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	path := encode(dir.GetPath(), d.Encoding)

	var entries []*ftp.Entry

	if d.CwdList {
		origDir, cwdErr := d.conn.CurrentDir()
		if cwdErr != nil {
			return nil, cwdErr
		}
		if cwdErr = d.conn.ChangeDir(path); cwdErr != nil {
			return nil, cwdErr
		}
		entries, err = d.conn.List("")
		if restoreErr := d.conn.ChangeDir(origDir); restoreErr != nil {
			d.connCancel()
			d.conn = nil
		}
	} else {
		entries, err = d.conn.List(path)
	}

	if err != nil {
		return nil, err
	}

	res := make([]model.Obj, 0)
	for _, entry := range entries {
		if entry.Name == "." || entry.Name == ".." || strings.Contains(entry.Name, "/") {
			continue
		}
		name := decode(entry.Name, d.Encoding)
		f := model.Object{
			Name:     name,
			Size:     int64(entry.Size),
			Modified: entry.Time,
			IsFolder: entry.Type == ftp.EntryTypeFolder,
			Path:     stdpath.Join(dir.GetPath(), name),
		}
		res = append(res, &f)
	}
	return res, nil
}

func (d *FTP) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	remotePath := encode(file.GetPath(), d.Encoding)
	size := file.GetSize()
	reader := func(ctx context.Context, r http_range.Range) (io.ReadCloser, error) {
		if size < 0 || r.Start < 0 || r.Length < -1 {
			return nil, http_range.ErrInvalid
		}
		if r.Start > size || (size > 0 && r.Start == size) {
			return nil, http_range.ErrNoOverlap
		}
		length := r.Length
		if length < 0 || length > size-r.Start {
			length = size - r.Start
		}
		if length == 0 {
			return io.NopCloser(strings.NewReader("")), nil
		}
		conn, closeConn, err := d.connect(ctx)
		if err != nil {
			return nil, err
		}
		resp, err := conn.RetrFrom(remotePath, uint64(r.Start))
		if err != nil {
			closeConn()
			return nil, err
		}
		var once sync.Once
		closeReader := func() error {
			var closeErr error
			once.Do(func() { closeErr = resp.Close(); closeConn() })
			return closeErr
		}
		return utils.ReadCloser{Reader: io.LimitReader(resp, length), Closer: utils.CloseFunc(closeReader)}, nil
	}
	return &model.Link{RangeReader: stream.RateLimitRangeReaderFunc(reader)}, nil
}

// MLST is optional. Let OpenList resolve through the parent listing when the
// server lacks it or returns an ambiguous 550 (missing or inaccessible).
func (d *FTP) Get(ctx context.Context, p string) (model.Obj, error) {
	release, err := d.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	p = stdpath.Join(d.RootFolderPath, p)
	entry, err := d.conn.GetEntry(encode(p, d.Encoding))
	if err != nil {
		var protocolErr *textproto.Error
		if errors.As(err, &protocolErr) && (protocolErr.Code == 500 || protocolErr.Code == 501 || protocolErr.Code == 502 || protocolErr.Code == 504 || protocolErr.Code == 550) {
			return nil, errs.NotImplement
		}
		return nil, err
	}
	if entry.Type == ftp.EntryTypeLink {
		return nil, errs.NotImplement
	}
	return &model.Object{Name: stdpath.Base(p), Path: p, Size: int64(entry.Size), Modified: entry.Time, IsFolder: entry.Type == ftp.EntryTypeFolder}, nil
}

func (d *FTP) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	release, err := d.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	return d.conn.MakeDir(encode(stdpath.Join(parentDir.GetPath(), dirName), d.Encoding))
}

func (d *FTP) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	release, err := d.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	return d.conn.Rename(
		encode(srcObj.GetPath(), d.Encoding),
		encode(stdpath.Join(dstDir.GetPath(), srcObj.GetName()), d.Encoding),
	)
}

func (d *FTP) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	release, err := d.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	return d.conn.Rename(
		encode(srcObj.GetPath(), d.Encoding),
		encode(stdpath.Join(stdpath.Dir(srcObj.GetPath()), newName), d.Encoding),
	)
}

func (d *FTP) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	return errs.NotSupport
}

func (d *FTP) Remove(ctx context.Context, obj model.Obj) error {
	release, err := d.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	path := encode(obj.GetPath(), d.Encoding)
	if obj.IsDir() {
		return d.conn.RemoveDirRecur(path)
	} else {
		return d.conn.Delete(path)
	}
}

func (d *FTP) Put(ctx context.Context, dstDir model.Obj, s model.FileStreamer, up driver.UpdateProgress) error {
	conn, closeConn, err := d.connect(ctx)
	if err != nil {
		return err
	}
	defer closeConn()
	p := stdpath.Join(dstDir.GetPath(), s.GetName())
	err = conn.Stor(encode(p, d.Encoding), driver.NewLimitedUploadStream(ctx, &driver.ReaderUpdatingProgress{Reader: s, UpdateProgress: up}))
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

var _ driver.Driver = (*FTP)(nil)
var _ driver.Getter = (*FTP)(nil)
