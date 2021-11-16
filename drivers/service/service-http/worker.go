package service_http

import (
	"errors"
	"fmt"

	"github.com/eolinker/goku/upstream/balance"

	"time"

	upstream_http "github.com/eolinker/goku/drivers/upstream/upstream-http"

	"github.com/eolinker/eosc"
	"github.com/eolinker/goku/upstream"

	"github.com/eolinker/goku/service"
)

var (
	ErrorStructType      = errors.New("error struct type")
	ErrorNeedUpstream    = errors.New("need upstream")
	ErrorInvalidUpstream = errors.New("not upstream")
)

type serviceWorker struct {
	Service
	id     string
	name   string
	desc   string
	driver string
}

//Id 返回服务实例 worker id
func (s *serviceWorker) Id() string {
	return s.id
}

func (s *serviceWorker) Start() error {
	return nil
}

//Reset 重置服务实例的配置
func (s *serviceWorker) Reset(conf interface{}, workers map[eosc.RequireId]interface{}) error {
	data, ok := conf.(*Config)
	if !ok {
		return fmt.Errorf("need %s,now %s", eosc.TypeNameOf((*Config)(nil)), eosc.TypeNameOf(conf))
	}
	data.rebuild()

	if data.Upstream == "" && data.UpstreamAnonymous == nil {
		return ErrorNeedUpstream
	}
	var upstreamCreate upstream.IUpstream
	if upstreamWork, has := workers[data.Upstream]; has {
		if up, ok := upstreamWork.(upstream.IUpstream); ok {
			upstreamCreate = up
		} else {
			return fmt.Errorf("%s:%w", data.Upstream, ErrorInvalidUpstream)
		}
	} else {
		balanceFactory, err := balance.GetFactory(data.UpstreamAnonymous.Type)
		if err != nil {
			return err
		}

		anonymous, err := defaultDiscovery.GetApp(data.UpstreamAnonymous.Config)
		if err != nil {
			return err
		}
		balanceHandler, err := balanceFactory.Create(anonymous)
		if err != nil {
			return err
		}
		upstreamCreate = upstream_http.NewUpstream(s.scheme, anonymous, balanceHandler, nil)
	}

	//
	s.desc = data.Desc
	s.Service.timeout = time.Duration(data.Timeout) * time.Millisecond

	s.Service.retry = data.Retry
	s.Service.scheme = data.Scheme
	s.Service.proxyMethod = data.ProxyMethod

	s.Service.reset(upstreamCreate, data.PluginConfig)

	return nil

}

func (s *serviceWorker) Stop() error {
	return nil
}

//CheckSkill 检查目标能力是否存在
func (s *serviceWorker) CheckSkill(skill string) bool {
	return service.CheckSkill(skill)
}