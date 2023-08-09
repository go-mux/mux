package mux

import (
	"errors"
	"net/http"
)

var (
	// ErrMethodMismatch 当请求中的方法不匹配时返回，针对路由定义的方法。
	ErrMethodMismatch = errors.New("method is not allowed")
	// ErrNotFound 当没有找到匹配的路由时返回
	ErrNotFound = errors.New("no matching route was found")
)

// NewRouter 创建一个路由器实例
func NewRouter() *Router {
	return &Router{namedRoutes: make(map[string]*Route)}
}

// Router 路由器
type Router struct {
	// 404
	NotFoundHandler http.Handler
	// 405不被允许
	MethodNotAllowedHandler http.Handler
	// 路由
	routes []*Route
	// 名称路由
	namedRoutes map[string]*Route
	// 中间件
	middlewares []middleware
	// 路由的共享配置
	routeConf
}

// ' Router '和' route '之间共享的公共路由配置
type routeConf struct {
	// 如果为 true, "/path/foo%2Fbar/to" 将匹配路径 "/path/{var}/to"
	useEncodedPath bool

	// 如果为 true, 当模式为 "/path/"时, 访问 "/path" 反之依然
	strictSlash bool

	// 如果为 true, 请求 "/path//to", 访问 "/path//to"，不会清理路径中多余的/
	skipClean bool

	// 来自host和path的变量管理器
	regexp routeRegexpGroup

	// 匹配器列表
	matchers []matcher

	// 构建url时使用的方案
	buildScheme string

	buildVarsFunc BuildVarsFunc
}

// 深拷贝 `routeConf`
func copyRouteConf(r routeConf) routeConf {
	c := r

	if r.regexp.path != nil {
		c.regexp.path = copyRouteRegexp(r.regexp.path)
	}

	if r.regexp.host != nil {
		c.regexp.host = copyRouteRegexp(r.regexp.host)
	}

	c.regexp.queries = make([]*routeRegexp, 0, len(r.regexp.queries))
	for _, q := range r.regexp.queries {
		c.regexp.queries = append(c.regexp.queries, copyRouteRegexp(q))
	}

	c.matchers = make([]matcher, len(r.matchers))
	copy(c.matchers, r.matchers)

	return c
}

func copyRouteRegexp(r *routeRegexp) *routeRegexp {
	c := *r
	return &c
}

// Match 根据路由器注册的路由匹配给定的请求，match参数被填充
func (r *Router) Match(req *http.Request, match *RouteMatch) bool {
	for _, route := range r.routes {
		if route.Match(req, match) {
			// 如果没有发现错误，则构建中间件链
			if match.MatchErr == nil {
				for i := len(r.middlewares) - 1; i >= 0; i-- {
					match.Handler = r.middlewares[i].Middleware(match.Handler)
				}
			}
			return true
		}
	}

	if match.MatchErr == ErrMethodMismatch {
		if r.MethodNotAllowedHandler != nil {
			match.Handler = r.MethodNotAllowedHandler
			return true
		}
		return false
	}

	// 最接近匹配的路由器(包括子路由器)
	if r.NotFoundHandler != nil {
		match.Handler = r.NotFoundHandler
		match.MatchErr = ErrNotFound
		return true
	}

	match.MatchErr = ErrNotFound
	return false
}

// ServeHTTP 分派匹配路由中注册的处理器，当有匹配时，可以调用mux.Vars(request)
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if !r.skipClean {
		path := req.URL.Path
		if r.useEncodedPath {
			path = req.URL.EscapedPath()
		}
		// 清理路径到规范形式并重定向。
		if p := cleanPath(path); p != path {
			// http://code.google.com/p/go/issues/detail?id=5252
			url := *req.URL
			url.Path = p
			p = url.String()

			w.Header().Set("Location", p)
			w.WriteHeader(http.StatusMovedPermanently)
			return
		}
	}
	var match RouteMatch
	var handler http.Handler
	if r.Match(req, &match) {
		handler = match.Handler
		req = requestWithVars(req, match.Vars)
		req = requestWithRoute(req, match.Route)
	}

	if handler == nil && match.MatchErr == ErrMethodMismatch {
		handler = methodNotAllowedHandler()
	}

	if handler == nil {
		handler = http.NotFoundHandler()
	}

	handler.ServeHTTP(w, req)
}

// Get 返回用给定名称注册的路由
func (r *Router) Get(name string) *Route {
	return r.namedRoutes[name]
}

// StrictSlash 定义新路由的尾斜杠行为，默认值为false，子路由会继承此设置
func (r *Router) StrictSlash(value bool) *Router {
	r.strictSlash = value
	return r
}

