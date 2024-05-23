package telemetriesgql

import _ "github.com/suessflorian/gqlfetch"

//go:generate sh -c "go run github.com/suessflorian/gqlfetch/gqlfetch --endpoint http://localhost:8081/api/telemetry/query > schema.graphql"
//go:generate go run github.com/Khan/genqlient ./genqlient.yaml
