package mux

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// Route 存储匹配请求和构建url的信息
type Route struct {
	// 路由的请求处理程序
	handler http.Handler
	// 如果为true，则此路由永远不匹配:它只用于构建url
	buildOnly bool
	// 用于构建url的名称
	name string
	// 由于建立路由导致的错误
	err error

	// 对所有命名路由的全局引用
	namedRoutes map[string]*Route

	// 从`Router`传入的配置
	routeConf
}

// SkipClean 跳过路径清洗功能
func (r *Route) SkipClean() bool {
	return r.skipClean
}

// Match 根据请求匹配路由
func (r *Route) Match(req *http.Request, match *RouteMatch) bool {
	if r.buildOnly || r.err != nil {
		return false
	}

	var matchErr error

	// 匹配所有
	for _, m := range r.matchers {
		if matched := m.Match(req, match); !matched {
			if _, ok := m.(methodMatcher); ok {
				matchErr = ErrMethodMismatch
				continue
			}

			// 忽略ErrNotFound错误，包括子路由
			// 非空的MatchErr和被跳过，即使有匹配到的路由
			if match.MatchErr == ErrNotFound {
				match.MatchErr = nil
			}

			matchErr = nil
			return false
		}
	}

	if matchErr != nil {
		match.MatchErr = matchErr
		return false
	}

	if match.MatchErr == ErrMethodMismatch && r.handler != nil {
		match.MatchErr = nil
		match.Handler = r.handler
	}

	if match.Route == nil {
		match.Route = r
	}
	if match.Handler == nil {
		match.Handler = r.handler
	}
	if match.Vars == nil {
		match.Vars = make(map[string]string)
	}

	// 设置变量
	r.regexp.setMatch(req, match, r)
	return true
}

// ----------------------------------------------------------------------------
// 路由属性
// ----------------------------------------------------------------------------

// GetError 返回构建路由时产生的错误(如果有)。
func (r *Route) GetError() error {
	return r.err
}

// BuildOnly 将路由设置为永远不匹配:它只用于构建url。
func (r *Route) BuildOnly() *Route {
	r.buildOnly = true
	return r
}

// Handler --------------------------------------------------------------------

// Handler 为路由设置一个处理程序
func (r *Route) Handler(handler http.Handler) *Route {
	if r.err == nil {
		r.handler = handler
	}
	return r
}

// HandlerFunc 为路由设置处理函数
func (r *Route) HandlerFunc(f func(http.ResponseWriter, *http.Request)) *Route {
	return r.Handler(http.HandlerFunc(f))
}

// GetHandler 返回路由的处理程序(如果有的话)
func (r *Route) GetHandler() http.Handler {
	return r.handler
}

// Name -----------------------------------------------------------------------

// Name 设置路由的名称，用于构建url，在路由上多次调用Name是错误的
func (r *Route) Name(name string) *Route {
	if r.name != "" {
		r.err = fmt.Errorf("mux: route already has name %q, can't set %q",
			r.name, name)
	}
	if r.err == nil {
		r.name = name
		r.namedRoutes[name] = r
	}
	return r
}

// GetName 返回路由的名称(如果有)
func (r *Route) GetName() string {
	return r.name
}

// ----------------------------------------------------------------------------
// 匹配器
// ----------------------------------------------------------------------------

// addMatcher 向路由添加匹配器
func (r *Route) addMatcher(m matcher) *Route {
	if r.err == nil {
		r.matchers = append(r.matchers, m)
	}
	return r
}

// addRegexpMatcher 将主机或路径匹配器和生成器添加到路由
func (r *Route) addRegexpMatcher(tpl string, typ regexpType) error {
	if r.err != nil {
		return r.err
	}
	if typ == regexpTypePath || typ == regexpTypePrefix {
		if len(tpl) > 0 && tpl[0] != '/' {
			return fmt.Errorf("mux: path must start with a slash, got %q", tpl)
		}
		if r.regexp.path != nil {
			tpl = strings.TrimRight(r.regexp.path.template, "/") + tpl
		}
	}
	rr, err := newRouteRegexp(tpl, typ, routeRegexpOptions{
		strictSlash:    r.strictSlash,
		useEncodedPath: r.useEncodedPath,
	})
	if err != nil {
		return err
	}
	for _, q := range r.regexp.queries {
		if err = uniqueVars(rr.varsN, q.varsN); err != nil {
			return err
		}
	}
	if typ == regexpTypeHost {
		if r.regexp.path != nil {
			if err = uniqueVars(rr.varsN, r.regexp.path.varsN); err != nil {
				return err
			}
		}
		r.regexp.host = rr
	} else {
		if r.regexp.host != nil {
			if err = uniqueVars(rr.varsN, r.regexp.host.varsN); err != nil {
				return err
			}
		}
		if typ == regexpTypeQuery {
			r.regexp.queries = append(r.regexp.queries, rr)
		} else {
			r.regexp.path = rr
		}
	}
	r.addMatcher(rr)
	return nil
}

