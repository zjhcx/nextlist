package smb

import (
	"context"
	"errors"
	"net"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/errs"
	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
	"github.com/cloudsoda/go-smb2"
)

// A retired share stays alive until its last reader closes. Reconnecting must
// not unmount the share underneath another upload, copy or ranged download.
type shareLease struct {
	share  *smb2.Share
	conn   net.Conn
	users  int
	failed bool
}

func (d *SMB) updateLastConnTime() { d.lastConnTime.Store(time.Now().Unix()) }
func (d *SMB) cleanLastConnTime() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.current != nil {
		d.current.failed = true
	}
}

func (d *SMB) acquireShare(ctx context.Context) (*smb2.Share, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, nil, errs.StorageNotInit
	}
	if d.current == nil || d.current.failed {
		dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", d.Address)
		if err != nil {
			return nil, nil, err
		}
		dialer := &smb2.Dialer{Initiator: &smb2.NTLMInitiator{User: d.Username, Password: d.Password, Domain: d.Domain}}
		session, err := dialer.DialConn(dialCtx, conn, d.Address)
		if err != nil {
			_ = conn.Close()
			return nil, nil, err
		}
		share, err := session.WithContext(dialCtx).Mount(d.ShareName)
		if err != nil {
			_ = conn.Close()
			return nil, nil, err
		}
		old := d.current
		d.current = &shareLease{share: share, conn: conn}
		if d.leases == nil {
			d.leases = make(map[*shareLease]struct{})
		}
		d.leases[d.current] = struct{}{}
		if old != nil && old.users == 0 {
			_ = old.conn.Close()
			delete(d.leases, old)
		}
	}
	lease := d.current
	lease.users++
	var once sync.Once
	release := func() {
		once.Do(func() {
			d.mu.Lock()
			defer d.mu.Unlock()
			lease.users--
			if ctx.Err() != nil {
				lease.failed = true
			}
			if lease == d.current && lease.failed && lease.users == 0 {
				d.current = nil
			}
			if lease != d.current && lease.users == 0 {
				_ = lease.conn.Close()
				delete(d.leases, lease)
			}
		})
	}
	return lease.share.WithContext(ctx), release, nil
}

func overlappingCopy(src, dst string, directory bool) bool {
	clean := func(p string) string { return strings.ToLower(path.Clean("/" + strings.ReplaceAll(p, "\\", "/"))) }
	src, dst = clean(src), clean(dst)
	return src == dst || (directory && strings.HasPrefix(dst, strings.TrimSuffix(src, "/")+"/"))
}

func copyFile(ctx context.Context, share *smb2.Share, src, dst string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if overlappingCopy(src, dst, false) {
		return errors.New("cannot copy an SMB file onto itself")
	}
	srcfd, err := share.Open(src)
	if err != nil {
		return err
	}
	defer srcfd.Close()
	info, err := srcfd.Stat()
	if err != nil {
		return err
	}
	if err := share.MkdirAll(path.Dir(dst), 0700); err != nil {
		return err
	}
	dstfd, err := share.Create(dst)
	if err != nil {
		return err
	}
	// Keep *smb2.File on both sides so the SDK can use server-side copy.
	_, copyErr := utils.CopyWithBuffer(dstfd, srcfd)
	closeErr := dstfd.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return share.Chmod(dst, info.Mode())
}

func copyDir(ctx context.Context, share *smb2.Share, src, dst string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if overlappingCopy(src, dst, true) {
		return errors.New("cannot copy an SMB directory into itself")
	}
	info, err := share.Stat(src)
	if err != nil {
		return err
	}
	if err := share.MkdirAll(dst, info.Mode()); err != nil {
		return err
	}
	entries, err := share.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := entry.Name()
		if name == "." || name == ".." || name == "" || strings.ContainsAny(name, "/\\\x00") {
			return errs.PermissionDenied
		}
		// Do not follow reparse points recursively: they can escape the source tree
		// or form a cycle. The regular OpenList copy path can handle unsupported cases.
		if entry.Mode()&os.ModeSymlink != 0 {
			return errs.NotSupport
		}
		from, to := path.Join(src, name), path.Join(dst, name)
		if entry.IsDir() {
			err = copyDir(ctx, share, from, to)
		} else {
			err = copyFile(ctx, share, from, to)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
