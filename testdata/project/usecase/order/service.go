package order

import (
	"errors"

	"github.com/satorunooshie/depolicytest/api/main/handler"
	"github.com/satorunooshie/depolicytest/domain/order/entity"
	"github.com/satorunooshie/depolicytest/generated/sqlc"
)

var _ = errors.New
var _ = handler.Handle
var _ = entity.Order{}
var _ = sqlc.Query
