package service

import (
	"github.com/gogu-x/gogs/battle/service/internal"
	"github.com/gogu-x/tree"
)

func New() tree.Actor {
	return internal.New()
}
