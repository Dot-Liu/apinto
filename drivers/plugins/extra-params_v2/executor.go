package extra_params_v2

import (
	"fmt"
	"mime"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"

	"github.com/ohler55/ojg/oj"

	"github.com/eolinker/eosc/log"

	"github.com/eolinker/apinto/drivers"

	"github.com/ohler55/ojg/jp"

	"github.com/eolinker/eosc"
	"github.com/eolinker/eosc/eocontext"
	http_service "github.com/eolinker/eosc/eocontext/http-context"
)

var _ http_service.HttpFilter = (*executor)(nil)
var _ eocontext.IFilter = (*executor)(nil)

var (
	errorExist = "%s: %s is already exists"
)

type executor struct {
	drivers.WorkerBase
	baseParam       *baseParam
	requestBodyType string
	errorType       string
}

func (e *executor) DoFilter(ctx eocontext.EoContext, next eocontext.IChain) (err error) {
	return http_service.DoHttpFilter(e, ctx, next)
}

func (e *executor) DoHttpFilter(ctx http_service.IHttpContext, next eocontext.IChain) error {
	statusCode, err := e.access(ctx)
	if err != nil {
		ctx.Response().SetBody([]byte(err.Error()))
		ctx.Response().SetStatus(statusCode, strconv.Itoa(statusCode))
		return err
	}

	if next != nil {
		return next.DoChain(ctx)
	}
	return nil
}

func addParamToBody(ctx http_service.IHttpContext, contentType string, params []*paramInfo) (interface{}, error) {

	//var bodyParam map[string]interface{}
	if contentType == "application/json" {
		body, _ := ctx.Proxy().Body().RawBody()
		if string(body) == "" {
			body = []byte("{}")
		}
		bodyParam, err := oj.Parse(body)
		if err != nil {
			return nil, err
		}
		for _, param := range params {
			key := param.name
			if !strings.HasPrefix(param.name, "$.") {
				key = "$." + key
			}

			x, err := jp.ParseString(key)
			if err != nil {
				return nil, fmt.Errorf("parse key error: %v", err)
			}
			result := x.Get(bodyParam)
			if len(result) > 0 {
				if param.conflict == paramError {
					return nil, fmt.Errorf(errorExist, positionBody, param.name)
				} else if param.conflict == paramOrigin {
					continue
				}
			}
			value, err := param.Build(ctx, contentType, bodyParam)
			if err != nil {
				log.Errorf("build param(s) error: %v", key, err)
				continue
			}
			err = x.Set(bodyParam, value)
			if err != nil {
				log.Errorf("set param(s) error: %v", key, err)
				continue
			}
		}

		b, _ := oj.Marshal(bodyParam)
		ctx.Proxy().Body().SetRaw(contentType, b)
		return bodyParam, nil
	} else if contentType == "application/x-www-form-urlencoded" || contentType == "multipart/form-data" {
		bodyParam := make(map[string]interface{})
		bodyForm, _ := ctx.Proxy().Body().BodyForm()
		for _, param := range params {
			_, has := bodyParam[param.name]
			if has {
				if param.conflict == paramError {
					return nil, fmt.Errorf("[extra_params] body(%s) has a conflict", param.name)
				} else if param.conflict == paramOrigin {
					continue
				}
			}
			value, err := param.Build(ctx, contentType, nil)
			if err != nil {
				log.Errorf("build param(s) error: %v", param.name, err)
				continue
			}
			bodyParam[param.name] = value

		}
		ctx.Proxy().Body().SetForm(bodyForm)
	}
	return nil, nil
}

func (e *executor) access(ctx http_service.IHttpContext) (int, error) {
	// 判断请求携带的content-type
	contentType, _, _ := mime.ParseMediaType(ctx.Proxy().Body().ContentType())
	var bodyParam interface{}
	var err error
	if ctx.Proxy().Method() == http.MethodPost || ctx.Proxy().Method() == http.MethodPut || ctx.Proxy().Method() == http.MethodPatch {
		if e.requestBodyType != "" {
			if e.requestBodyType == "json" && contentType != "application/json" {
				return clientErrStatusCode, encodeErr(e.errorType, `[extra_params] request body type is not json`, clientErrStatusCode)
			} else if e.requestBodyType == "form-data" && contentType != "multipart/form-data" {
				return clientErrStatusCode, encodeErr(e.errorType, `[extra_params] request body type is not form-data`, clientErrStatusCode)
			}
		}
		bodyParam, err = addParamToBody(ctx, contentType, e.baseParam.body)
		if err != nil {
			return clientErrStatusCode, encodeErr(e.errorType, err.Error(), clientErrStatusCode)
		}
	}

	// 处理Query参数
	for _, param := range e.baseParam.query {
		v := ctx.Proxy().URI().GetQuery(param.name)
		if v != "" {
			if param.conflict == paramError {
				return clientErrStatusCode, encodeErr(e.errorType, `[extra_params] query("`+param.name+`") has a conflict.`, clientErrStatusCode)
			} else if param.conflict == paramOrigin {
				continue
			}
		}
		value, err := param.Build(ctx, contentType, bodyParam)
		if err != nil {
			log.Errorf("build query extra param(%s) error: %s", param.name, err.Error())
			continue
		}
		ctx.Proxy().URI().SetQuery(param.name, value)
	}

	// 处理Header参数
	for _, param := range e.baseParam.header {
		name := textproto.CanonicalMIMEHeaderKey(param.name)
		_, has := ctx.Proxy().Header().Headers()[name]
		if has {
			if param.conflict == paramError {
				return clientErrStatusCode, encodeErr(e.errorType, `[extra_params] header("`+name+`") has a conflict.`, clientErrStatusCode)
			} else if param.conflict == paramOrigin {
				continue
			}
		}
		value, err := param.Build(ctx, contentType, bodyParam)
		if err != nil {
			log.Errorf("build header extra param(%s) error: %s", name, err.Error())
			continue
		}
		ctx.Proxy().Header().SetHeader(param.name, value)
	}
	return successStatusCode, nil
}

func (e *executor) Start() error {
	return nil
}

func (e *executor) Reset(conf interface{}, workers map[eosc.RequireId]eosc.IWorker) error {
	cfg, err := check(conf)
	if err != nil {
		return err
	}

	e.baseParam = generateBaseParam(cfg.Params)
	e.requestBodyType = cfg.RequestBodyType
	e.errorType = cfg.ErrorType

	return nil
}

func (e *executor) Stop() error {
	return nil
}

func (e *executor) Destroy() {
	e.baseParam = nil
	e.errorType = ""
}

func (e *executor) CheckSkill(skill string) bool {
	return http_service.FilterSkillName == skill
}
