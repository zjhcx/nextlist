package smb

import (
	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
)

type Addition struct {
	driver.RootPath
	Address      string `json:"address" required:"true"`
	Username     string `json:"username" required:"true"`
	Domain       string `json:"domain" help:"Optional NTLM domain"`
	Password     string `json:"password"`
	ShareName    string `json:"share_name" required:"true"`
	AtomicUpload bool   `json:"atomic_upload" default:"true" help:"Stage uploads before rename; requires create, rename and delete permissions"`
}

var config = driver.Config{
	Name:        "SMB",
	LocalSort:   true,
	OnlyProxy:   true,
	DefaultRoot: ".",
	NoCache:     true,
	NoLinkURL:   true,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &SMB{}
	})
}
