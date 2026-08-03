//go:build !xpack && !enterprise

package xpack

import (
	"github.com/1Panel-dev/1Panel/core/utils/xpack/helper"
	"github.com/1Panel-dev/1Panel/core/utils/xpack/providers"
)

var AuthProvider = helper.NewIAuthProvider()

var MultiNodeProvider providers.MultiNodeProvider = helper.NewIMultiNodeProvider()
