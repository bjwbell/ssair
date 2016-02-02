package ssair

import (
	"go/token"
	"go/types"
)

type Ctx struct {
	file *token.File
	fn   *types.Info
}
