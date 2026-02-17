package server

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ServiceFunc is the generic signature for every service method.
type ServiceFunc[Req any, Resp any] func(context.Context, *Req) (*Resp, error)

// ──────────────────────────────────────────────
//  Generic binding helpers (same pattern as contentsvc)
// ──────────────────────────────────────────────

type bindingType int

const (
	bindJSON bindingType = iota
	bindURI
	bindQuery
	bindAll
)

// Response is the standard JSON envelope returned by every endpoint.
type Response struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func handleGenericWithBinding[Req any, Resp any](
	svcFunc ServiceFunc[Req, Resp],
	bt bindingType,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req Req
		var bindErr error

		switch bt {
		case bindJSON:
			bindErr = c.ShouldBindJSON(&req)
		case bindURI:
			bindErr = c.ShouldBindUri(&req)
		case bindQuery:
			bindErr = c.ShouldBindQuery(&req)
		case bindAll:
			if err := c.ShouldBindUri(&req); err != nil {
				bindErr = err
			}
			if err := c.ShouldBindQuery(&req); err != nil {
				bindErr = err
			}
			if c.Request.ContentLength > 0 {
				if err := c.ShouldBindJSON(&req); err != nil {
					bindErr = err
				}
			}
		}

		if bindErr != nil {
			c.JSON(http.StatusBadRequest, Response{
				Code:    "BAD_REQUEST",
				Message: bindErr.Error(),
			})
			return
		}

		resp, err := svcFunc(c.Request.Context(), &req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, Response{
				Code:    "INTERNAL_ERROR",
				Message: err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, Response{
			Code:    "OK",
			Message: "success",
			Data:    resp,
		})
	}
}

// handleQuery binds request from query string parameters.
func handleQuery[Req any, Resp any](fn ServiceFunc[Req, Resp]) gin.HandlerFunc {
	return handleGenericWithBinding(fn, bindQuery)
}

// handleURI binds request from URI path parameters.
func handleURI[Req any, Resp any](fn ServiceFunc[Req, Resp]) gin.HandlerFunc {
	return handleGenericWithBinding(fn, bindURI)
}

// handleJSON binds request from JSON body.
func handleJSON[Req any, Resp any](fn ServiceFunc[Req, Resp]) gin.HandlerFunc {
	return handleGenericWithBinding(fn, bindJSON)
}

// handleAll binds from URI + query + JSON body (for PUT with path params).
func handleAll[Req any, Resp any](fn ServiceFunc[Req, Resp]) gin.HandlerFunc {
	return handleGenericWithBinding(fn, bindAll)
}
