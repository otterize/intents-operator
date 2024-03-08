package telemetriesgql

//go:generate sh -c "go run github.com/suessflorian/gqlfetch/gqlfetch --endpoint https://app.staging.otterize.com/api/telemetry/query > schema.graphql"
//go:generate go run github.com/Khan/genqlient ./genqlient.yaml
