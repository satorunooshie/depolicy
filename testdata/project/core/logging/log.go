package logging

import (
	"log/slog"

	"github.com/satorunooshie/depolicytest/domain/user/entity"
)

var _ = slog.LevelInfo
var _ = entity.User{}
