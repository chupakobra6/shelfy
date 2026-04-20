package ui

import copycat "github.com/igor/shelfy/internal/copy"

type Renderer struct {
	copy *copycat.Loader
}

func New(copy *copycat.Loader) *Renderer {
	return &Renderer{copy: copy}
}
