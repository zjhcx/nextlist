package sftp

import (
	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
)

type Addition struct {
	Address            string `json:"address" required:"true"`
	Username           string `json:"username" required:"true"`
	PrivateKey         string `json:"private_key" type:"text"`
	Password           string `json:"password"`
	Passphrase         string `json:"passphrase"`
	HostKeyFingerprint string `json:"host_key_fingerprint" help:"Optional pinned SSH SHA256 host-key fingerprint"`
	AtomicUpload       bool   `json:"atomic_upload" default:"true" help:"Stage uploads when the server supports posix-rename@openssh.com"`
	driver.RootPath
	IgnoreSymlinkError bool `json:"ignore_symlink_error" default:"false" info:"Ignore symlink error"`
}

var config = driver.Config{
	Name:        "SFTP",
	LocalSort:   true,
	OnlyProxy:   true,
	DefaultRoot: "/",
	CheckStatus: true,
	NoLinkURL:   true,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &SFTP{}
	})
}
