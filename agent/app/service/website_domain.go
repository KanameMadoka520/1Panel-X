package service

import (
	"context"
	"fmt"
	"strconv"

	"github.com/1Panel-dev/1Panel/agent/app/dto/request"
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/1Panel-dev/1Panel/agent/app/repo"
	"github.com/1Panel-dev/1Panel/agent/constant"
)

func (w WebsiteService) CreateWebsiteDomain(create request.WebsiteDomainCreate) ([]model.WebsiteDomain, error) {
	var (
		domainModels []model.WebsiteDomain
		addPorts     []int
	)
	httpPort, httpsPort, err := getAppInstallPort(constant.AppOpenresty)
	if err != nil {
		return nil, err
	}
	website, err := websiteRepo.GetFirst(repo.WithByID(create.WebsiteID))
	if err != nil {
		return nil, err
	}

	domainModels, addPorts, _, err = getWebsiteDomains(create.Domains, httpPort, httpsPort, create.WebsiteID)
	if err != nil {
		return nil, err
	}
	go func() {
		_ = OperateFirewallPort(nil, addPorts)
	}()

	if err = addListenAndServerName(website, domainModels); err != nil {
		return nil, err
	}

	return domainModels, websiteDomainRepo.BatchCreate(context.TODO(), domainModels)
}

func (w WebsiteService) GetWebsiteDomain(websiteId uint) ([]model.WebsiteDomain, error) {
	return websiteDomainRepo.GetBy(websiteDomainRepo.WithWebsiteId(websiteId))
}

func (w WebsiteService) DeleteWebsiteDomain(domainId uint) error {
	webSiteDomain, err := websiteDomainRepo.GetFirst(repo.WithByID(domainId))
	if err != nil {
		return err
	}

	if websiteDomains, _ := websiteDomainRepo.GetBy(websiteDomainRepo.WithWebsiteId(webSiteDomain.WebsiteID)); len(websiteDomains) == 1 {
		return fmt.Errorf("can not delete last domain")
	}
	website, err := websiteRepo.GetFirst(repo.WithByID(webSiteDomain.WebsiteID))
	if err != nil {
		return err
	}
	var ports []int
	if oldDomains, _ := websiteDomainRepo.GetBy(websiteDomainRepo.WithWebsiteId(webSiteDomain.WebsiteID), websiteDomainRepo.WithPort(webSiteDomain.Port)); len(oldDomains) == 1 {
		ports = append(ports, webSiteDomain.Port)
	}

	var domains []string
	if oldDomains, _ := websiteDomainRepo.GetBy(websiteDomainRepo.WithWebsiteId(webSiteDomain.WebsiteID), websiteDomainRepo.WithDomain(webSiteDomain.Domain)); len(oldDomains) == 1 {
		domains = append(domains, webSiteDomain.Domain)
	}

	if len(ports) > 0 || len(domains) > 0 {
		stringBinds := make([]string, len(ports))
		for i := 0; i < len(ports); i++ {
			stringBinds[i] = strconv.Itoa(ports[i])
		}
		if err := deleteListenAndServerName(website, stringBinds, domains); err != nil {
			return err
		}
	}

	return websiteDomainRepo.DeleteBy(context.TODO(), repo.WithByID(domainId))
}