// Headers --------------------------------------------------------------------

// Headers 为请求标头值添加匹配器
// 示例:
//
//	r := mux.NewRouter()
//	r.Headers("Content-Type", "application/json",
//	          "X-Requested-With", "XMLHttpRequest")
//
// 面的路由只有在两个请求报头值匹配的情况下才会匹配，如果值是一个空字符串，它将匹配设置了键的任何值
func (r *Route) Headers(pairs ...string) *Route {
	if r.err == nil {
		var headers map[string]string
		headers, r.err = mapFromPairsToString(pairs...)
		return r.addMatcher(headerMatcher(headers))
	}
	return r
}

// HeadersRegexp 受一个键/值对序列，其中值具有正则表达式
// 示例:
//
//	r := mux.NewRouter()
//	r.HeadersRegexp("Content-Type", "application/(text|json)",
//	          "X-Requested-With", "XMLHttpRequest")
//
// 只有当两个请求头都匹配两个正则表达式时，上面的路由才会匹配
// 如果值是一个空字符串，它将匹配设置了键的任何值
// 使用字符串锚的开始和结束符(^和$)来匹配精确的值
func (r *Route) HeadersRegexp(pairs ...string) *Route {
	if r.err == nil {
		var headers map[string]*regexp.Regexp
		headers, r.err = mapFromPairsToRegex(pairs...)
		return r.addMatcher(headerRegexMatcher(headers))
	}
	return r
}

// Host -----------------------------------------------------------------------

// Host 为URL主机添加匹配器，它接受包含0个或多个URL变量的模板，这些变量用{}括起来
// 变量可以定义一个可选的regexp模式来匹配:
//
// - {name} 匹配任何东西直到下一个点.
//
// - {name:pattern} 匹配给定的regexp模式.
//
// 示例:
//
//	r := mux.NewRouter()
//	r.Host("www.example.com")
//	r.Host("{subdomain}.domain.com")
//	r.Host("{subdomain:[a-z]+}.domain.com")
//
// 在给定路由中，变量名必须是唯一的。它们可以被检索到
// 调用 mux.Vars(request).
func (r *Route) Host(tpl string) *Route {
	r.err = r.addRegexpMatcher(tpl, regexpTypeHost)
	return r
}

// MatcherFunc ----------------------------------------------------------------

// MatcherFunc 是自定义匹配器使用的函数签名
type MatcherFunc func(*http.Request, *RouteMatch) bool

// Match 返回给定请求的匹配项
func (m MatcherFunc) Match(r *http.Request, match *RouteMatch) bool {
	return m(r, match)
}

// MatcherFunc 添加要用作请求匹配器的自定义函数
func (r *Route) MatcherFunc(f MatcherFunc) *Route {
	return r.addMatcher(f)
}

// Methods --------------------------------------------------------------------

// Methods 为HTTP方法添加匹配器，它接受一个或多个方法的序列来匹配
// 如："GET", "POST", "PUT"
func (r *Route) Methods(methods ...string) *Route {
	for k, v := range methods {
		methods[k] = strings.ToUpper(v)
	}
	return r.addMatcher(methodMatcher(methods))
}

// Path -----------------------------------------------------------------------

// Path 为URL路径添加匹配器
// 它接受包含0个或多个URL变量的模板，这些变量用{}括起来
// 模板必须以"/"开头
// 变量可以定义一个可选的regexp模式来匹配:
//
// - {name} 匹配下一个斜杠之前的任何内容
// - {name:pattern} 匹配给定的regexp模式
//
// 示例:
//
//	r := mux.NewRouter()
//	r.Path("/products/").Handler(ProductsHandler)
//	r.Path("/products/{key}").Handler(ProductsHandler)
//	r.Path("/articles/{category}/{id:[0-9]+}").
//	  Handler(ArticleHandler)
//
// 在给定路由中，变量名必须是唯一的 可通过 mux.Vars(request)调用
func (r *Route) Path(tpl string) *Route {
	r.err = r.addRegexpMatcher(tpl, regexpTypePath)
	return r
}