// SkipClean 清理path路径，默认为false
func (r *Router) SkipClean(value bool) *Router {
	r.skipClean = value
	return r
}

// UseEncodedPath 匹配经过编码的原始路径
// 如： "/path/foo%2Fbar/to" 会匹配到 "/path/{var}/to".
// 如果没被调用 "/path/foo%2Fbar/to" 匹配到 "/path/foo/bar/to"
func (r *Router) UseEncodedPath() *Router {
	r.useEncodedPath = true
	return r
}

// ----------------------------------------------------------------------------
// 路由工厂
// ----------------------------------------------------------------------------

// NewRoute 注册空路由
func (r *Router) NewRoute() *Route {
	// initialize a route with a copy of the parent router's configuration
	route := &Route{routeConf: copyRouteConf(r.routeConf), namedRoutes: r.namedRoutes}
	r.routes = append(r.routes, route)
	return route
}

// Name 注册一个带有名称的新路由
func (r *Router) Name(name string) *Route {
	return r.NewRoute().Name(name)
}

// Handle 使用url匹配器注册新路由
func (r *Router) Handle(path string, handler http.Handler) *Route {
	return r.NewRoute().Path(path).Handler(handler)
}

// HandleFunc 用匹配器为URL路径注册一个新路由。
func (r *Router) HandleFunc(path string, f func(http.ResponseWriter,
	*http.Request)) *Route {
	return r.NewRoute().Path(path).HandlerFunc(f)
}

// Headers 用请求标头值匹配器注册一个新路由
func (r *Router) Headers(pairs ...string) *Route {
	return r.NewRoute().Headers(pairs...)
}

// Host 为URL主机注册一个新的路由匹配器
func (r *Router) Host(tpl string) *Route {
	return r.NewRoute().Host(tpl)
}

// MatcherFunc 用自定义匹配器函数注册一条新路由
func (r *Router) MatcherFunc(f MatcherFunc) *Route {
	return r.NewRoute().MatcherFunc(f)
}

// Methods 用HTTP方法的匹配器注册一个新路由
func (r *Router) Methods(methods ...string) *Route {
	return r.NewRoute().Methods(methods...)
}

// Path 用匹配器为URL路径注册一个新路由
func (r *Router) Path(tpl string) *Route {
	return r.NewRoute().Path(tpl)
}

// PathPrefix 用URL路径前缀的匹配器注册一个新路由
func (r *Router) PathPrefix(tpl string) *Route {
	return r.NewRoute().PathPrefix(tpl)
}

// Queries 用URL查询值的匹配器注册一个新路由
func (r *Router) Queries(pairs ...string) *Route {
	return r.NewRoute().Queries(pairs...)
}

// Schemes 为URL方案的匹配器注册一个新路由
func (r *Router) Schemes(schemes ...string) *Route {
	return r.NewRoute().Schemes(schemes...)
}

// BuildVarsFunc 用自定义函数注册一条新的路由
func (r *Router) BuildVarsFunc(f BuildVarsFunc) *Route {
	return r.NewRoute().BuildVarsFunc(f)
}

// Walk 遍历路由器及其所有子路由器，对每条路由调用walkFn
// 在树中。路径是按照添加的顺序进行遍历的。Sub-routers 深度优先
func (r *Router) Walk(walkFn WalkFunc) error {
	return r.walk(walkFn, []*Route{})
}

// SkipRouter 从WalkFuncs返回一个SkipRouter值,walk将要下行到的路由器应该被跳过
var SkipRouter = errors.New("skip this router")

// WalkFunc 是Walk访问的每条路由所调用的函数类型,每次调用时，都会给出当前路由和当前路由器以及指向当前路由的祖先路由列表
type WalkFunc func(route *Route, router *Router, ancestors []*Route) error

func (r *Router) walk(walkFn WalkFunc, ancestors []*Route) error {
	for _, t := range r.routes {
		err := walkFn(t, r, ancestors)
		if err == SkipRouter {
			continue
		}
		if err != nil {
			return err
		}
		for _, sr := range t.matchers {
			if h, ok := sr.(*Router); ok {
				ancestors = append(ancestors, t)
				err := h.walk(walkFn, ancestors)
				if err != nil {
					return err
				}
				ancestors = ancestors[:len(ancestors)-1]
			}
		}
		if h, ok := t.handler.(*Router); ok {
			ancestors = append(ancestors, t)
			err := h.walk(walkFn, ancestors)
			if err != nil {
				return err
			}
			ancestors = ancestors[:len(ancestors)-1]
		}
	}
	return nil
}
