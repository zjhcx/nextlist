package sftp

import (
	"context"
	"errors"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/errs"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func (d *SFTP) getClient(ctx context.Context) (*sftp.Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d.connMu.Lock()
	defer d.connMu.Unlock()
	if d.closed {
		return nil, errs.StorageNotInit
	}
	if d.client != nil {
		select {
		case <-d.clientDone:
		default:
			return d.client, nil
		}
		_ = d.sshClient.Close()
		_ = d.client.Close()
		d.client = nil
	}
	client, conn, err := d.connect(ctx)
	if err != nil {
		return nil, err
	}
	done := make(chan struct{})
	d.client, d.sshClient, d.clientDone = client, conn, done
	go func() { _ = client.Wait(); close(done) }()
	return client, nil
}

func (d *SFTP) connect(ctx context.Context) (*sftp.Client, *ssh.Client, error) {
	var auth ssh.AuthMethod
	if d.PrivateKey != "" {
		var signer ssh.Signer
		var err error
		if d.Passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(d.PrivateKey), []byte(d.Passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey([]byte(d.PrivateKey))
		}
		if err != nil {
			return nil, nil, err
		}
		auth = ssh.PublicKeys(signer)
	} else {
		auth = ssh.Password(d.Password)
	}
	hostKey := ssh.InsecureIgnoreHostKey()
	if fingerprint := strings.TrimSpace(d.HostKeyFingerprint); fingerprint != "" {
		if !strings.HasPrefix(fingerprint, "SHA256:") {
			return nil, nil, errors.New("SFTP host key fingerprint must use SHA256 format")
		}
		hostKey = func(_ string, _ net.Addr, key ssh.PublicKey) error {
			if ssh.FingerprintSHA256(key) != fingerprint {
				return errors.New("SFTP host key fingerprint mismatch")
			}
			return nil
		}
	}
	config := &ssh.ClientConfig{User: d.Username, Auth: []ssh.AuthMethod{auth}, HostKeyCallback: hostKey}
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if d.lifetime == nil {
		return nil, nil, errs.StorageNotInit
	}
	stopMount := context.AfterFunc(d.lifetime, cancel)
	defer stopMount()
	raw, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", d.Address)
	if err != nil {
		return nil, nil, err
	}
	stop := context.AfterFunc(dialCtx, func() { _ = raw.Close() })
	defer stop()
	deadline, _ := dialCtx.Deadline()
	_ = raw.SetDeadline(deadline)
	conn, channels, requests, err := ssh.NewClientConn(raw, d.Address, config)
	if err != nil {
		_ = raw.Close()
		return nil, nil, err
	}
	transport := ssh.NewClient(conn, channels, requests)
	client, err := sftp.NewClient(transport)
	if err != nil {
		_ = transport.Close()
		return nil, nil, err
	}
	if !stop() && dialCtx.Err() != nil {
		_ = transport.Close()
		return nil, nil, dialCtx.Err()
	}
	_ = raw.SetDeadline(time.Time{})
	return client, transport, nil
}

func remove(ctx context.Context, client *sftp.Client, p string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := client.Lstat(p)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return client.Remove(p)
	}
	entries, err := client.ReadDirContext(ctx, p)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == "." || name == ".." || name == "" || strings.ContainsAny(name, "/\x00") {
			return errs.PermissionDenied
		}
		if err := remove(ctx, client, path.Join(p, name)); err != nil {
			return err
		}
	}
	return client.RemoveDirectory(p)
}