// PathPrefix -----------------------------------------------------------------

// PathPrefix 为URL路径前缀添加一个匹配器见Route.Path()
func (r *Route) PathPrefix(tpl string) *Route {
	r.err = r.addRegexpMatcher(tpl, regexpTypePrefix)
	return r
}

// Query ----------------------------------------------------------------------

// Queries 为URL查询值添加匹配器
// 接受一个键/值对序列。值可以定义变量
// 示例:
//
//	r := mux.NewRouter()
//	r.Queries("foo", "bar", "id", "{id:[0-9]+}")
//
// 只有当URL包含定义的查询时，上面的路由才会匹配，如：?foo=bar&id=42
// 如果值是一个空字符串，它将匹配设置了键的任何值
// 变量可以定义一个可选的regexp模式来匹配
// - {name} 匹配下一个斜杠之前的任何内容
// - {name:pattern} 匹配给定的regexp模式
func (r *Route) Queries(pairs ...string) *Route {
	length := len(pairs)
	if length%2 != 0 {
		r.err = fmt.Errorf(
			"mux: number of parameters must be multiple of 2, got %v", pairs)
		return nil
	}
	for i := 0; i < length; i += 2 {
		if r.err = r.addRegexpMatcher(pairs[i]+"="+pairs[i+1], regexpTypeQuery); r.err != nil {
			return r
		}
	}

	return r
}

// Schemes 为URL模式添加匹配器
func (r *Route) Schemes(schemes ...string) *Route {
	for k, v := range schemes {
		schemes[k] = strings.ToLower(v)
	}
	if len(schemes) > 0 {
		r.buildScheme = schemes[0]
	}
	return r.addMatcher(schemeMatcher(schemes))
}

// BuildVarsFunc --------------------------------------------------------------

// BuildVarsFunc 自定义构建变量是否使用函数签名(可以在构建路由URL之前修改路由变量)
type BuildVarsFunc func(map[string]string) map[string]string

// BuildVarsFunc 添加要用于修改生成变量的自定义函数（在构建路由的URL之前）
func (r *Route) BuildVarsFunc(f BuildVarsFunc) *Route {
	if r.buildVarsFunc != nil {
		// compose the old and new functions
		old := r.buildVarsFunc
		r.buildVarsFunc = func(m map[string]string) map[string]string {
			return f(old(m))
		}
	} else {
		r.buildVarsFunc = f
	}
	return r
}

// 子路由器 ------------------------------------------------------------------

// Subrouter 为路由创建子路由器
//
// 只有当父路由匹配时，它才会走内部路由，如:
//
//	r := mux.NewRouter()
//	s := r.Host("www.example.com").Subrouter()
//	s.HandleFunc("/products/", ProductsHandler)
//	s.HandleFunc("/products/{key}", ProductHandler)
//	s.HandleFunc("/articles/{category}/{id:[0-9]+}"), ArticleHandler)
//
// 如果主机不匹配，也不会到子路由器
func (r *Route) Subrouter() *Router {
	// 用父路由配置的副本初始化子路由
	router := &Router{routeConf: copyRouteConf(r.routeConf), namedRoutes: r.namedRoutes}
	r.addMatcher(router)
	return router
}

// ----------------------------------------------------------------------------
// URL构建
// ----------------------------------------------------------------------------

// URL 为路由构建一个URL，示例：
//
//	r := mux.NewRouter()
//	r.HandleFunc("/articles/{category}/{id:[0-9]+}", ArticleHandler).
//	  Name("article")
//
// 获取url示例:
//
//	url, err := r.Get("article").URL("category", "technology", "id", "42")
//	"/articles/technology/42"
//
// 适用于主机变量:
//
//	r := mux.NewRouter()
//	r.HandleFunc("/articles/{category}/{id:[0-9]+}", ArticleHandler).
//	  Host("{subdomain}.domain.com").
//	  Name("article")
//
//	// url.String() "http://news.domain.com/articles/technology/42"
//	url, err := r.Get("article").URL("subdomain", "news",
//	                                 "category", "technology",
//	                                 "id", "42")
//
// 结果url的模式将是传递给Schemes的第一个参数:
//
//	// url.String() "https://example.com"
//	r := mux.NewRouter()
//	url, err := r.Host("example.com")
//	             .Schemes("https", "http").URL()
//
// 路由中定义的所有变量都是必需的，它们的值也必须是必需的
func (r *Route) URL(pairs ...string) (*url.URL, error) {
	if r.err != nil {
		return nil, r.err
	}
	values, err := r.prepareVars(pairs...)
	if err != nil {
		return nil, err
	}
	var scheme, host, path string
	queries := make([]string, 0, len(r.regexp.queries))
	if r.regexp.host != nil {
		if host, err = r.regexp.host.url(values); err != nil {
			return nil, err
		}
		scheme = "http"
		if r.buildScheme != "" {
			scheme = r.buildScheme
		}
	}
	if r.regexp.path != nil {
		if path, err = r.regexp.path.url(values); err != nil {
			return nil, err
		}
	}
	for _, q := range r.regexp.queries {
		var query string
		if query, err = q.url(values); err != nil {
			return nil, err
		}
		queries = append(queries, query)
	}
	return &url.URL{
		Scheme:   scheme,
		Host:     host,
		Path:     path,
		RawQuery: strings.Join(queries, "&"),
	}, nil
}

