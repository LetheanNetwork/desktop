// SPDX-Licence-Identifier: EUPL-1.2

// HTTP control surface — POST /v1/api/opencode/sandbox spawns a new
// sandbox; GET /v1/api/opencode/sandbox lists running ones; DELETE
// /v1/api/opencode/sandbox/:id stops one. The CLI subcommand is a
// thin client over these endpoints so opencode lifecycle work always
// happens in the lthn-serve process — same Core, same proxy map.

package opencode

import (
	"net/http"

	core "dappco.re/go"
	"github.com/gin-gonic/gin"
)

// ControlGroup implements coreapi.RouteGroup for the opencode HTTP
// control surface.
type ControlGroup struct {
	svc *Service
}

// NewControlGroup binds the route group to an opencode Service.
//
// Usage example:
//
//	engine.Register(opencode.NewControlGroup(opencodeSvc))
func NewControlGroup(svc *Service) *ControlGroup {
	return &ControlGroup{svc: svc}
}

// Name satisfies coreapi.RouteGroup.
func (g *ControlGroup) Name() string { return "opencode" }

// BasePath satisfies coreapi.RouteGroup.
func (g *ControlGroup) BasePath() string { return "/v1/api/opencode" }

// RegisterRoutes satisfies coreapi.RouteGroup.
func (g *ControlGroup) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/sandbox", g.spawn)
	rg.GET("/sandbox", g.list)
	rg.DELETE("/sandbox/:id", g.stop)
	rg.GET("/sandbox/:id", g.inspect)
}

// spawn POST /v1/api/opencode/sandbox → spawns a new container.
// Returns the sandbox record + reachable URL on success.
func (g *ControlGroup) spawn(c *gin.Context) {
	r := g.svc.Start()
	if !r.OK {
		c.JSON(http.StatusInternalServerError, gin.H{"error": r.Error()})
		return
	}
	id, _ := r.Value.(string)
	c.JSON(http.StatusOK, gin.H{
		"id":  id,
		"url": "/v1/api/sandbox/" + id,
	})
}

// list GET /v1/api/opencode/sandbox → returns all running sandboxes.
func (g *ControlGroup) list(c *gin.Context) {
	r := g.svc.Status()
	if !r.OK {
		c.JSON(http.StatusInternalServerError, gin.H{"error": r.Error()})
		return
	}
	list, _ := r.Value.([]Sandbox)
	c.JSON(http.StatusOK, gin.H{"sandboxes": list})
}

// stop DELETE /v1/api/opencode/sandbox/:id → stops + removes one.
func (g *ControlGroup) stop(c *gin.Context) {
	id := core.TrimCutset(c.Param("id"), "/ ")
	r := g.svc.Stop(id)
	if !r.OK {
		c.JSON(http.StatusInternalServerError, gin.H{"error": r.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"stopped": id})
}

// inspect GET /v1/api/opencode/sandbox/:id → returns one record.
func (g *ControlGroup) inspect(c *gin.Context) {
	id := core.TrimCutset(c.Param("id"), "/ ")
	r := g.svc.Inspect(id)
	if !r.OK {
		c.JSON(http.StatusNotFound, gin.H{"error": r.Error()})
		return
	}
	sb, _ := r.Value.(Sandbox)
	c.JSON(http.StatusOK, sb)
}
