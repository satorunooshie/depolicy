package handler

import (
	"errors"
	"fmt"

	"github.com/satorunooshie/depolicytest/component/billing/repository"
	"github.com/satorunooshie/depolicytest/generated/sqlc"
	"github.com/satorunooshie/depolicytest/infra/database"
)

var _ = fmt.Sprintf
var _ = errors.New
var _ = repository.Repository{}
var _ = sqlc.Query
var _ = database.Open

func Handle() {}
