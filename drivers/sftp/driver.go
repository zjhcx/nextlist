package sftp

import (
	"context"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/errs"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/stream"
	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
	"github.com/google/uuid"
	"github.com/pkg/sftp"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

type SFTP struct {
	model.Storage
	Addition
	client     *sftp.Client
	connMu     sync.Mutex
	sshClient  *ssh.Client
	clientDone <-chan struct{}
	closed     bool
	lifetime   context.Context
	cancel     context.CancelFunc
}

func (d *SFTP) Config() driver.Config {
	return config
}

func (d *SFTP) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *SFTP) Init(ctx context.Context) error {
	d.connMu.Lock()
	d.lifetime, d.cancel = context.WithCancel(context.Background())
	d.closed = false
	d.connMu.Unlock()
	_, err := d.getClient(ctx)
	return err
}

func (d *SFTP) Drop(ctx context.Context) error {
	if d.cancel != nil {
		d.cancel()
	}
	d.connMu.Lock()
	defer d.connMu.Unlock()
	d.closed = true
	if d.sshClient != nil {
		_ = d.sshClient.Close()
		d.sshClient = nil
	}
	if d.client != nil {
		_ = d.client.Close()
		d.client = nil
	}
	return nil
}

func (d *SFTP) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	client, err := d.getClient(ctx)
	if err != nil {
		return nil, err
	}
	log.Debugf("[sftp] list dir: %s", dir.GetPath())
	files, err := client.ReadDirContext(ctx, dir.GetPath())
	if err != nil {
		return nil, err
	}
	objs, err := utils.SliceConvert(files, func(src os.FileInfo) (model.Obj, error) {
		return d.fileToObj(client, src, dir.GetPath())
	})
	return objs, err
}

func (d *SFTP) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	client, err := d.getClient(ctx)
	if err != nil {
		return nil, err
	}
	remoteFile, err := client.Open(file.GetPath())
	if err != nil {
		return nil, err
	}
	mFile := &stream.RateLimitFile{
		File:    remoteFile,
		Limiter: stream.ServerDownloadLimit,
		Ctx:     ctx,
	}
	return &model.Link{
		RangeReader:      stream.GetRangeReaderFromMFile(file.GetSize(), mFile),
		SyncClosers:      utils.NewSyncClosers(remoteFile),
		RequireReference: true,
	}, nil
}

func (d *SFTP) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	client, err := d.getClient(ctx)
	if err != nil {
		return err
	}
	return client.MkdirAll(path.Join(parentDir.GetPath(), dirName))
}

func (d *SFTP) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	client, err := d.getClient(ctx)
	if err != nil {
		return err
	}
	return client.Rename(srcObj.GetPath(), path.Join(dstDir.GetPath(), srcObj.GetName()))
}

func (d *SFTP) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	client, err := d.getClient(ctx)
	if err != nil {
		return err
	}
	return client.Rename(srcObj.GetPath(), path.Join(path.Dir(srcObj.GetPath()), newName))
}

func (d *SFTP) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	return errs.NotSupport
}

func (d *SFTP) Remove(ctx context.Context, obj model.Obj) error {
	client, err := d.getClient(ctx)
	if err != nil {
		return err
	}
	return remove(ctx, client, obj.GetPath())
}

func (d *SFTP) Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress) error {
	// Give uploads their own transport so cancellation can interrupt blocked SSH
	// writes without disconnecting unrelated metadata requests and downloads.
	client, transport, err := d.connect(ctx)
	if err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { _ = transport.Close() })
	stopMount := context.AfterFunc(d.lifetime, func() { _ = transport.Close() })
	defer func() { stop(); stopMount(); _ = transport.Close(); _ = client.Close() }()
	fullPath := path.Join(dstDir.GetPath(), file.GetName())
	_, supported := client.HasExtension("posix-rename@openssh.com")
	staged := d.AtomicUpload && supported
	if staged {
		info, statErr := client.Lstat(fullPath)
		if statErr != nil && !os.IsNotExist(statErr) {
			return statErr
		}
		if statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			staged = false
		}
	}
	target := fullPath
	if staged {
		target = path.Join(dstDir.GetPath(), ".openlist-upload-"+uuid.NewString())
	}
	var dstFile *sftp.File
	if staged {
		dstFile, err = client.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY)
	} else {
		dstFile, err = client.Create(target)
	}
	if err != nil {
		return err
	}
	if staged {
		defer func() {
			if target == "" {
				return
			}
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			stopCleanup := context.AfterFunc(cleanupCtx, func() { _ = transport.Close() })
			defer stopCleanup()
			_ = client.Remove(target)
		}()
	}
	err = utils.CopyWithCtx(ctx, dstFile, driver.NewLimitedUploadStream(ctx, file), file.GetSize(), up)
	closeErr := dstFile.Close()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if staged {
		if err := client.PosixRename(target, fullPath); err != nil {
			return err
		}
		target = ""
	}
	return nil
}

func (d *SFTP) GetDetails(ctx context.Context) (*model.StorageDetails, error) {
	client, err := d.getClient(ctx)
	if err != nil {
		return nil, err
	}
	stat, err := client.StatVFS(d.RootFolderPath)
	if err != nil {
		if strings.Contains(err.Error(), "unimplemented") {
			return nil, errs.NotImplement
		}
		return nil, err
	}
	total := int64(stat.Blocks * stat.Bsize)
	free := int64(stat.Bfree * stat.Bsize)
	return &model.StorageDetails{
		DiskUsage: model.DiskUsage{
			TotalSpace: total,
			UsedSpace:  total - free,
		},
	}, nil
}

var _ driver.Driver = (*SFTP)(nil)

func (d *SFTP) Get(ctx context.Context, p string) (model.Obj, error) {
	client, err := d.getClient(ctx)
	if err != nil {
		return nil, err
	}
	p = path.Join(d.RootFolderPath, p)
	info, err := client.Lstat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errs.ObjectNotFound
		}
		return nil, err
	}
	return d.fileToObj(client, info, path.Dir(p))
}

var _ driver.Getter = (*SFTP)(nil)
var _ driver.WithDetails = (*SFTP)(nil)
