package api

// rotas e tipos (sem as tags registradas à mão) e, separado, o spec completo para /docs
//go:generate go tool oapi-codegen -config config.yaml ../../docs/openapi.yaml
//go:generate go tool oapi-codegen -config spec.config.yaml ../../docs/openapi.yaml
