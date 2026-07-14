package job

import (
	"github.com/1Panel-dev/1Panel/agent/app/service"
)

type websiteStat struct{}

func NewWebsiteStatJob() *websiteStat {
	return &websiteStat{}
}

func (w *websiteStat) Run() {
	service.NewIWebsiteStatService().Collect()
}
