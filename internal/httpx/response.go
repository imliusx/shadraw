// Package httpx provides shared HTTP plumbing: envelope responses,
// error codes, middlewares, CORS, rate limiting and validation glue.
package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Envelope is the canonical response shape.
//
//	{ "data": ..., "error": null, "meta": ... }
type Envelope struct {
	Data  any    `json:"data"`
	Error *Error `json:"error"`
	Meta  *Meta  `json:"meta,omitempty"`
}

// Error is the canonical error payload.
type Error struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// Meta carries optional pagination/metadata.
type Meta struct {
	Page       int   `json:"page,omitempty"`
	PageSize   int   `json:"pageSize,omitempty"`
	Total      int64 `json:"total,omitempty"`
	TotalPages int   `json:"totalPages,omitempty"`
}

// OK writes 200 with the data envelope.
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Envelope{Data: data})
}

// OKWithMeta writes 200 with data + meta (used for paginated lists).
func OKWithMeta(c *gin.Context, data any, meta *Meta) {
	c.JSON(http.StatusOK, Envelope{Data: data, Meta: meta})
}

// Created writes 201 with the data envelope.
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, Envelope{Data: data})
}

// Fail writes a status with an error envelope and aborts the chain.
func Fail(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, Envelope{Error: &Error{Code: code, Message: message}})
}

// FailWithFields is Fail plus a field-error map (typically 422 validation).
func FailWithFields(c *gin.Context, status int, code, message string, fields map[string]string) {
	c.AbortWithStatusJSON(status, Envelope{Error: &Error{Code: code, Message: message, Fields: fields}})
}
