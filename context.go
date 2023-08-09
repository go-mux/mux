package mux

import (
	"context"
	"net/http"
)

// RouteMatch 保存匹配的路由信息
type RouteMatch struct {
	Route   *Route
	Handler http.Handler
	Vars    map[string]string

	// MatchErr 设置为适当的匹配错误，如果存在不匹配，则设置为ErrMethodMismatch
	MatchErr error
}

type contextKey int

const (
	varsKey contextKey = iota
	routeKey
)

// Vars 返回当前请求的路由变量(如果有)
func Vars(r *http.Request) map[string]string {
	if rv := r.Context().Value(varsKey); rv != nil {
		return rv.(map[string]string)
	}
	return nil
}

// CurrentRoute 返回当前请求匹配的路由(如果有)
func CurrentRoute(r *http.Request) *Route {
	if rv := r.Context().Value(routeKey); rv != nil {
		return rv.(*Route)
	}
	return nil
}

func requestWithVars(r *http.Request, vars map[string]string) *http.Request {
	ctx := context.WithValue(r.Context(), varsKey, vars)
	return r.WithContext(ctx)
}

func requestWithRoute(r *http.Request, route *Route) *http.Request {
	ctx := context.WithValue(r.Context(), routeKey, route)
	return r.WithContext(ctx)
}
