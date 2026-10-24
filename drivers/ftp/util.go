package ftp

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/errs"
	"github.com/axgle/mahonia"
	"github.com/jlaffaye/ftp"
)

func (d *FTP) validate() error {
	if d.Encoding != "" && (mahonia.NewEncoder(d.Encoding) == nil || mahonia.NewDecoder(d.Encoding) == nil) {
		return errors.New("unsupported FTP character encoding")
	}
	switch d.TLS {
	case "", "none", "explicit", "implicit":
	default:
		return errors.New("invalid FTP TLS mode")
	}
	if d.RootFolderPath == "" {
		d.RootFolderPath = "/"
	}
	return nil
}

// acquire serializes the metadata connection: ftp.ServerConn is not safe for
// concurrent use. Transfers use their own connections so lists remain usable.
func (d *FTP) acquire(ctx context.Context) (func(), error) {
	d.mu.Lock()
	if err := ctx.Err(); err != nil {
		d.mu.Unlock()
		return nil, err
	}
	if d.ctx == nil || d.ctx.Err() != nil {
		d.mu.Unlock()
		return nil, errs.StorageNotInit
	}
	if d.conn != nil {
		stop := context.AfterFunc(ctx, d.connCancel)
		err := d.conn.NoOp()
		stop()
		if err != nil {
			d.connCancel()
			d.conn = nil
		}
	}
	if d.conn == nil {
		lifetime, cancel := context.WithCancel(d.ctx)
		stop := context.AfterFunc(ctx, cancel)
		conn, closeConn, err := d.connectContext(lifetime)
		stop()
		if err != nil {
			cancel()
			d.mu.Unlock()
			return nil, err
		}
		d.conn, d.connCancel = conn, func() { cancel(); closeConn() }
	}
	closeConn := d.connCancel
	stop := context.AfterFunc(ctx, closeConn)
	return func() {
		stop()
		if ctx.Err() != nil {
			closeConn()
			d.conn = nil
		}
		d.mu.Unlock()
	}, nil
}

func (d *FTP) connect(ctx context.Context) (*ftp.ServerConn, context.CancelFunc, error) {
	if d.ctx == nil {
		return nil, nil, errs.StorageNotInit
	}
	return d.connectContext(ctx)
}

func (d *FTP) connectContext(ctx context.Context) (*ftp.ServerConn, context.CancelFunc, error) {
	if d.ctx == nil || d.ctx.Err() != nil {
		return nil, nil, errs.StorageNotInit
	}
	ctx, cancel := context.WithCancel(ctx)
	stopMount := context.AfterFunc(d.ctx, cancel)
	var once sync.Once
	cleanup := func() { once.Do(func() { stopMount(); cancel() }) }
	openedControl := false
	var control net.Conn
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: d.TLSSkipVerify}
	host, _, hostErr := net.SplitHostPort(d.Address)
	if hostErr == nil {
		tlsConfig.ServerName = strings.Trim(host, "[]")
	}
	dial := func(network, address string) (net.Conn, error) {
		c, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		stop := context.AfterFunc(ctx, func() { _ = c.Close() })
		if !openedControl {
			control = c
			_ = c.SetDeadline(time.Now().Add(30 * time.Second))
		}
		raw := &contextConn{Conn: c, stop: stop}
		secure := d.TLS == "implicit" || (d.TLS == "explicit" && openedControl)
		openedControl = true
		if secure {
			return tls.Client(raw, tlsConfig), nil
		}
		return raw, nil
	}
	options := []ftp.DialOption{ftp.DialWithContext(ctx), ftp.DialWithDialFunc(dial), ftp.DialWithShutTimeout(10 * time.Second)}
	switch d.TLS {
	case "explicit":
		options = append(options, ftp.DialWithExplicitTLS(tlsConfig))
	case "implicit":
		options = append(options, ftp.DialWithTLS(tlsConfig))
	}
	conn, err := ftp.Dial(d.Address, options...)
	if err == nil {
		err = conn.Login(d.Username, d.Password)
	}
	if err != nil {
		cleanup()
		if conn != nil {
			_ = conn.Quit()
		}
		return nil, nil, err
	}
	_ = control.SetDeadline(time.Time{})
	return conn, cleanup, nil
}

type contextConn struct {
	net.Conn
	stop func() bool
}

func (c *contextConn) Close() error { c.stop(); return c.Conn.Close() }
