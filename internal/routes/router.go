package routes

import (
	"context"
	_ "embed"
	"log"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"

	"code-base-golang/internal/config"
	"code-base-golang/internal/controllers"
	"code-base-golang/internal/middlewares"
)

//go:embed routes.yaml
var routesYAML []byte

// routeItem defines a route mapping from routes.yaml.
// The schema contract strictly uses lowercase keys: method, path, handler.
type routeItem struct {
	Method          string `yaml:"method"`
	Path            string `yaml:"path"`
	Handler         string `yaml:"handler"`
	Auth            bool   `yaml:"auth,omitempty"`
	OptionalAuth    bool   `yaml:"optional_auth,omitempty"`
	BasicAuth       bool   `yaml:"basic_auth,omitempty"`
	AdminAuth       bool   `yaml:"admin_auth,omitempty"`
	AdminPermission string `yaml:"admin_permission,omitempty"`
}

// UnmarshalYAML implements case-insensitive mapping for route keys (e.g. method/Method, path/Path).
func (r *routeItem) UnmarshalYAML(value *yaml.Node) error {
	var raw map[string]interface{}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	for k, v := range raw {
		switch strings.ToLower(k) {
		case "method":
			if s, ok := v.(string); ok {
				r.Method = s
			}
		case "path":
			if s, ok := v.(string); ok {
				r.Path = s
			}
		case "handler":
			if s, ok := v.(string); ok {
				r.Handler = s
			}
		case "auth":
			if b, ok := v.(bool); ok {
				r.Auth = b
			}
		case "optional_auth":
			if b, ok := v.(bool); ok {
				r.OptionalAuth = b
			}
		case "basic_auth":
			if b, ok := v.(bool); ok {
				r.BasicAuth = b
			}
		case "admin_auth":
			if b, ok := v.(bool); ok {
				r.AdminAuth = b
			}
		case "admin_permission":
			if s, ok := v.(string); ok {
				r.AdminPermission = s
			}
		}
	}
	return nil
}

type routeConfig struct {
	Routes []routeItem `yaml:"routes"`
}

// NewRouter constructs the central Gin engine and wires controllers and middleware.
func NewRouter(cfg config.Config, ctrls *controllers.Controllers, authValidator ...middlewares.AuthValidator) *gin.Engine {
	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	corsCfg := middlewares.CORSConfig{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   cfg.CORSAllowedMethods,
		AllowedHeaders:   cfg.CORSAllowedHeaders,
		AllowCredentials: cfg.CORSAllowCredentials,
		MaxAge:           cfg.CORSMaxAge,
	}

	// Central controllers container
	if ctrls == nil {
		ctrls = controllers.New(cfg, nil)
	}
	ctrlsVal := reflect.ValueOf(ctrls)

	router.Use(
		middlewares.StructuredLogger(),
		middlewares.Recovery(),
		middlewares.CORS(corsCfg),
		middlewares.RateLimit(cfg.RateLimiterLimit, cfg.RateLimiterBurst, cfg.RateLimiterEnabled),
		middlewares.ClientMeta(),
		middlewares.BodyLimit(cfg.MaxRequestBodySize),
		middlewares.MaintenanceMode(func(ctx context.Context) bool {
			if ctrls != nil && ctrls.Service() != nil {
				return ctrls.Service().IsMaintenanceMode(ctx)
			}
			return false
		}),
	)

	// Auth validator for protected routes
	var validator middlewares.AuthValidator
	var adminValidator middlewares.AdminAuthValidator
	if len(authValidator) > 0 && authValidator[0] != nil {
		validator = authValidator[0]
		if av, ok := validator.(middlewares.AdminAuthValidator); ok {
			adminValidator = av
		}
	}

	// Load and parse embedded YAML routes
	var rCfg routeConfig
	if err := yaml.Unmarshal(routesYAML, &rCfg); err != nil {
		log.Fatalf("failed to parse routes.yaml: %v", err)
	}

	// Auto-dispatch routes dynamically via reflection (zero manual mapping)
	for _, r := range rCfg.Routes {
		methodName := strings.TrimSpace(r.Handler)
		method := ctrlsVal.MethodByName(methodName)

		if !method.IsValid() {
			log.Fatalf("route configuration error: method '%s' not found on Controllers for route %s %s", methodName, r.Method, r.Path)
		}

		fn, ok := method.Interface().(func(*gin.Context))
		if !ok {
			log.Fatalf("route configuration error: method '%s' must have signature func(*gin.Context)", methodName)
		}

		var handlers []gin.HandlerFunc
		if r.BasicAuth {
			handlers = append(handlers, middlewares.BasicAuth(cfg.BasicAuthUsername, cfg.BasicAuthPassword))
		}
		if r.Auth {
			handlers = append(handlers, middlewares.RequireAuth(validator))
		}
		if r.OptionalAuth {
			handlers = append(handlers, middlewares.OptionalAuth(validator))
		}
		if r.AdminAuth || r.AdminPermission != "" {
			if adminValidator == nil {
				log.Fatalf("route configuration error: admin validator required for route %s %s", r.Method, r.Path)
			}
			handlers = append(handlers, middlewares.RequireAdminAuth(adminValidator))
		}
		if r.AdminPermission != "" {
			handlers = append(handlers, middlewares.RequireAdminPermission(r.AdminPermission))
		}
		handlers = append(handlers, fn)

		httpMethod := strings.ToUpper(strings.TrimSpace(r.Method))
		router.Handle(httpMethod, r.Path, handlers...)
	}

	return router
}
