package job

import (
	"github.com/1Panel-dev/1Panel/agent/app/service"
)

type waf struct{}

func NewWafJob() *waf {
	return &waf{}
}

func (w *waf) Run() {
	service.NewIWafEventService().Collect()
}
