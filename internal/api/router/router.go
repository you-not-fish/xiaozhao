// Package router wires middlewares and v1 handlers into a single
// chi.Router. Composition happens here so cmd/server stays small.
package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	v1 "github.com/xiaozhao/xiaozhao/internal/api/v1"
	"github.com/xiaozhao/xiaozhao/internal/app/rbac"
	"github.com/xiaozhao/xiaozhao/internal/gateway/middleware"
	"github.com/xiaozhao/xiaozhao/internal/pkg/config"
	"github.com/xiaozhao/xiaozhao/internal/pkg/jwt"
)

// Deps groups everything the router needs from the outer composition root.
type Deps struct {
	Log      *zap.Logger
	Config   *config.Config
	JWT      *jwt.Manager
	Redis    *redis.Client
	RBAC     *rbac.Checker
	Auth     *v1.AuthHandler
	Org      *v1.OrgHandler
	Project  *v1.ProjectHandler
	Response *v1.ResponseHandler
	Health   *v1.HealthHandler
}

// New builds the top-level HTTP router.
func New(d Deps) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID())
	r.Use(middleware.Recovery())
	r.Use(middleware.Logging(d.Log))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "X-Request-Id", "X-Organization-Id"},
		ExposedHeaders:   []string{"X-Request-Id"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// Public routes (no auth).
	r.Get("/healthz", d.Health.Live)

	r.Route("/v1", func(r chi.Router) {
		// Rate limiting is applied to the whole /v1 surface; it's fail-open
		// so auth endpoints still work when Redis is unreachable.
		r.Use(middleware.RateLimit(d.Redis, d.Config.RateLimit))

		// Public auth endpoints.
		r.Group(func(r chi.Router) {
			r.Post("/auth/register", d.Auth.Register)
			r.Post("/auth/login", d.Auth.Login)
		})

		// Authenticated routes that don't need a tenant context.
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(d.JWT))
			r.Get("/auth/me", d.Auth.Me)
			r.Post("/auth/switch_org", d.Auth.SwitchOrg)
			// Org listing/creation operates above the tenant boundary.
			r.Get("/orgs", d.Org.List)
			r.Post("/orgs", d.Org.Create)
			r.Get("/orgs/{orgID}", d.Org.Get)
			r.Patch("/orgs/{orgID}", d.Org.Rename)
			r.Get("/orgs/{orgID}/members", d.Org.ListMembers)
			r.Post("/orgs/{orgID}/members", d.Org.UpsertMember)
			r.Delete("/orgs/{orgID}/members/{userID}", d.Org.RemoveMember)
		})

		// Tenant-scoped routes (require X-Organization-Id or JWT-embedded org).
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(d.JWT))
			r.Use(middleware.Tenant(d.RBAC))
			r.Post("/projects", d.Project.Create)
			r.Get("/projects", d.Project.List)
			r.Get("/projects/{projectID}", d.Project.Get)
			r.Patch("/projects/{projectID}", d.Project.Update)
			r.Delete("/projects/{projectID}", d.Project.Delete)
			if d.Response != nil {
				r.Post("/responses", d.Response.Create)
				r.Get("/responses/{responseID}/events", d.Response.Events)
			}
		})
	})

	return r
}
