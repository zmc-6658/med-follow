package model

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{Code: 0, Message: "ok", Data: data})
}

func AbortErr(c *gin.Context, httpStatus, bizCode int, msg string) {
	c.AbortWithStatusJSON(httpStatus, Response{Code: bizCode, Message: msg})
}
