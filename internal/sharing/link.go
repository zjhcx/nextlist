package sharing

import (
	"context"
	"strings"

	"errors"

	"github.com/OpenListTeam/OpenList/v4/internal/conf"
	"github.com/OpenListTeam/OpenList/v4/internal/errs"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
)

func link(ctx context.Context, sid, path string, args *LinkArgs) (*model.Sharing, *model.Link, model.Obj, error) {
	sharing, err := op.GetSharingById(sid, args.SharingListArgs.Refresh)
	if err != nil {
		return nil, nil, nil, errs.SharingNotFound
	}
	if !sharing.Valid() {
		return sharing, nil, nil, errs.InvalidSharing
	}
	if !sharing.Verify(args.Pwd) {
		return sharing, nil, nil, errs.WrongShareCode
	}
	path = utils.FixAndCleanPath(path)
	if len(sharing.Files) == 1 || path != "/" {
		unwrapPath, err := op.GetSharingUnwrapPath(sharing, path)
		if err != nil {
			return nil, nil, nil, errs.Wrap(err, "failed get sharing unwrap path")
		}
		storage, actualPath, err := op.GetStorageAndActualPath(unwrapPath)
		if err != nil {
			return nil, nil, nil, errs.Wrap(err, "failed get sharing link")
		}
		l, obj, err := op.Link(ctx, storage, actualPath, args.LinkArgs)
		if err != nil {
			return nil, nil, nil, errs.Wrap(err, "failed get sharing link")
		}
		if l.URL != "" && !strings.HasPrefix(l.URL, "http://") && !strings.HasPrefix(l.URL, "https://") {
			l.URL = conf.GetApiUrl(ctx) + l.URL
		}
		return sharing, l, obj, nil
	}
	return nil, nil, nil, errors.New("cannot get sharing root link")
}
