package client

import (
	"example.com/ext/allowed"
	"example.com/ext/forbidden"
)

var _ = allowed.Client{}
var _ = forbidden.Client{}
