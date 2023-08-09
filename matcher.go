package mux

import (
	"net/http"
	"regexp"
)

// matcher 匹配器接口
type matcher interface {
	Match(*http.Request, *RouteMatch) bool
}

// headerMatcher 根据报头值匹配请求
type headerMatcher map[string]string

func (m headerMatcher) Match(r *http.Request, match *RouteMatch) bool {
	return matchMapWithString(m, r.Header, true)
}

// headerRegexMatcher 根据给定报头正则表达式的路由匹配请求
type headerRegexMatcher map[string]*regexp.Regexp

func (m headerRegexMatcher) Match(r *http.Request, match *RouteMatch) bool {
	return matchMapWithRegex(m, r.Header, true)
}

// methodMatcher 根据HTTP方法匹配请求
type methodMatcher []string

func (m methodMatcher) Match(r *http.Request, match *RouteMatch) bool {
	return matchInArray(m, r.Method)
}

// schemeMatcher 根据URL模式匹配请求
type schemeMatcher []string

func (m schemeMatcher) Match(r *http.Request, match *RouteMatch) bool {
	scheme := r.URL.Scheme
	if scheme == "" {
		if r.TLS == nil {
			scheme = "http"
		} else {
			scheme = "https"
		}
	}
	return matchInArray(m, scheme)
}
