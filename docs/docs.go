// GENERATED BY THE COMMAND ABOVE; DO NOT EDIT
// This file was generated by swaggo/swag

package docs

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/alecthomas/template"
	"github.com/swaggo/swag"
)

var doc = `{
    "schemes": {{ marshal .Schemes }},
    "swagger": "2.0",
    "info": {
        "description": "{{.Description}}",
        "title": "{{.Title}}",
        "termsOfService": "http://swagger.io/terms/",
        "contact": {
            "name": "API Support",
            "url": "http://www.swagger.io/support",
            "email": "support@swagger.io"
        },
        "license": {
            "name": "Apache 2.0",
            "url": "http://www.apache.org/licenses/LICENSE-2.0.html"
        },
        "version": "{{.Version}}"
    },
    "host": "{{.Host}}",
    "basePath": "{{.BasePath}}",
    "paths": {
        "/graph/namespace/{namespace}/duration/{duration}/deadEdges/{deadEdges}/passThrough/{passThrough}": {
            "post": {
                "description": "通过namespace来查询流量视图",
                "consumes": [
                    "application/json"
                ],
                "tags": [
                    "graph"
                ],
                "summary": "graph-namespace",
                "operationId": "GetNamespaces",
                "parameters": [
                    {
                        "type": "string",
                        "description": "命名空间",
                        "name": "namespace",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "时长",
                        "name": "duration",
                        "in": "path",
                        "required": true
                    },
                    {
                        "description": "集群信息",
                        "name": "cluster",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/handlers.NamespacesRequest"
                        }
                    },
                    {
                        "type": "boolean",
                        "description": "是否去掉没有流量的线",
                        "name": "deadEdges",
                        "in": "path"
                    },
                    {
                        "type": "boolean",
                        "description": "是否需要加多集群的线",
                        "name": "passThrough",
                        "in": "path"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/handlers.GraphNamespacesResponse"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/handlers.responseError"
                        }
                    }
                }
            }
        },
        "/graph/namespace/{namespace}/service/{service}/duration/{duration}/deadEdges/{deadEdges}/passThrough/{passThrough}": {
            "post": {
                "description": "通过node来查询流量视图",
                "consumes": [
                    "application/json"
                ],
                "tags": [
                    "graph"
                ],
                "summary": "graph-Node",
                "operationId": "GetNode",
                "parameters": [
                    {
                        "type": "string",
                        "description": "命名空间",
                        "name": "namespace",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "时长",
                        "name": "duration",
                        "in": "path",
                        "required": true
                    },
                    {
                        "description": "集群信息",
                        "name": "cluster",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/handlers.NamespacesRequest"
                        }
                    },
                    {
                        "type": "string",
                        "description": "service 名称",
                        "name": "service",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "boolean",
                        "description": "是否去掉没有流量的线",
                        "name": "deadEdges",
                        "in": "path"
                    },
                    {
                        "type": "boolean",
                        "description": "是否需要加多集群的线",
                        "name": "passThrough",
                        "in": "path"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/handlers.GraphNamespacesResponse"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/handlers.responseError"
                        }
                    }
                }
            }
        }
    },
    "definitions": {
        "handlers.GraphName": {
            "type": "object",
            "properties": {
                "cluster": {
                    "type": "object"
                },
                "passthrough": {
                    "type": "object"
                }
            }
        },
        "handlers.GraphNamespacesResponse": {
            "type": "object",
            "properties": {
                "code": {
                    "type": "integer"
                },
                "data": {
                    "type": "object",
                    "$ref": "#/definitions/handlers.GraphName"
                },
                "message": {
                    "type": "string"
                }
            }
        },
        "handlers.NamespacesRequest": {
            "type": "object",
            "properties": {
                "clusters": {
                    "type": "object",
                    "additionalProperties": {
                        "type": "string"
                    }
                }
            }
        },
        "handlers.responseError": {
            "type": "object",
            "properties": {
                "detail": {
                    "type": "string"
                },
                "error": {
                    "type": "string"
                }
            }
        }
    }
}`

type swaggerInfo struct {
	Version     string
	Host        string
	BasePath    string
	Schemes     []string
	Title       string
	Description string
}

// SwaggerInfo holds exported Swagger Info so clients can modify it
var SwaggerInfo = swaggerInfo{
	Version:     "1.0",
	Host:        "localhost:8000",
	BasePath:    "/",
	Schemes:     []string{},
	Title:       "Swagger Kiali API",
	Description: "This is a sample server Petstore server.",
}

type s struct{}

func (s *s) ReadDoc() string {
	sInfo := SwaggerInfo
	sInfo.Description = strings.Replace(sInfo.Description, "\n", "\\n", -1)

	t, err := template.New("swagger_info").Funcs(template.FuncMap{
		"marshal": func(v interface{}) string {
			a, _ := json.Marshal(v)
			return string(a)
		},
	}).Parse(doc)
	if err != nil {
		return doc
	}

	var tpl bytes.Buffer
	if err := t.Execute(&tpl, sInfo); err != nil {
		return doc
	}

	return tpl.String()
}

func init() {
	swag.Register(swag.Name, &s{})
}
