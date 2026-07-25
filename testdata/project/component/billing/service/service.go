package service

import (
	"github.com/satorunooshie/depolicytest/api/main/handler"
	"github.com/satorunooshie/depolicytest/component/billing/repository"
	userservice "github.com/satorunooshie/depolicytest/component/user/service"
)

var _ = handler.Handle
var _ = repository.Repository{}
var _ = userservice.Service{}
