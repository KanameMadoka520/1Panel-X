package job

import (
	"github.com/1Panel-dev/1Panel/agent/app/service"
)

type waf struct{}

func NewWafJob() *waf {
	return &waf{}
}

func (w *waf) Run() {
	// Two independent journals, two tailers. The rule set writes its own audit
	// log; every decision the gateway makes itself goes to a separate one, and
	// without the second tailer those controls would enforce correctly and stay
	// entirely invisible in the panel.
	service.NewIWafEventService().Collect()
	service.NewIWafBlockRecordService().Collect()
}