// URLHost 为路由构建URL的主机部分（路由必须定义了主机），参考 Route.URL()
func (r *Route) URLHost(pairs ...string) (*url.URL, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.regexp.host == nil {
		return nil, errors.New("mux: route doesn't have a host")
	}
	values, err := r.prepareVars(pairs...)
	if err != nil {
		return nil, err
	}
	host, err := r.regexp.host.url(values)
	if err != nil {
		return nil, err
	}
	u := &url.URL{
		Scheme: "http",
		Host:   host,
	}
	if r.buildScheme != "" {
		u.Scheme = r.buildScheme
	}
	return u, nil
}

// URLPath 为路由构建URL的路径部分（路由必须定义了路径）. 参考 Route.URL()
func (r *Route) URLPath(pairs ...string) (*url.URL, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.regexp.path == nil {
		return nil, errors.New("mux: route doesn't have a path")
	}
	values, err := r.prepareVars(pairs...)
	if err != nil {
		return nil, err
	}
	path, err := r.regexp.path.url(values)
	if err != nil {
		return nil, err
	}
	return &url.URL{
		Path: path,
	}, nil
}

// GetPathTemplate 返回用于构建的模板
func (r *Route) GetPathTemplate() (string, error) {
	if r.err != nil {
		return "", r.err
	}
	if r.regexp.path == nil {
		return "", errors.New("mux: route doesn't have a path")
	}
	return r.regexp.path.template, nil
}

// GetPathRegexp 返回用于匹配路由路径的扩展正则表达式
func (r *Route) GetPathRegexp() (string, error) {
	if r.err != nil {
		return "", r.err
	}
	if r.regexp.path == nil {
		return "", errors.New("mux: route does not have a path")
	}
	return r.regexp.path.regexp.String(), nil
}

// GetQueriesRegexp 对象匹配的扩展正则表达式
func (r *Route) GetQueriesRegexp() ([]string, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.regexp.queries == nil {
		return nil, errors.New("mux: route doesn't have queries")
	}
	queries := make([]string, 0, len(r.regexp.queries))
	for _, query := range r.regexp.queries {
		queries = append(queries, query.regexp.String())
	}
	return queries, nil
}

// GetQueriesTemplates 返回查询的模板
func (r *Route) GetQueriesTemplates() ([]string, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.regexp.queries == nil {
		return nil, errors.New("mux: route doesn't have queries")
	}
	queries := make([]string, 0, len(r.regexp.queries))
	for _, query := range r.regexp.queries {
		queries = append(queries, query.template)
	}
	return queries, nil
}

// GetMethods 返回路由匹配的方法
func (r *Route) GetMethods() ([]string, error) {
	if r.err != nil {
		return nil, r.err
	}
	for _, m := range r.matchers {
		if methods, ok := m.(methodMatcher); ok {
			return []string(methods), nil
		}
	}
	return nil, errors.New("mux: route doesn't have methods")
}

// GetHostTemplate returns 返回主机匹配规则
func (r *Route) GetHostTemplate() (string, error) {
	if r.err != nil {
		return "", r.err
	}
	if r.regexp.host == nil {
		return "", errors.New("mux: route doesn't have a host")
	}
	return r.regexp.host.template, nil
}

// prepareVars 将路由变量对转换为映射。如果路由有调用BuildVarsFunc
func (r *Route) prepareVars(pairs ...string) (map[string]string, error) {
	m, err := mapFromPairsToString(pairs...)
	if err != nil {
		return nil, err
	}
	return r.buildVars(m), nil
}

func (r *Route) buildVars(m map[string]string) map[string]string {
	if r.buildVarsFunc != nil {
		m = r.buildVarsFunc(m)
	}
	return m
}
