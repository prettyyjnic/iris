package iris

import (
	"fmt"
	"strconv"
	"strings"
)

type (
	// IRoute is the interface which the Route should implements
	// it useful to have it as an interface because this interface is passed to the plugins
	IRoute interface {
		GetMethod() string
		GetDomain() string
		GetPath() string
		GetName() string
		// Name sets the name of the route
		Name(string) IRoute
		GetMiddleware() Middleware
		HasCors() bool
		// used internaly to check arguments with the route's named parameters
		parse(...interface{}) (string, bool)
	}

	// RouteNameFunc is returned to from route handle
	RouteNameFunc func(string) IRoute

	// Route contains basic and temporary info about the route in order to be stored to the tree
	Route struct {
		method   string
		domain   string
		fullpath string
		// the name of the route, the default name is just the registed path.
		name       string
		middleware Middleware

		// this is used to convert  /mypath/:aparam/:something to -> /mypath/%v/%v and /mypath/* -> mypath/%v
		// we use %v to escape from the conversions between strings,booleans and integers.
		// used inside custom html template func 'url'
		formattedPath string
		// formattedParts is just the formattedPath count, used to see if we have one path parameter then the url's function arguments will be passed as one string to the %v
		formattedParts int
	}
)

var _ IRoute = &Route{}

// NewRoute creates, from a path string, and a slice of HandlerFunc
func NewRoute(method string, registedPath string, middleware Middleware) *Route {
	domain := ""
	//dirdy but I'm not touching this again:P
	if registedPath[0] != SlashByte && strings.Contains(registedPath, ".") && (strings.IndexByte(registedPath, SlashByte) == -1 || strings.IndexByte(registedPath, SlashByte) > strings.IndexByte(registedPath, '.')) {
		//means that is a path with domain
		//we have to extract the domain

		//find the first '/'
		firstSlashIndex := strings.IndexByte(registedPath, SlashByte)

		//firt of all remove the first '/' if that exists and we have domain
		if firstSlashIndex == 0 {
			//e.g /admin.ideopod.com/hey
			//then just remove the first slash and re-execute the NewRoute and return it
			registedPath = registedPath[1:]
			return NewRoute(method, registedPath, middleware)
		}
		//if it's just the domain, then set it(registedPath) as the domain
		//and after set the registedPath to a slash '/' for the path part
		if firstSlashIndex == -1 {
			domain = registedPath
			registedPath = Slash
		} else {
			//we have a domain + path
			domain = registedPath[0:firstSlashIndex]
			registedPath = registedPath[len(domain):]
		}

	}
	r := &Route{method: method, domain: domain, fullpath: registedPath, middleware: middleware, name: registedPath, formattedPath: registedPath}
	r.formatPath()
	return r
}

func (r *Route) formatPath() {
	// we don't care about performance here, no runtime func.

	n1Len := strings.Count(r.fullpath, ":")
	isMatchEverything := r.fullpath[len(r.fullpath)-1] == MatchEverythingByte
	if n1Len == 0 && !isMatchEverything {
		// its a static
		return
	}
	if n1Len == 0 && isMatchEverything {
		//if we have something like: /mypath/anything/* -> /mypatch/anything/%v
		r.formattedPath = r.fullpath[0:len(r.fullpath)-2] + "%v"
		r.formattedParts++
		return
	}

	tempPath := r.fullpath

	splittedN1 := strings.Split(r.fullpath, "/")

	for _, v := range splittedN1 {
		if len(v) > 0 {
			if v[0] == ':' || v[0] == MatchEverythingByte {
				r.formattedParts++
				tempPath = strings.Replace(tempPath, v, "%v", -1) // n1Len, but let it we don't care about performance here.
			}
		}

	}
	r.formattedPath = tempPath
}

// GetMethod returns the http method
func (r Route) GetMethod() string {
	return r.method
}

// GetDomain returns the registed domain which this route is ( if none, is "" which is means "localhost"/127.0.0.1)
func (r Route) GetDomain() string {
	return r.domain
}

// GetPath returns the full registed path
func (r Route) GetPath() string {
	return r.fullpath
}

// GetName returns the name of the route
func (r Route) GetName() string {
	return r.name
}

// Name sets the route's name
func (r *Route) Name(newName string) IRoute {
	r.name = newName
	return r
}

// GetMiddleware returns the chain of the []HandlerFunc registed to this Route
func (r Route) GetMiddleware() Middleware {
	return r.middleware
}

// HasCors check if middleware passsed to a route has cors
func (r *Route) HasCors() bool {
	return RouteConflicts(r, "httpmethod")
}

// used internaly to check arguments with the route's named parameters (iris.initTemplates for funcs)
func (r *Route) parse(args ...interface{}) (string, bool) {
	// check if arguments are not equal to the named parameters ( : = 1, * = all named parameters split to / ), if this happens then send not found err
	///TODO: I'm thinking of making an option to disable these checks and just return a result, because they have cost when rendering an html/template, not too big compared to the render action but... we will see
	// can also do a check if this url can be realy served (_tree.rootBranch.GetBranch(path, ctx.Params))  and if not then return a 404 or a link to a ./templates/errors/404.html
	// but we don't have access to the context itself(so we will have some memory allocations), although it's a good idea but let's keep things simple here.
	argsLen := len(args)
	// we have named parameters but arguments not given
	if argsLen == 0 && r.formattedParts > 0 {
		return "", false
	}

	// we have arguments but they are much more than the named parameters

	// 1 check if we have /*, if yes then join all arguments to one as path and pass that as parameter
	if argsLen > r.formattedParts {
		if r.fullpath[len(r.fullpath)-1] == MatchEverythingByte {
			// we have to convert each argument to a string in this case

			argsString := make([]string, argsLen, argsLen)

			for i, v := range args {
				if s, ok := v.(string); ok {
					argsString[i] = s
				} else if num, ok := v.(int); ok {
					argsString[i] = strconv.Itoa(num)
				} else if b, ok := v.(bool); ok {
					argsString[i] = strconv.FormatBool(b)
				}
			}

			parameter := strings.Join(argsString, Slash)
			result := fmt.Sprintf(r.formattedPath, parameter)
			return result, true
		}
		// 2 if !1 return false
		return "", false
	}

	return fmt.Sprintf(r.formattedPath, args...), true
}

// RouteConflicts checks for route's middleware conflicts
func RouteConflicts(r *Route, with string) bool {
	for _, h := range r.middleware {
		if m, ok := h.(interface {
			Conflicts() string
		}); ok {
			if c := m.Conflicts(); c == with {
				return true
			}
		}
	}
	return false
}
