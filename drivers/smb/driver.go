package smb

import (
	"context"
	"errors"
	"github.com/OpenListTeam/OpenList/v4/pkg/http_range"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/errs"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/stream"
	"github.com/OpenListTeam/OpenList/v4/pkg/utils"

	"github.com/cloudsoda/go-smb2"
	"github.com/google/uuid"
)

type SMB struct {
	lastConnTime atomic.Int64
	model.Storage
	Addition
	mu      sync.Mutex
	current *shareLease
	leases  map[*shareLease]struct{}
	closed  bool
}

func (d *SMB) Config() driver.Config {
	return config
}

func (d *SMB) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *SMB) Init(ctx context.Context) error {
	if _, _, err := net.SplitHostPort(d.Address); err != nil {
		d.Address = net.JoinHostPort(strings.Trim(d.Address, "[]"), "445")
	}
	d.mu.Lock()
	d.closed = false
	d.mu.Unlock()
	_, release, err := d.acquireShare(ctx)
	if err != nil {
		return err
	}
	release()
	return nil
}

func (d *SMB) Drop(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	for lease := range d.leases {
		_ = lease.conn.Close()
	}
	d.current = nil
	d.leases = nil
	return nil
}

func (d *SMB) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	share, release, err := d.acquireShare(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	fullPath := dir.GetPath()
	rawFiles, err := share.ReadDir(fullPath)
	if err != nil {
		d.cleanLastConnTime()
		return nil, err
	}
	d.updateLastConnTime()
	files := make([]model.Obj, 0, len(rawFiles))
	for _, f := range rawFiles {
		file := model.Object{
			Path:     path.Join(fullPath, f.Name()),
			Name:     f.Name(),
			Modified: f.ModTime(),
			Size:     f.Size(),
			IsFolder: f.IsDir(),
		}

		if stat, ok := f.(*smb2.FileStat); ok {
			file.Ctime = stat.CreationTime
		}
		files = append(files, &file)
	}
	return files, nil
}

func (d *SMB) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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
		share, release, err := d.acquireShare(ctx)
		if err != nil {
			return nil, err
		}
		remote, err := share.Open(file.GetPath())
		if err != nil {
			release()
			return nil, err
		}
		var once sync.Once
		closeReader := func() error {
			var err error
			once.Do(func() { err = remote.Close(); release() })
			return err
		}
		return utils.ReadCloser{Reader: io.NewSectionReader(remote, r.Start, length), Closer: utils.CloseFunc(closeReader)}, nil
	}
	return &model.Link{RangeReader: stream.RateLimitRangeReaderFunc(reader)}, nil
}

func (d *SMB) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	share, release, err := d.acquireShare(ctx)
	if err != nil {
		return err
	}
	defer release()
	fullPath := filepath.Join(parentDir.GetPath(), dirName)
	err = share.MkdirAll(fullPath, 0700)
	if err != nil {
		d.cleanLastConnTime()
		return err
	}
	d.updateLastConnTime()
	return nil
}

func (d *SMB) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	share, release, err := d.acquireShare(ctx)
	if err != nil {
		return err
	}
	defer release()
	srcPath := srcObj.GetPath()
	dstPath := filepath.Join(dstDir.GetPath(), srcObj.GetName())
	err = share.Rename(srcPath, dstPath)
	if err != nil {
		d.cleanLastConnTime()
		return err
	}
	d.updateLastConnTime()
	return nil
}

func (d *SMB) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	share, release, err := d.acquireShare(ctx)
	if err != nil {
		return err
	}
	defer release()
	srcPath := srcObj.GetPath()
	dstPath := filepath.Join(filepath.Dir(srcPath), newName)
	err = share.Rename(srcPath, dstPath)
	if err != nil {
		d.cleanLastConnTime()
		return err
	}
	d.updateLastConnTime()
	return nil
}

func (d *SMB) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	share, release, err := d.acquireShare(ctx)
	if err != nil {
		return err
	}
	defer release()
	srcPath := srcObj.GetPath()
	dstPath := filepath.Join(dstDir.GetPath(), srcObj.GetName())
	if overlappingCopy(srcPath, dstPath, srcObj.IsDir()) {
		return errors.New("cannot copy an SMB path onto itself or into its descendants")
	}
	if srcObj.IsDir() {
		err = copyDir(ctx, share, srcPath, dstPath)
	} else {
		err = copyFile(ctx, share, srcPath, dstPath)
	}
	if err != nil {
		d.cleanLastConnTime()
		return err
	}
	d.updateLastConnTime()
	return nil
}

func (d *SMB) Remove(ctx context.Context, obj model.Obj) error {
	share, release, err := d.acquireShare(ctx)
	if err != nil {
		return err
	}
	defer release()
	fullPath := obj.GetPath()
	if obj.IsDir() {
		err = share.RemoveAll(fullPath)
	} else {
		err = share.Remove(fullPath)
	}
	if err != nil {
		d.cleanLastConnTime()
		return err
	}
	d.updateLastConnTime()
	return nil
}

func (d *SMB) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) error {
	share, release, err := d.acquireShare(ctx)
	if err != nil {
		return err
	}
	defer release()
	fullPath := filepath.Join(dstDir.GetPath(), stream.GetName())
	target := fullPath
	staged := d.AtomicUpload
	if staged {
		info, statErr := share.Lstat(fullPath)
		if statErr != nil && !os.IsNotExist(statErr) {
			return statErr
		}
		// Keep the existing behavior for symlinks instead of replacing the link.
		if statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			staged = false
		}
	}
	if staged {
		target = filepath.Join(dstDir.GetPath(), ".openlist-upload-"+uuid.NewString())
	}
	var out *smb2.File
	if staged {
		out, err = share.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0666)
	} else {
		out, err = share.Create(target)
	}
	if err != nil {
		d.cleanLastConnTime()
		return err
	}
	if staged {
		defer func() {
			if target == "" {
				return
			}
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = share.WithContext(cleanupCtx).Remove(target)
		}()
	}
	d.updateLastConnTime()
	err = utils.CopyWithCtx(ctx, out, driver.NewLimitedUploadStream(ctx, stream), stream.GetSize(), up)
	closeErr := out.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if staged {
		if err := share.Rename(target, fullPath); err != nil {
			return err
		}
		target = ""
	}
	return nil
}

func (d *SMB) GetDetails(ctx context.Context) (*model.StorageDetails, error) {
	share, release, err := d.acquireShare(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	stat, err := share.Statfs(d.RootFolderPath)
	if err != nil {
		return nil, err
	}
	total := int64(stat.BlockSize() * stat.TotalBlockCount())
	free := int64(stat.BlockSize() * stat.AvailableBlockCount())
	return &model.StorageDetails{
		DiskUsage: model.DiskUsage{
			TotalSpace: total,
			UsedSpace:  total - free,
		},
	}, nil
}

//func (d *SMB) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
//	return nil, errs.NotSupport
//}

var _ driver.Driver = (*SMB)(nil)

func (d *SMB) Get(ctx context.Context, p string) (model.Obj, error) {
	share, release, err := d.acquireShare(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	p = path.Join(d.RootFolderPath, p)
	info, err := share.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errs.ObjectNotFound
		}
		return nil, err
	}
	obj := &model.Object{Path: p, Name: info.Name(), Size: info.Size(), Modified: info.ModTime(), IsFolder: info.IsDir()}
	if stat, ok := info.(*smb2.FileStat); ok {
		obj.Ctime = stat.CreationTime
	}
	return obj, nil
}

var _ driver.Getter = (*SMB)(nil)
var _ driver.WithDetails = (*SMB)(nil)
