# mux
实现了一个请求路由器和调度程序，用于将传入的请求匹配到它们各自的处理程序
## 安装
```bash
go get -u github.com/go-mux/mux
```
## 示例
### 简单使用
```go
func main() {
    r := mux.NewRouter()
    r.HandleFunc("/", HomeHandler)
    r.HandleFunc("/products", ProductsHandler)
    r.HandleFunc("/articles", ArticlesHandler)
    http.Handle("/", r)
}
```
### 路径参数
```go
r := mux.NewRouter()
r.HandleFunc("/products/{key}", ProductHandler)
r.HandleFunc("/articles/{category}/", ArticlesCategoryHandler)
r.HandleFunc("/articles/{category}/{id:[0-9]+}", ArticleHandler)
```
### 子域
```go
r := mux.NewRouter()
r.Host("www.example.com")
r.Host("{subdomain:[a-z]+}.example.com")
```
### 其他
```go
// 前缀匹配
r.PathPrefix("/products/")
// 方法匹配
r.Methods("GET", "POST")
// schemes匹配
r.Schemes("https")
// header匹配
r.Headers("X-Requested-With", "XMLHttpRequest")
// 参数匹配
r.Queries("key", "value")
// 自定义匹配
r.MatcherFunc(func(r *http.Request, rm *RouteMatch) bool {
return r.ProtoMajor == 0
})
```
### 链式调用
```go
r.HandleFunc("/products", ProductsHandler).
  Host("www.example.com").
  Methods("GET").
  Schemes("http")
```
### 顺序
如果两条路线匹配，则第一条路线会匹配到
```go
r := mux.NewRouter()
r.HandleFunc("/specific", specificHandler)
r.PathPrefix("/").Handler(catchAllHandler)
```
### 子路由器
```go
// 主机
r := mux.NewRouter()
s := r.Host("www.example.com").Subrouter()
s.HandleFunc("/products/", ProductsHandler)
s.HandleFunc("/products/{key}", ProductHandler)
s.HandleFunc("/articles/{category}/{id:[0-9]+}", ArticleHandler)
// 路径
r := mux.NewRouter()
s := r.PathPrefix("/products").Subrouter()
// "/products/"
s.HandleFunc("/", ProductsHandler)
// "/products/{key}/"
s.HandleFunc("/{key}/", ProductHandler)
// "/products/{key}/details"
s.HandleFunc("/{key}/details", ProductDetailsHandler)
```
### 静态文件
```go
func main() {
    var dir string

    flag.StringVar(&dir, "dir", ".", "the directory to serve files from. Defaults to the current dir")
    flag.Parse()
    r := mux.NewRouter()

    // http://localhost:8000/static/<filename>
    r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir(dir))))

    srv := &http.Server{
        Handler:      r,
        Addr:         "127.0.0.1:8000",
        WriteTimeout: 15 * time.Second,
        ReadTimeout:  15 * time.Second,
    }
    log.Fatal(srv.ListenAndServe())
}
```
### 注册网址
```go
r := mux.NewRouter()
r.HandleFunc("/articles/{category}/{id:[0-9]+}", ArticleHandler).
  Name("article")

// "/articles/go/42"
url, err := r.Get("article").URL("category", "go", "id", "42")


r := mux.NewRouter()
r.Host("{subdomain}.example.com").
Path("/articles/{category}/{id:[0-9]+}").
Queries("filter", "{filter}").
HandlerFunc(ArticleHandler).
Name("article")

// url.String() "http://news.example.com/articles/go/42?filter=mux"
url, err := r.Get("article").URL("subdomain", "news",
"category", "go",
"id", "42",
"filter", "mux")
```
### 钩子
```go
package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-mux/mux"
)

func handler(w http.ResponseWriter, r *http.Request) {
	return
}

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/", handler)
	r.HandleFunc("/products", handler).Methods("POST")
	r.HandleFunc("/articles", handler).Methods("GET")
	r.HandleFunc("/articles/{id}", handler).Methods("GET", "PUT")
	r.HandleFunc("/authors", handler).Queries("surname", "{surname}")
	err := r.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		pathTemplate, err := route.GetPathTemplate()
		if err == nil {
			fmt.Println("ROUTE:", pathTemplate)
		}
		pathRegexp, err := route.GetPathRegexp()
		if err == nil {
			fmt.Println("Path regexp:", pathRegexp)
		}
		queriesTemplates, err := route.GetQueriesTemplates()
		if err == nil {
			fmt.Println("Queries templates:", strings.Join(queriesTemplates, ","))
		}
		queriesRegexps, err := route.GetQueriesRegexp()
		if err == nil {
			fmt.Println("Queries regexps:", strings.Join(queriesRegexps, ","))
		}
		methods, err := route.GetMethods()
		if err == nil {
			fmt.Println("Methods:", strings.Join(methods, ","))
		}
		fmt.Println()
		return nil
	})

	if err != nil {
		fmt.Println(err)
	}

	http.Handle("/", r)
}
```
### 中间件 Middleware
```go
func loggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        log.Println(r.RequestURI)
        next.ServeHTTP(w, r)
    })
}
r := mux.NewRouter()
r.HandleFunc("/", handler)
r.Use(loggingMiddleware)
```
### 处理CORS请求
```go
package main

import (
	"net/http"
	"github.com/go-mux/mux"
)

func main() {
    r := mux.NewRouter()

    // 您必须为中间件指定OPTIONS方法匹配器来设置CORS标头
    r.HandleFunc("/foo", fooHandler).Methods(http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodOptions)
    r.Use(mux.CORSMethodMiddleware(r))
    
    http.ListenAndServe(":8080", r)
}

func fooHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Access-Control-Allow-Origin", "*")
    if r.Method == http.MethodOptions {
        return
    }

    w.Write([]byte("foo"))
}
```

## 完整示例:
```go
package main

import (
    "net/http"
    "log"
    "github.com/go-mux/mux"
)

func YourHandler(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("Gorilla!\n"))
}

func main() {
    r := mux.NewRouter()
    r.HandleFunc("/", YourHandler)
    log.Fatal(http.ListenAndServe(":8000", r))
}
```